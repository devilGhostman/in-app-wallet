package repository_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"wallet-svc/internal/config"
	"wallet-svc/internal/database"
	"wallet-svc/internal/model"
	"wallet-svc/internal/repository"
	"wallet-svc/internal/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const testAppID = "test-app"

func testLimits() config.LimitsConfig {
	return config.LimitsConfig{
		MaxCreditAmount:  10_000,
		MaxDebitAmount:   10_000,
		MaxWalletBalance: 100_000,
		DailyCreditCap:   50_000,
	}
}

func openTestDB(t *testing.T) *database.PostgresDB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("MIGRATE_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL or MIGRATE_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	db, err := database.NewPostgresDB(ctx, config.DBConfig{
		URL:             dsn,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
	}, zap.NewNop())
	require.NoError(t, err)
	require.NoError(t, database.MigrateUp(ctx, db, zap.NewNop()))
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func fundApp(t *testing.T, repo repository.WalletRepository, appID string, amount int64) {
	t.Helper()
	order := uuid.NewString()
	_, err := repo.FundApp(context.Background(), repository.FundRequest{
		AppID:          appID,
		Amount:         amount,
		IdempotencyKey: "order:" + order + ":fund",
		OrderID:        order,
		RequestHash:    services.RequestHash(appID, "", model.EntryCredit, model.ReasonAppFund, amount, order),
		LockTimeout:    2 * time.Second,
		StmtTimeout:    5 * time.Second,
	})
	require.NoError(t, err)
}

func applyCredit(t *testing.T, repo repository.WalletRepository, appID, userID, key, ref string, amount int64) repository.ApplyResult {
	t.Helper()
	res, err := repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryCredit,
		Reason:         model.ReasonTokenPurchase,
		Amount:         amount,
		IdempotencyKey: key,
		OrderID:        ref,
		RequestHash:    services.RequestHash(appID, userID, model.EntryCredit, model.ReasonTokenPurchase, amount, ref),
		Limits:         testLimits(),
		LockTimeout:    2 * time.Second,
		StmtTimeout:    5 * time.Second,
	})
	require.NoError(t, err)
	return res
}

func applyDebit(t *testing.T, repo repository.WalletRepository, appID, userID, key, ref string, amount int64) repository.ApplyResult {
	t.Helper()
	res, err := repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryDebit,
		Reason:         model.ReasonSpend,
		Amount:         amount,
		IdempotencyKey: key,
		OrderID:        ref,
		RequestHash:    services.RequestHash(appID, userID, model.EntryDebit, model.ReasonSpend, amount, ref),
		Limits:         testLimits(),
		LockTimeout:    2 * time.Second,
		StmtTimeout:    5 * time.Second,
	})
	require.NoError(t, err)
	return res
}

func assertNoDrift(t *testing.T, repo repository.WalletRepository) {
	t.Helper()
	drifts, err := repo.ReconcileBalances(context.Background())
	require.NoError(t, err)
	require.Empty(t, drifts)
}

func assertTxBalanced(t *testing.T, db *database.PostgresDB, txID uuid.UUID) {
	t.Helper()
	var legs []model.LedgerEntry
	err := db.DB.NewSelect().Model(&legs).Where("transaction_id = ?", txID).Scan(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(legs), 2)

	var signed int64
	for _, le := range legs {
		switch le.EntryType {
		case model.EntryCredit:
			signed += le.Amount
		case model.EntryDebit:
			signed -= le.Amount
		default:
			t.Fatalf("unexpected entry_type %q", le.EntryType)
		}
	}
	require.Zero(t, signed, "transaction %s legs must sum to 0", txID)
}

func assertWalletMatchesLedger(t *testing.T, repo repository.WalletRepository, appID, userID string) {
	t.Helper()
	w, err := repo.GetWallet(context.Background(), appID, userID)
	require.NoError(t, err)
	page, err := repo.ListLedger(context.Background(), appID, userID, repository.LedgerListParams{Limit: 500})
	require.NoError(t, err)
	var signed int64
	for _, item := range page.Items {
		switch item.EntryType {
		case model.EntryCredit:
			signed += item.Amount
		case model.EntryDebit:
			signed -= item.Amount
		}
	}
	require.Equal(t, w.Balance, signed)
}

