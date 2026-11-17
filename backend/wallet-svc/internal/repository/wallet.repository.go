package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"time"

	"wallet-svc/internal/config"
	"wallet-svc/internal/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"
)

// readySystemApps caches app IDs whose APP shards + MINT are known ready in this process.
// Value is the shard count used when marked ready.
var readySystemApps sync.Map

const walletSelectCols = `id, app_id, user_id, wallet_type, shard_id, balance, status, version, created_at, updated_at`

type ApplyResult struct {
	Wallet        model.Wallet
	Entry         model.LedgerEntry
	ContraEntry   model.LedgerEntry
	TransactionID uuid.UUID
	Duplicate     bool
}

type LedgerPage struct {
	Items      []model.LedgerEntry
	NextCursor string
}

// LedgerListParams filters and sorts wallet ledger pages.
// SortAsc=false (default) orders by created_at DESC, id DESC.
type LedgerListParams struct {
	CursorTime time.Time
	CursorID   uuid.UUID
	Limit      int
	EntryType  string
	Reason     string
	SortAsc    bool
}

type ReconcileDrift struct {
	AppID          string `bun:"app_id"`
	UserID         string `bun:"user_id"`
	WalletType     string `bun:"wallet_type"`
	WalletBalance  int64  `bun:"wallet_balance"`
	LedgerComputed int64  `bun:"ledger_computed"`
	Kind           string `bun:"kind"`
}

type WalletRepository interface {
	CreateWallet(ctx context.Context, appID, userID string) (model.Wallet, bool, error)
	GetWallet(ctx context.Context, appID, userID string) (model.Wallet, error)
	Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error)
	FundApp(ctx context.Context, req FundRequest) (ApplyResult, error)
	ListLedger(ctx context.Context, appID, userID string, params LedgerListParams) (LedgerPage, error)
	ListByOrder(ctx context.Context, appID, orderID string) ([]model.LedgerEntry, error)
	SetStatus(ctx context.Context, appID, userID, action, reason, requestID string, status int16) (model.Wallet, error)
	ReconcileBalances(ctx context.Context) ([]ReconcileDrift, error)
	ListUnpublishedOutbox(ctx context.Context, limit int) ([]model.OutboxEvent, error)
	MarkOutboxPublished(ctx context.Context, id uuid.UUID, publishedAt time.Time) error
}

type ApplyRequest struct {
	AppID          string
	UserID         string
	EntryType      string
	Reason         string
	Amount         int64
	IdempotencyKey string
	OrderID        string
	RequestHash    string
	Metadata       map[string]any
	Limits         config.LimitsConfig
	LockTimeout    time.Duration
	StmtTimeout    time.Duration
}

type FundRequest struct {
	AppID          string
	Amount         int64
	IdempotencyKey string
	OrderID        string
	RequestHash    string
	Metadata       map[string]any
	LockTimeout    time.Duration
	StmtTimeout    time.Duration
}

type walletRepository struct {
	db         *bun.DB
	shardCount int
}

func NewWalletRepository(db *bun.DB, shardCount int) WalletRepository {
	if shardCount < 1 {
		shardCount = model.DefaultAppWalletShards
	}
	if shardCount > 64 {
		shardCount = 64
	}
	return &walletRepository{db: db, shardCount: shardCount}
}

func (r *walletRepository) CreateWallet(ctx context.Context, appID, userID string) (model.Wallet, bool, error) {
	var (
		out     model.Wallet
		created bool
	)
	err := r.db.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		if err := r.ensureSystemWallets(ctx, tx, appID); err != nil {
			return err
		}

		now := time.Now().UTC()
		w := model.Wallet{
			ID:         uuid.New(),
			AppID:      appID,
			UserID:     userID,
			WalletType: model.WalletTypeUser,
			ShardID:    0,
			Balance:    0,
			Status:     model.WalletStatusActive,
			Version:    0,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		res, err := tx.NewInsert().
			Model(&w).
			On("CONFLICT (app_id, user_id) WHERE wallet_type = 'USER' DO NOTHING").
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("insert wallet: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			out = w
			created = true
			return nil
		}

		existing, err := lockUserWallet(ctx, tx, appID, userID)
		if err != nil {
			return err
		}
		if existing.Status != model.WalletStatusClosed {
			return ErrWalletExists
		}

		existing.Status = model.WalletStatusActive
		existing.UpdatedAt = now
		if _, err := tx.NewUpdate().
			Model(&existing).
			Column("status", "updated_at").
			Where("id = ?", existing.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("reopen closed wallet: %w", err)
		}
		out = existing
		created = true
		return nil
	})
	if err != nil {
		return model.Wallet{}, false, mapTxError(err)
	}
	return out, created, nil
}