func TestCreditReplayAndMismatch(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-replay"
	fundApp(t, repo, appID, 10_000)
	userID := fmt.Sprintf("%024x", time.Now().UnixNano())[:24]
	order := uuid.NewString()

	first := applyCredit(t, repo, appID, userID, "order:"+order+":credit", order, 100)
	require.False(t, first.Duplicate)
	require.Equal(t, int64(100), first.Entry.BalanceAfter)
	require.Equal(t, model.EntryCredit, first.Entry.EntryType)
	require.Equal(t, model.EntryDebit, first.ContraEntry.EntryType)
	require.Equal(t, first.TransactionID, first.ContraEntry.TransactionID)

	second := applyCredit(t, repo, appID, userID, "order:"+order+":credit", order, 100)
	require.True(t, second.Duplicate)
	require.Equal(t, first.Entry.ID, second.Entry.ID)
	require.Equal(t, first.Entry.BalanceAfter, second.Entry.BalanceAfter)

	_, err := repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryCredit,
		Reason:         model.ReasonTokenPurchase,
		Amount:         50,
		IdempotencyKey: "order:" + order + ":credit",
		OrderID:        order,
		RequestHash:    services.RequestHash(appID, userID, model.EntryCredit, model.ReasonTokenPurchase, 50, order),
		Limits:         testLimits(),
	})
	require.ErrorIs(t, err, repository.ErrKeyPayloadMismatch)

	_, err = repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryCredit,
		Reason:         model.ReasonTokenPurchase,
		Amount:         100,
		IdempotencyKey: "order:" + order + "-retry:credit",
		OrderID:        order,
		RequestHash:    services.RequestHash(appID, userID, model.EntryCredit, model.ReasonTokenPurchase, 99, order),
		Limits:         testLimits(),
	})
	require.ErrorIs(t, err, repository.ErrKeyPayloadMismatch)

	replay := applyCredit(t, repo, appID, userID, "order:"+order+"-retry:credit", order, 100)
	require.True(t, replay.Duplicate)
}

func TestInsufficientAppPoolOnPurchase(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-empty-pool"
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+11)[:24]
	order := uuid.NewString()

	_, err := repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryCredit,
		Reason:         model.ReasonTokenPurchase,
		Amount:         10,
		IdempotencyKey: "order:" + order + ":credit",
		OrderID:        order,
		RequestHash:    services.RequestHash(appID, userID, model.EntryCredit, model.ReasonTokenPurchase, 10, order),
		Limits:         testLimits(),
	})
	require.ErrorIs(t, err, repository.ErrInsufficientFunds)
}

func TestCrossAppIsolation(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appA := testAppID + "-iso-a"
	appB := testAppID + "-iso-b"
	fundApp(t, repo, appA, 500)
	fundApp(t, repo, appB, 500)
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+12)[:24]

	orderA := uuid.NewString()
	applyCredit(t, repo, appA, userID, "order:"+orderA+":credit", orderA, 40)
	orderB := uuid.NewString()
	applyCredit(t, repo, appB, userID, "order:"+orderB+":credit", orderB, 15)

	wa, err := repo.GetWallet(context.Background(), appA, userID)
	require.NoError(t, err)
	require.Equal(t, int64(40), wa.Balance)

	wb, err := repo.GetWallet(context.Background(), appB, userID)
	require.NoError(t, err)
	require.Equal(t, int64(15), wb.Balance)
}

func TestParallelCreditsOneRow(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-parallel-credit"
	fundApp(t, repo, appID, 10_000)
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+1)[:24]
	order := uuid.NewString()

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.Apply(context.Background(), repository.ApplyRequest{
				AppID:          appID,
				UserID:         userID,
				EntryType:      model.EntryCredit,
				Reason:         model.ReasonTokenPurchase,
				Amount:         25,
				IdempotencyKey: "order:" + order + ":credit",
				OrderID:        order,
				RequestHash:    services.RequestHash(appID, userID, model.EntryCredit, model.ReasonTokenPurchase, 25, order),
				Limits:         testLimits(),
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	w, err := repo.GetWallet(context.Background(), appID, userID)
	require.NoError(t, err)
	require.Equal(t, int64(25), w.Balance)

	page, err := repo.ListLedger(context.Background(), appID, userID, repository.LedgerListParams{Limit: 20})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
}

func TestParallelDebitNeverOverdraws(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-parallel-debit"
	fundApp(t, repo, appID, 10_000)
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+2)[:24]
	creditOrder := uuid.NewString()
	applyCredit(t, repo, appID, userID, "order:"+creditOrder+":credit", creditOrder, 50)

	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			order := fmt.Sprintf("spend-%d-%s", i, uuid.NewString())
			_, err := repo.Apply(context.Background(), repository.ApplyRequest{
				AppID:          appID,
				UserID:         userID,
				EntryType:      model.EntryDebit,
				Reason:         model.ReasonSpend,
				Amount:         20,
				IdempotencyKey: "order:" + order + ":debit",
				OrderID:        order,
				RequestHash:    services.RequestHash(appID, userID, model.EntryDebit, model.ReasonSpend, 20, order),
				Limits:         testLimits(),
			})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)

	var ok, insufficient int
	for err := range results {
		if err == nil {
			ok++
			continue
		}
		require.ErrorIs(t, err, repository.ErrInsufficientFunds)
		insufficient++
	}
	require.Equal(t, 2, ok)
	require.Equal(t, 8, insufficient)

	w, err := repo.GetWallet(context.Background(), appID, userID)
	require.NoError(t, err)
	require.Equal(t, int64(10), w.Balance)
}

func TestFrozenWalletMatrix(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-frozen"
	fundApp(t, repo, appID, 10_000)
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+3)[:24]
	order := uuid.NewString()
	applyCredit(t, repo, appID, userID, "order:"+order+":credit", order, 40)

	_, err := repo.SetStatus(context.Background(), appID, userID, "freeze", "fraud", "req", model.WalletStatusFrozen)
	require.NoError(t, err)

	spend := uuid.NewString()
	_, err = repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryDebit,
		Reason:         model.ReasonSpend,
		Amount:         10,
		IdempotencyKey: "order:" + spend + ":debit",
		OrderID:        spend,
		RequestHash:    services.RequestHash(appID, userID, model.EntryDebit, model.ReasonSpend, 10, spend),
		Limits:         testLimits(),
	})
	require.ErrorIs(t, err, repository.ErrWalletFrozen)

	paid := uuid.NewString()
	_, err = repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryCredit,
		Reason:         model.ReasonTokenPurchase,
		Amount:         15,
		IdempotencyKey: "order:" + paid + ":credit",
		OrderID:        paid,
		RequestHash:    services.RequestHash(appID, userID, model.EntryCredit, model.ReasonTokenPurchase, 15, paid),
		Limits:         testLimits(),
	})
	require.ErrorIs(t, err, repository.ErrWalletFrozen)

	replay := applyCredit(t, repo, appID, userID, "order:"+order+":credit", order, 40)
	require.True(t, replay.Duplicate)
	require.Equal(t, int64(40), replay.Entry.BalanceAfter)
}