func (r *walletRepository) GetWallet(ctx context.Context, appID, userID string) (model.Wallet, error) {
	var w model.Wallet
	err := r.db.NewSelect().
		Model(&w).
		Where("app_id = ? AND user_id = ? AND wallet_type = ?", appID, userID, model.WalletTypeUser).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Wallet{}, ErrWalletNotFound
	}
	if err != nil {
		return model.Wallet{}, fmt.Errorf("get wallet: %w", err)
	}
	return w, nil
}

func (r *walletRepository) Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error) {
	var result ApplyResult
	err := r.db.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		if err := setLocalTimeouts(ctx, tx, req.LockTimeout, req.StmtTimeout); err != nil {
			return err
		}
		if req.AppID == "" {
			return fmt.Errorf("app_id is required")
		}

		if err := r.ensureSystemWallets(ctx, tx, req.AppID); err != nil {
			return err
		}

		if req.Reason == model.ReasonTokenPurchase {
			if err := lazyCreateUserWallet(ctx, tx, req.AppID, req.UserID); err != nil {
				return err
			}
		}

		userWallet, err := getUserWallet(ctx, tx, req.AppID, req.UserID)
		if err != nil {
			if errors.Is(err, ErrWalletNotFound) && req.Reason == model.ReasonSpend {
				return ErrInsufficientFunds
			}
			return err
		}

		if found, replayUser, replayContra, err := lookupExistingPair(ctx, tx, userWallet.ID, req.IdempotencyKey, req.Reason, req.OrderID, req.RequestHash); err != nil {
			return err
		} else if found {
			result = ApplyResult{
				Wallet:        userWallet,
				Entry:         replayUser,
				ContraEntry:   replayContra,
				TransactionID: replayUser.TransactionID,
				Duplicate:     true,
			}
			result.Wallet.Balance = replayUser.BalanceAfter
			return nil
		}

		shard := appShardIndex(req.UserID, r.shardCount)
		appWallet, err := getAppWalletShard(ctx, tx, req.AppID, shard)
		if err != nil {
			return err
		}

		userWallet, appWallet, err = lockPairByID(ctx, tx, userWallet, appWallet)
		if err != nil {
			return err
		}

		// Re-check under row locks so concurrent same-key retries see the winner as Duplicate
		// instead of racing into insufficient-funds / unique violations.
		if found, replayUser, replayContra, err := lookupExistingPair(ctx, tx, userWallet.ID, req.IdempotencyKey, req.Reason, req.OrderID, req.RequestHash); err != nil {
			return err
		} else if found {
			result = ApplyResult{
				Wallet:        userWallet,
				Entry:         replayUser,
				ContraEntry:   replayContra,
				TransactionID: replayUser.TransactionID,
				Duplicate:     true,
			}
			result.Wallet.Balance = replayUser.BalanceAfter
			return nil
		}

		if err := checkStatus(userWallet); err != nil {
			return err
		}
		if err := checkStatus(appWallet); err != nil {
			return err
		}

		var (
			userType   string
			appType    string
			userBefore = userWallet.Balance
			appBefore  = appWallet.Balance
			userAfter  int64
			appAfter   int64
		)

		switch req.Reason {
		case model.ReasonTokenPurchase:
			userType = model.EntryCredit
			appType = model.EntryDebit
			if req.Amount > req.Limits.MaxCreditAmount {
				return ErrLimitExceeded
			}
			if userWallet.Balance+req.Amount > req.Limits.MaxWalletBalance {
				return ErrLimitExceeded
			}
			credited, err := dailyCreditSum(ctx, tx, userWallet.ID)
			if err != nil {
				return err
			}
			if credited+req.Amount > req.Limits.DailyCreditCap {
				return ErrLimitExceeded
			}
			if appWallet.Balance < req.Amount {
				return ErrInsufficientFunds
			}
			userAfter = userWallet.Balance + req.Amount
			appAfter = appWallet.Balance - req.Amount
		case model.ReasonSpend:
			userType = model.EntryDebit
			appType = model.EntryCredit
			if req.Amount > req.Limits.MaxDebitAmount {
				return ErrLimitExceeded
			}
			if userWallet.Balance < req.Amount {
				return ErrInsufficientFunds
			}
			userAfter = userWallet.Balance - req.Amount
			appAfter = appWallet.Balance + req.Amount
		default:
			return fmt.Errorf("unsupported reason %q", req.Reason)
		}

		txID := uuid.New()
		now := time.Now().UTC()
		userEntry := model.LedgerEntry{
			ID:             uuid.New(),
			TransactionID:  txID,
			WalletID:       userWallet.ID,
			AppID:          req.AppID,
			EntryType:      userType,
			Reason:         req.Reason,
			Amount:         req.Amount,
			BalanceBefore:  userBefore,
			BalanceAfter:   userAfter,
			IdempotencyKey: req.IdempotencyKey,
			RequestHash:    req.RequestHash,
			OrderID:        req.OrderID,
			Metadata:       req.Metadata,
			CreatedAt:      now,
		}
		appEntry := model.LedgerEntry{
			ID:             uuid.New(),
			TransactionID:  txID,
			WalletID:       appWallet.ID,
			AppID:          req.AppID,
			EntryType:      appType,
			Reason:         req.Reason,
			Amount:         req.Amount,
			BalanceBefore:  appBefore,
			BalanceAfter:   appAfter,
			IdempotencyKey: req.IdempotencyKey + ":app",
			RequestHash:    req.RequestHash,
			OrderID:        req.OrderID,
			Metadata:       req.Metadata,
			CreatedAt:      now,
		}

		if userEntry.Amount != appEntry.Amount {
			return fmt.Errorf("double-entry imbalance: amounts differ")
		}
		creditSum, debitSum := int64(0), int64(0)
		for _, e := range []model.LedgerEntry{userEntry, appEntry} {
			switch e.EntryType {
			case model.EntryCredit:
				creditSum += e.Amount
			case model.EntryDebit:
				debitSum += e.Amount
			}
		}
		if creditSum != debitSum {
			return fmt.Errorf("double-entry imbalance: credit %d debit %d", creditSum, debitSum)
		}

		if _, err := tx.NewInsert().Model(&userEntry).Exec(ctx); err != nil {
			return mapWriteError(err)
		}
		if _, err := tx.NewInsert().Model(&appEntry).Exec(ctx); err != nil {
			return mapWriteError(err)
		}

		userWallet.Balance = userAfter
		userWallet.Version++
		userWallet.UpdatedAt = now
		if _, err := tx.NewUpdate().
			Model(&userWallet).
			Column("balance", "version", "updated_at").
			Where("id = ?", userWallet.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("update user wallet: %w", err)
		}

		appWallet.Balance = appAfter
		appWallet.Version++
		appWallet.UpdatedAt = now
		if _, err := tx.NewUpdate().
			Model(&appWallet).
			Column("balance", "version", "updated_at").
			Where("id = ?", appWallet.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("update app wallet: %w", err)
		}

		eventType := model.OutboxCredited
		if req.Reason == model.ReasonSpend {
			eventType = model.OutboxDebited
		}
		outbox := model.OutboxEvent{
			ID:          uuid.New(),
			AggregateID: userWallet.ID,
			EventType:   eventType,
			Payload: map[string]any{
				"app_id":              req.AppID,
				"user_id":             req.UserID,
				"transaction_id":      txID.String(),
				"ledger_id":           userEntry.ID.String(),
				"contra_ledger_id":    appEntry.ID.String(),
				"amount":              req.Amount,
				"user_balance_before": userBefore,
				"user_balance_after":  userAfter,
				"app_balance_before":  appBefore,
				"app_balance_after":   appAfter,
				"app_shard_id":        appWallet.ShardID,
				"reason":              req.Reason,
				"order_id":            req.OrderID,
				"idempotency_key":     req.IdempotencyKey,
			},
			CreatedAt: now,
		}
		if _, err := tx.NewInsert().Model(&outbox).Exec(ctx); err != nil {
			return fmt.Errorf("insert outbox: %w", err)
		}

		result = ApplyResult{
			Wallet:        userWallet,
			Entry:         userEntry,
			ContraEntry:   appEntry,
			TransactionID: txID,
			Duplicate:     false,
		}
		return nil
	})
	if err != nil {
		return ApplyResult{}, mapTxError(err)
	}
	return result, nil
}