func TestClosedWalletRejectsMutations(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-closed"
	fundApp(t, repo, appID, 10_000)
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+7)[:24]
	order := uuid.NewString()
	applyCredit(t, repo, appID, userID, "order:"+order+":credit", order, 40)

	_, err := repo.SetStatus(context.Background(), appID, userID, "close", "account_deleted", "req", model.WalletStatusClosed)
	require.NoError(t, err)

	spend := uuid.NewString()
	_, err = repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryDebit,
		Reason:         model.ReasonSpend,
		Amount:         10,
		IdempotencyKey: "order:" + spend + ":debit",
		OrderID:        spend,
		RequestHash:    services.RequestHash(appID, userID, model.EntryDebit, model.ReasonSpend, 10, spend),
		Limits:         testLimits(),
	})
	require.ErrorIs(t, err, repository.ErrWalletClosed)

	paid := uuid.NewString()
	_, err = repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryCredit,
		Reason:         model.ReasonTokenPurchase,
		Amount:         15,
		IdempotencyKey: "order:" + paid + ":credit",
		OrderID:        paid,
		RequestHash:    services.RequestHash(appID, userID, model.EntryCredit, model.ReasonTokenPurchase, 15, paid),
		Limits:         testLimits(),
	})
	require.ErrorIs(t, err, repository.ErrWalletClosed)

	replay := applyCredit(t, repo, appID, userID, "order:"+order+":credit", order, 40)
	require.True(t, replay.Duplicate)
	require.Equal(t, int64(40), replay.Entry.BalanceAfter)
}

func TestCrashBeforeAndAfterCommit(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-crash"
	fundApp(t, repo, appID, 10_000)
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+4)[:24]
	order := uuid.NewString()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := repo.Apply(ctx, repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryCredit,
		Reason:         model.ReasonTokenPurchase,
		Amount:         30,
		IdempotencyKey: "order:" + order + ":credit",
		OrderID:        order,
		RequestHash:    services.RequestHash(appID, userID, model.EntryCredit, model.ReasonTokenPurchase, 30, order),
		Limits:         testLimits(),
	})
	require.Error(t, err)

	_, err = repo.GetWallet(context.Background(), appID, userID)
	require.ErrorIs(t, err, repository.ErrWalletNotFound)

	first := applyCredit(t, repo, appID, userID, "order:"+order+":credit", order, 30)
	require.False(t, first.Duplicate)
	second := applyCredit(t, repo, appID, userID, "order:"+order+":credit", order, 30)
	require.True(t, second.Duplicate)
	require.Equal(t, first.Entry.ID, second.Entry.ID)
}

func TestDebitMissingWallet(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-missing"
	fundApp(t, repo, appID, 100)
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+5)[:24]
	order := uuid.NewString()
	_, err := repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryDebit,
		Reason:         model.ReasonSpend,
		Amount:         5,
		IdempotencyKey: "order:" + order + ":debit",
		OrderID:        order,
		RequestHash:    services.RequestHash(appID, userID, model.EntryDebit, model.ReasonSpend, 5, order),
		Limits:         testLimits(),
	})
	require.ErrorIs(t, err, repository.ErrInsufficientFunds)
}