func (r *walletRepository) FundApp(ctx context.Context, req FundRequest) (ApplyResult, error) {
	var result ApplyResult
	err := r.db.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		if err := setLocalTimeouts(ctx, tx, req.LockTimeout, req.StmtTimeout); err != nil {
			return err
		}
		if req.AppID == "" {
			return fmt.Errorf("app_id is required")
		}
		if req.Amount <= 0 {
			return fmt.Errorf("amount must be positive")
		}

		if err := r.ensureSystemWallets(ctx, tx, req.AppID); err != nil {
			return err
		}

		mintWallet, err := getMintWallet(ctx, tx, req.AppID)
		if err != nil {
			return err
		}

		if found, replayMint, replayContra, err := lookupExistingPair(ctx, tx, mintWallet.ID, req.IdempotencyKey, model.ReasonAppFund, req.OrderID, req.RequestHash); err != nil {
			return err
		} else if found {
			total, err := sumAppPoolBalance(ctx, tx, req.AppID)
			if err != nil {
				return err
			}
			result = ApplyResult{
				Wallet:        model.Wallet{AppID: req.AppID, WalletType: model.WalletTypeApp, Balance: total},
				Entry:         replayMint,
				ContraEntry:   replayContra,
				TransactionID: replayMint.TransactionID,
				Duplicate:     true,
			}
			return nil
		}

		appShards, err := listAppWalletShards(ctx, tx, req.AppID)
		if err != nil {
			return err
		}
		if len(appShards) != r.shardCount {
			return fmt.Errorf("expected %d APP shards, found %d", r.shardCount, len(appShards))
		}

		toLock := append(appShards, mintWallet)
		locked, err := lockWalletsByID(ctx, tx, toLock)
		if err != nil {
			return err
		}
		mintWallet = locked[mintWallet.ID]
		for i := range appShards {
			appShards[i] = locked[appShards[i].ID]
		}
		if err := checkStatus(mintWallet); err != nil {
			return err
		}
		for _, w := range appShards {
			if err := checkStatus(w); err != nil {
				return err
			}
		}

		shares := splitAmount(req.Amount, r.shardCount)
		txID := uuid.New()
		now := time.Now().UTC()
		mintBefore := mintWallet.Balance
		mintAfter := mintBefore - req.Amount

		mintEntry := model.LedgerEntry{
			ID:             uuid.New(),
			TransactionID:  txID,
			WalletID:       mintWallet.ID,
			AppID:          req.AppID,
			EntryType:      model.EntryDebit,
			Reason:         model.ReasonAppFund,
			Amount:         req.Amount,
			BalanceBefore:  mintBefore,
			BalanceAfter:   mintAfter,
			IdempotencyKey: req.IdempotencyKey,
			RequestHash:    req.RequestHash,
			OrderID:        req.OrderID,
			Metadata:       req.Metadata,
			CreatedAt:      now,
		}
		if _, err := tx.NewInsert().Model(&mintEntry).Exec(ctx); err != nil {
			return mapWriteError(err)
		}

		var firstAppEntry model.LedgerEntry
		totalAfter := int64(0)
		for i := range appShards {
			share := shares[i]
			if share == 0 {
				totalAfter += appShards[i].Balance
				continue
			}
			before := appShards[i].Balance
			after := before + share
			entry := model.LedgerEntry{
				ID:             uuid.New(),
				TransactionID:  txID,
				WalletID:       appShards[i].ID,
				AppID:          req.AppID,
				EntryType:      model.EntryCredit,
				Reason:         model.ReasonAppFund,
				Amount:         share,
				BalanceBefore:  before,
				BalanceAfter:   after,
				IdempotencyKey: fmt.Sprintf("%s:app:%d", req.IdempotencyKey, appShards[i].ShardID),
				RequestHash:    req.RequestHash,
				OrderID:        req.OrderID,
				Metadata:       req.Metadata,
				CreatedAt:      now,
			}
			if _, err := tx.NewInsert().Model(&entry).Exec(ctx); err != nil {
				return mapWriteError(err)
			}
			if firstAppEntry.ID == uuid.Nil {
				firstAppEntry = entry
			}

			appShards[i].Balance = after
			appShards[i].Version++
			appShards[i].UpdatedAt = now
			if _, err := tx.NewUpdate().
				Model(&appShards[i]).
				Column("balance", "version", "updated_at").
				Where("id = ?", appShards[i].ID).
				Exec(ctx); err != nil {
				return fmt.Errorf("update app shard %d: %w", appShards[i].ShardID, err)
			}
			totalAfter += after
		}

		mintWallet.Balance = mintAfter
		mintWallet.Version++
		mintWallet.UpdatedAt = now
		if _, err := tx.NewUpdate().
			Model(&mintWallet).
			Column("balance", "version", "updated_at").
			Where("id = ?", mintWallet.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("update mint wallet: %w", err)
		}

		outbox := model.OutboxEvent{
			ID:          uuid.New(),
			AggregateID: mintWallet.ID,
			EventType:   model.OutboxCredited,
			Payload: map[string]any{
				"app_id":            req.AppID,
				"transaction_id":    txID.String(),
				"ledger_id":         mintEntry.ID.String(),
				"contra_ledger_id":  firstAppEntry.ID.String(),
				"amount":            req.Amount,
				"app_pool_balance":  totalAfter,
				"app_wallet_shards": r.shardCount,
				"reason":            model.ReasonAppFund,
				"order_id":          req.OrderID,
				"idempotency_key":   req.IdempotencyKey,
			},
			CreatedAt: now,
		}
		if _, err := tx.NewInsert().Model(&outbox).Exec(ctx); err != nil {
			return fmt.Errorf("insert outbox: %w", err)
		}

		result = ApplyResult{
			Wallet:        model.Wallet{AppID: req.AppID, WalletType: model.WalletTypeApp, Balance: totalAfter},
			Entry:         mintEntry,
			ContraEntry:   firstAppEntry,
			TransactionID: txID,
			Duplicate:     false,
		}
		return nil
	})
	if err != nil {
		return ApplyResult{}, mapTxError(err)
	}
	return result, nil
}

func (r *walletRepository) ListLedger(ctx context.Context, appID, userID string, params LedgerListParams) (LedgerPage, error) {
	wallet, err := r.GetWallet(ctx, appID, userID)
	if err != nil {
		return LedgerPage{}, err
	}
	limit := params.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	q := r.db.NewSelect().
		Model((*model.LedgerEntry)(nil)).
		Where("wallet_id = ?", wallet.ID).
		Limit(int64(limit + 1))
	if params.EntryType != "" {
		q = q.Where("entry_type = ?", params.EntryType)
	}
	if params.Reason != "" {
		q = q.Where("reason = ?", params.Reason)
	}
	if params.SortAsc {
		q = q.Order("created_at ASC", "id ASC")
		if !params.CursorTime.IsZero() && params.CursorID != uuid.Nil {
			q = q.Where("(created_at, id) > (?, ?)", params.CursorTime, params.CursorID)
		}
	} else {
		q = q.Order("created_at DESC", "id DESC")
		if !params.CursorTime.IsZero() && params.CursorID != uuid.Nil {
			q = q.Where("(created_at, id) < (?, ?)", params.CursorTime, params.CursorID)
		}
	}

	var items []model.LedgerEntry
	if err := q.Scan(ctx, &items); err != nil {
		return LedgerPage{}, fmt.Errorf("list ledger: %w", err)
	}

	page := LedgerPage{Items: items}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = items[:limit]
		page.NextCursor = last.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID.String()
	}
	return page, nil
}