func TestAppRoleCannotMutateLedger(t *testing.T) {
	dsn := os.Getenv("TEST_APP_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_APP_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := database.NewPostgresDB(ctx, config.DBConfig{URL: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute}, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.DB.NewUpdate().Table("ledger_entries").Set("amount = amount").Where("true").Exec(ctx)
	require.Error(t, err)
	_, err = db.DB.ExecContext(ctx, "DELETE FROM ledger_entries")
	require.Error(t, err)
	_ = sql.ErrNoRows
}

func TestLazyCreateRace(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-lazy"
	fundApp(t, repo, appID, 10_000)
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+6)[:24]

	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			order := fmt.Sprintf("lazy-%d-%s", i, uuid.NewString())
			_, err := repo.Apply(context.Background(), repository.ApplyRequest{
				AppID:          appID,
				UserID:         userID,
				EntryType:      model.EntryCredit,
				Reason:         model.ReasonTokenPurchase,
				Amount:         1,
				IdempotencyKey: "order:" + order + ":credit",
				OrderID:        order,
				RequestHash:    services.RequestHash(appID, userID, model.EntryCredit, model.ReasonTokenPurchase, 1, order),
				Limits:         testLimits(),
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	w, err := repo.GetWallet(context.Background(), appID, userID)
	require.NoError(t, err)
	require.Equal(t, int64(6), w.Balance)

	drifts, err := repo.ReconcileBalances(context.Background())
	require.NoError(t, err)
	require.Empty(t, drifts)
}

func TestFundAppIdempotent(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-fund-" + uuid.NewString()[:8]
	order := uuid.NewString()
	req := repository.FundRequest{
		AppID:          appID,
		Amount:         250,
		IdempotencyKey: "order:" + order + ":fund",
		OrderID:        order,
		RequestHash:    services.RequestHash(appID, "", model.EntryCredit, model.ReasonAppFund, 250, order),
		LockTimeout:    2 * time.Second,
		StmtTimeout:    5 * time.Second,
	}
	first, err := repo.FundApp(context.Background(), req)
	require.NoError(t, err)
	require.False(t, first.Duplicate)
	require.Equal(t, model.EntryDebit, first.Entry.EntryType)
	require.Equal(t, model.EntryCredit, first.ContraEntry.EntryType)
	require.Equal(t, int64(250), first.Wallet.Balance)

	second, err := repo.FundApp(context.Background(), req)
	require.NoError(t, err)
	require.True(t, second.Duplicate)
	require.Equal(t, first.Entry.ID, second.Entry.ID)
	require.Equal(t, first.Wallet.Balance, second.Wallet.Balance)
}

func TestAppShardStickyRouting(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-shard-route"
	fundApp(t, repo, appID, 10_000)

	userA := fmt.Sprintf("%024x", time.Now().UnixNano()+21)[:24]
	userB := fmt.Sprintf("%024x", time.Now().UnixNano()+22)[:24]
	orderA := uuid.NewString()
	orderB := uuid.NewString()
	a := applyCredit(t, repo, appID, userA, "order:"+orderA+":credit", orderA, 10)
	b := applyCredit(t, repo, appID, userB, "order:"+orderB+":credit", orderB, 10)
	require.NotEqual(t, uuid.Nil, a.ContraEntry.WalletID)
	require.NotEqual(t, uuid.Nil, b.ContraEntry.WalletID)
}

func TestLedgerInvariantsAfterCreditAndDebit(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-inv-" + uuid.NewString()[:8]
	fundApp(t, repo, appID, 10_000)
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+31)[:24]

	creditOrder := uuid.NewString()
	credit := applyCredit(t, repo, appID, userID, "order:"+creditOrder+":credit", creditOrder, 100)
	assertTxBalanced(t, db, credit.TransactionID)
	assertWalletMatchesLedger(t, repo, appID, userID)
	assertNoDrift(t, repo)

	spendOrder := uuid.NewString()
	debit := applyDebit(t, repo, appID, userID, "order:"+spendOrder+":debit", spendOrder, 40)
	assertTxBalanced(t, db, debit.TransactionID)
	assertWalletMatchesLedger(t, repo, appID, userID)
	assertNoDrift(t, repo)

	w, err := repo.GetWallet(context.Background(), appID, userID)
	require.NoError(t, err)
	require.Equal(t, int64(60), w.Balance)
}

func TestFundAppMultiLegBalanced(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-fund-legs-" + uuid.NewString()[:8]
	order := uuid.NewString()
	amount := int64(250)

	res, err := repo.FundApp(context.Background(), repository.FundRequest{
		AppID:          appID,
		Amount:         amount,
		IdempotencyKey: "order:" + order + ":fund",
		OrderID:        order,
		RequestHash:    services.RequestHash(appID, "", model.EntryCredit, model.ReasonAppFund, amount, order),
		LockTimeout:    2 * time.Second,
		StmtTimeout:    5 * time.Second,
	})
	require.NoError(t, err)
	require.False(t, res.Duplicate)

	assertTxBalanced(t, db, res.TransactionID)

	legs, err := repo.ListByOrder(context.Background(), appID, order)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(legs), 2)

	var mintDebit, appCredits int64
	for _, le := range legs {
		switch {
		case le.EntryType == model.EntryDebit:
			mintDebit += le.Amount
		case le.EntryType == model.EntryCredit:
			appCredits += le.Amount
		}
	}
	require.Equal(t, amount, mintDebit)
	require.Equal(t, amount, appCredits)
	assertNoDrift(t, repo)
}