func (r *walletRepository) ListByOrder(ctx context.Context, appID, orderID string) ([]model.LedgerEntry, error) {
	var items []model.LedgerEntry
	q := r.db.NewSelect().Model(&items).Where("order_id = ?", orderID)
	if appID != "" {
		q = q.Where("app_id = ?", appID)
	}
	err := q.Order("created_at ASC").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list by order: %w", err)
	}
	return items, nil
}

func (r *walletRepository) SetStatus(ctx context.Context, appID, userID, action, reason, requestID string, status int16) (model.Wallet, error) {
	var wallet model.Wallet
	err := r.db.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx bun.Tx) error {
		var err error
		wallet, err = lockUserWallet(ctx, tx, appID, userID)
		if err != nil {
			return err
		}
		if wallet.Status == model.WalletStatusClosed {
			return ErrWalletClosed
		}
		now := time.Now().UTC()
		wallet.Status = status
		wallet.UpdatedAt = now
		if _, err := tx.NewUpdate().
			Model(&wallet).
			Column("status", "updated_at").
			Where("id = ?", wallet.ID).
			Exec(ctx); err != nil {
			return err
		}
		log := model.AuditLog{
			ID:        uuid.New(),
			UserID:    userID,
			Action:    action,
			Reason:    reason,
			RequestID: requestID,
			CreatedAt: now,
		}
		_, err = tx.NewInsert().Model(&log).Exec(ctx)
		return err
	})
	if err != nil {
		return model.Wallet{}, mapTxError(err)
	}
	return wallet, nil
}

func (r *walletRepository) ReconcileBalances(ctx context.Context) ([]ReconcileDrift, error) {
	var walletDrifts []ReconcileDrift
	err := r.db.NewRaw(`
		SELECT w.app_id,
		       w.user_id,
		       w.wallet_type,
		       w.balance AS wallet_balance,
		       COALESCE(SUM(
		           CASE WHEN le.entry_type = 'CREDIT' THEN le.amount
		                WHEN le.entry_type = 'DEBIT' THEN -le.amount
		                ELSE 0 END
		       ), 0) AS ledger_computed,
		       'wallet' AS kind
		FROM wallets w
		LEFT JOIN ledger_entries le ON le.wallet_id = w.id
		GROUP BY w.id, w.app_id, w.user_id, w.wallet_type, w.balance
		HAVING w.balance <> COALESCE(SUM(
		           CASE WHEN le.entry_type = 'CREDIT' THEN le.amount
		                WHEN le.entry_type = 'DEBIT' THEN -le.amount
		                ELSE 0 END
		       ), 0)
	`).Scan(ctx, &walletDrifts)
	if err != nil {
		return nil, fmt.Errorf("reconcile wallets: %w", err)
	}

	var appDrifts []ReconcileDrift
	err = r.db.NewRaw(`
		SELECT app_id,
		       '' AS user_id,
		       '' AS wallet_type,
		       0 AS wallet_balance,
		       COALESCE(SUM(
		           CASE WHEN entry_type = 'CREDIT' THEN amount
		                WHEN entry_type = 'DEBIT' THEN -amount
		                ELSE 0 END
		       ), 0) AS ledger_computed,
		       'app_double_entry' AS kind
		FROM ledger_entries
		GROUP BY app_id
		HAVING COALESCE(SUM(
		           CASE WHEN entry_type = 'CREDIT' THEN amount
		                WHEN entry_type = 'DEBIT' THEN -amount
		                ELSE 0 END
		       ), 0) <> 0
	`).Scan(ctx, &appDrifts)
	if err != nil {
		return nil, fmt.Errorf("reconcile app double-entry: %w", err)
	}

	return append(walletDrifts, appDrifts...), nil
}

func (r *walletRepository) ListUnpublishedOutbox(ctx context.Context, limit int) ([]model.OutboxEvent, error) {
	var rows []model.OutboxEvent
	err := r.db.NewSelect().Model(&rows).
		Where("published_at IS NULL").
		OrderExpr("created_at ASC").
		Limit(int64(limit)).
		Scan(ctx)
	return rows, err
}

func (r *walletRepository) MarkOutboxPublished(ctx context.Context, id uuid.UUID, publishedAt time.Time) error {
	_, err := r.db.NewUpdate().Model((*model.OutboxEvent)(nil)).
		Set("published_at = ?", publishedAt).
		Where("id = ? AND published_at IS NULL", id).
		Exec(ctx)
	return err
}

func setLocalTimeouts(ctx context.Context, tx bun.Tx, lockTimeout, stmtTimeout time.Duration) error {
	if lockTimeout <= 0 {
		lockTimeout = 2 * time.Second
	}
	if stmtTimeout <= 0 {
		stmtTimeout = 5 * time.Second
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL lock_timeout = '%dms'", lockTimeout.Milliseconds())); err != nil {
		return fmt.Errorf("set lock_timeout: %w", err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL statement_timeout = '%dms'", stmtTimeout.Milliseconds())); err != nil {
		return fmt.Errorf("set statement_timeout: %w", err)
	}
	return nil
}

func (r *walletRepository) ensureSystemWallets(ctx context.Context, tx bun.Tx, appID string) error {
	if v, ok := readySystemApps.Load(appID); ok {
		if n, ok := v.(int); ok && n == r.shardCount {
			return nil
		}
	}

	type row struct {
		WalletType string `bun:"wallet_type"`
		ShardID    int16  `bun:"shard_id"`
	}
	var existing []row
	if err := tx.NewSelect().
		Model((*model.Wallet)(nil)).
		Column("wallet_type", "shard_id").
		Where("app_id = ? AND wallet_type IN (?, ?)", appID, model.WalletTypeApp, model.WalletTypeMint).
		Scan(ctx, &existing); err != nil {
		return fmt.Errorf("lookup system wallets: %w", err)
	}

	haveMint := false
	haveShard := make([]bool, r.shardCount)
	for _, e := range existing {
		switch e.WalletType {
		case model.WalletTypeMint:
			haveMint = true
		case model.WalletTypeApp:
			if int(e.ShardID) >= 0 && int(e.ShardID) < r.shardCount {
				haveShard[e.ShardID] = true
			}
		}
	}

	allShards := true
	for _, ok := range haveShard {
		if !ok {
			allShards = false
			break
		}
	}
	if haveMint && allShards {
		readySystemApps.Store(appID, r.shardCount)
		return nil
	}

	now := time.Now().UTC()
	if !haveMint {
		if err := insertSystemWallet(ctx, tx, appID, model.MintWalletUserID, model.WalletTypeMint, 0, now); err != nil {
			return err
		}
	}
	for shard := 0; shard < r.shardCount; shard++ {
		if haveShard[shard] {
			continue
		}
		if err := insertSystemWallet(ctx, tx, appID, model.AppWalletUserID, model.WalletTypeApp, int16(shard), now); err != nil {
			return err
		}
	}

	// Do not cache readiness here: this runs inside a transaction that may roll back
	// (e.g. purchase fails for empty APP pool). Caching would skip recreate on the next call.
	return nil
}

func insertSystemWallet(ctx context.Context, tx bun.Tx, appID, userID, walletType string, shardID int16, now time.Time) error {
	w := model.Wallet{
		ID:         uuid.New(),
		AppID:      appID,
		UserID:     userID,
		WalletType: walletType,
		ShardID:    shardID,
		Balance:    0,
		Status:     model.WalletStatusActive,
		Version:    0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	conflict := "CONFLICT (app_id) WHERE wallet_type = 'MINT' DO NOTHING"
	if walletType == model.WalletTypeApp {
		conflict = "CONFLICT (app_id, shard_id) WHERE wallet_type = 'APP' DO NOTHING"
	}
	if _, err := tx.NewInsert().Model(&w).On(conflict).Exec(ctx); err != nil {
		return fmt.Errorf("ensure %s wallet shard=%d: %w", walletType, shardID, err)
	}
	return nil
}

func lazyCreateUserWallet(ctx context.Context, tx bun.Tx, appID, userID string) error {
	now := time.Now().UTC()
	w := model.Wallet{
		ID:         uuid.New(),
		AppID:      appID,
		UserID:     userID,
		WalletType: model.WalletTypeUser,
		ShardID:    0,
		Balance:    0,
		Status:     model.WalletStatusActive,
		Version:    0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err := tx.NewInsert().
		Model(&w).
		On("CONFLICT (app_id, user_id) WHERE wallet_type = 'USER' DO NOTHING").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("lazy create wallet: %w", err)
	}
	return nil
}

func appShardIndex(userID string, shards int) int16 {
	if shards <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(userID))
	return int16(h.Sum32() % uint32(shards))
}

func splitAmount(total int64, shards int) []int64 {
	out := make([]int64, shards)
	if shards <= 0 || total <= 0 {
		return out
	}
	base := total / int64(shards)
	rem := total % int64(shards)
	for i := 0; i < shards; i++ {
		out[i] = base
		if int64(i) < rem {
			out[i]++
		}
	}
	return out
}

func getUserWallet(ctx context.Context, tx bun.Tx, appID, userID string) (model.Wallet, error) {
	var w model.Wallet
	err := tx.NewSelect().
		Model(&w).
		Where("app_id = ? AND user_id = ? AND wallet_type = ?", appID, userID, model.WalletTypeUser).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Wallet{}, ErrWalletNotFound
	}
	if err != nil {
		return model.Wallet{}, fmt.Errorf("get user wallet: %w", err)
	}
	return w, nil
}