func TestFailedMutationsLeaveNoPartialLegs(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-fail-clean-" + uuid.NewString()[:8]
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+32)[:24]

	emptyOrder := uuid.NewString()
	_, err := repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      model.EntryCredit,
		Reason:         model.ReasonTokenPurchase,
		Amount:         10,
		IdempotencyKey: "order:" + emptyOrder + ":credit",
		OrderID:        emptyOrder,
		RequestHash:    services.RequestHash(appID, userID, model.EntryCredit, model.ReasonTokenPurchase, 10, emptyOrder),
		Limits:         testLimits(),
	})
	require.ErrorIs(t, err, repository.ErrInsufficientFunds)
	legs, err := repo.ListByOrder(context.Background(), appID, emptyOrder)
	require.NoError(t, err)
	require.Empty(t, legs)

	fundApp(t, repo, appID, 100)
	missingUser := fmt.Sprintf("%024x", time.Now().UnixNano()+33)[:24]
	spendOrder := uuid.NewString()
	_, err = repo.Apply(context.Background(), repository.ApplyRequest{
		AppID:          appID,
		UserID:         missingUser,
		EntryType:      model.EntryDebit,
		Reason:         model.ReasonSpend,
		Amount:         5,
		IdempotencyKey: "order:" + spendOrder + ":debit",
		OrderID:        spendOrder,
		RequestHash:    services.RequestHash(appID, missingUser, model.EntryDebit, model.ReasonSpend, 5, spendOrder),
		Limits:         testLimits(),
	})
	require.ErrorIs(t, err, repository.ErrInsufficientFunds)
	legs, err = repo.ListByOrder(context.Background(), appID, spendOrder)
	require.NoError(t, err)
	require.Empty(t, legs)
	assertNoDrift(t, repo)
}

func TestParallelSameKeyDebit(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewWalletRepository(db.DB, 4)
	appID := testAppID + "-parallel-same-debit"
	fundApp(t, repo, appID, 10_000)
	userID := fmt.Sprintf("%024x", time.Now().UnixNano()+34)[:24]
	creditOrder := uuid.NewString()
	applyCredit(t, repo, appID, userID, "order:"+creditOrder+":credit", creditOrder, 50)

	spendOrder := uuid.NewString()
	key := "order:" + spendOrder + ":debit"
	hash := services.RequestHash(appID, userID, model.EntryDebit, model.ReasonSpend, 30, spendOrder)

	var wg sync.WaitGroup
	results := make(chan repository.ApplyResult, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := repo.Apply(context.Background(), repository.ApplyRequest{
				AppID:          appID,
				UserID:         userID,
				EntryType:      model.EntryDebit,
				Reason:         model.ReasonSpend,
				Amount:         30,
				IdempotencyKey: key,
				OrderID:        spendOrder,
				RequestHash:    hash,
				Limits:         testLimits(),
				LockTimeout:    2 * time.Second,
				StmtTimeout:    5 * time.Second,
			})
			if err != nil {
				errs <- err
				return
			}
			results <- res
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var firstID uuid.UUID
	var duplicates int
	for res := range results {
		if firstID == uuid.Nil {
			firstID = res.Entry.ID
		}
		require.Equal(t, firstID, res.Entry.ID)
		if res.Duplicate {
			duplicates++
		}
	}
	require.Equal(t, 7, duplicates)

	w, err := repo.GetWallet(context.Background(), appID, userID)
	require.NoError(t, err)
	require.Equal(t, int64(20), w.Balance)
	assertWalletMatchesLedger(t, repo, appID, userID)
	assertNoDrift(t, repo)
}