func lockUserWallet(ctx context.Context, tx bun.Tx, appID, userID string) (model.Wallet, error) {
	var w model.Wallet
	err := tx.NewRaw(`SELECT `+walletSelectCols+`
		FROM wallets WHERE app_id = ? AND user_id = ? AND wallet_type = ? FOR UPDATE`,
		appID, userID, model.WalletTypeUser).Scan(ctx, &w)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Wallet{}, ErrWalletNotFound
	}
	if err != nil {
		return model.Wallet{}, mapWriteError(err)
	}
	return w, nil
}

func getAppWalletShard(ctx context.Context, tx bun.Tx, appID string, shard int16) (model.Wallet, error) {
	var w model.Wallet
	err := tx.NewSelect().
		Model(&w).
		Where("app_id = ? AND wallet_type = ? AND shard_id = ?", appID, model.WalletTypeApp, shard).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Wallet{}, ErrWalletNotFound
	}
	if err != nil {
		return model.Wallet{}, err
	}
	return w, nil
}

func getMintWallet(ctx context.Context, tx bun.Tx, appID string) (model.Wallet, error) {
	var w model.Wallet
	err := tx.NewSelect().
		Model(&w).
		Where("app_id = ? AND wallet_type = ?", appID, model.WalletTypeMint).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Wallet{}, ErrWalletNotFound
	}
	if err != nil {
		return model.Wallet{}, err
	}
	return w, nil
}

func listAppWalletShards(ctx context.Context, tx bun.Tx, appID string) ([]model.Wallet, error) {
	var wallets []model.Wallet
	err := tx.NewSelect().
		Model(&wallets).
		Where("app_id = ? AND wallet_type = ?", appID, model.WalletTypeApp).
		Order("shard_id ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list app shards: %w", err)
	}
	return wallets, nil
}

func sumAppPoolBalance(ctx context.Context, tx bun.Tx, appID string) (int64, error) {
	var sum int64
	err := tx.NewRaw(
		`SELECT COALESCE(SUM(balance), 0) FROM wallets WHERE app_id = ? AND wallet_type = ?`,
		appID, model.WalletTypeApp,
	).Scan(ctx, &sum)
	if err != nil {
		return 0, fmt.Errorf("sum app pool: %w", err)
	}
	return sum, nil
}

func lockPairByID(ctx context.Context, tx bun.Tx, a, b model.Wallet) (model.Wallet, model.Wallet, error) {
	locked, err := lockWalletsByID(ctx, tx, []model.Wallet{a, b})
	if err != nil {
		return model.Wallet{}, model.Wallet{}, err
	}
	return locked[a.ID], locked[b.ID], nil
}

func lockWalletsByID(ctx context.Context, tx bun.Tx, wallets []model.Wallet) (map[uuid.UUID]model.Wallet, error) {
	sorted := append([]model.Wallet(nil), wallets...)
	sort.Slice(sorted, func(i, j int) bool {
		return bytesLessUUID(sorted[i].ID, sorted[j].ID)
	})
	out := make(map[uuid.UUID]model.Wallet, len(sorted))
	for _, w := range sorted {
		var locked model.Wallet
		err := tx.NewRaw(`SELECT `+walletSelectCols+`
			FROM wallets WHERE id = ? FOR UPDATE`, w.ID).Scan(ctx, &locked)
		if err != nil {
			return nil, mapWriteError(err)
		}
		out[locked.ID] = locked
	}
	return out, nil
}

func bytesLessUUID(a, b uuid.UUID) bool {
	for i := 0; i < len(a); i++ {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return false
}

func lookupExistingPair(
	ctx context.Context,
	tx bun.Tx,
	primaryWalletID uuid.UUID,
	idempotencyKey, reason, orderID, requestHash string,
) (bool, model.LedgerEntry, model.LedgerEntry, error) {
	var byKey model.LedgerEntry
	err := tx.NewSelect().
		Model(&byKey).
		Where("wallet_id = ? AND idempotency_key = ?", primaryWalletID, idempotencyKey).
		Scan(ctx)
	if err == nil {
		if byKey.RequestHash != requestHash {
			return false, model.LedgerEntry{}, model.LedgerEntry{}, ErrKeyPayloadMismatch
		}
		contra, err := loadContra(ctx, tx, byKey)
		if err != nil {
			return false, model.LedgerEntry{}, model.LedgerEntry{}, err
		}
		return true, byKey, contra, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, model.LedgerEntry{}, model.LedgerEntry{}, fmt.Errorf("lookup idempotency: %w", err)
	}

	var byOrder model.LedgerEntry
	err = tx.NewSelect().
		Model(&byOrder).
		Where("wallet_id = ? AND reason = ? AND order_id = ?", primaryWalletID, reason, orderID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, model.LedgerEntry{}, model.LedgerEntry{}, nil
	}
	if err != nil {
		return false, model.LedgerEntry{}, model.LedgerEntry{}, fmt.Errorf("lookup order: %w", err)
	}
	if byOrder.RequestHash != requestHash {
		return false, model.LedgerEntry{}, model.LedgerEntry{}, ErrKeyPayloadMismatch
	}
	contra, err := loadContra(ctx, tx, byOrder)
	if err != nil {
		return false, model.LedgerEntry{}, model.LedgerEntry{}, err
	}
	return true, byOrder, contra, nil
}

func loadContra(ctx context.Context, tx bun.Tx, primary model.LedgerEntry) (model.LedgerEntry, error) {
	var contra model.LedgerEntry
	err := tx.NewSelect().
		Model(&contra).
		Where("transaction_id = ? AND id <> ?", primary.TransactionID, primary.ID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return model.LedgerEntry{}, fmt.Errorf("missing contra ledger for transaction %s", primary.TransactionID)
	}
	if err != nil {
		return model.LedgerEntry{}, fmt.Errorf("load contra: %w", err)
	}
	return contra, nil
}

func checkStatus(w model.Wallet) error {
	switch w.Status {
	case model.WalletStatusActive:
		return nil
	case model.WalletStatusFrozen:
		return ErrWalletFrozen
	case model.WalletStatusClosed:
		return ErrWalletClosed
	default:
		return ErrWalletClosed
	}
}

func dailyCreditSum(ctx context.Context, tx bun.Tx, walletID uuid.UUID) (int64, error) {
	start := time.Now().UTC().Truncate(24 * time.Hour)
	var sum int64
	err := tx.NewRaw(
		`SELECT COALESCE(SUM(amount), 0) FROM ledger_entries
		 WHERE wallet_id = ? AND entry_type = 'CREDIT' AND created_at >= ?`,
		walletID, start,
	).Scan(ctx, &sum)
	if err != nil {
		return 0, fmt.Errorf("daily credit sum: %w", err)
	}
	return sum, nil
}

func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "55P03":
			return ErrLockTimeout
		case "40P01", "40001":
			return ErrLockTimeout
		case "23505":
			return ErrDuplicateIdempotency
		}
	}
	return err
}

func mapTxError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrWalletNotFound),
		errors.Is(err, ErrWalletExists),
		errors.Is(err, ErrKeyPayloadMismatch),
		errors.Is(err, ErrInsufficientFunds),
		errors.Is(err, ErrWalletFrozen),
		errors.Is(err, ErrWalletClosed),
		errors.Is(err, ErrLimitExceeded),
		errors.Is(err, ErrLockTimeout),
		errors.Is(err, ErrDuplicateIdempotency):
		return err
	default:
		return mapWriteError(err)
	}
}
