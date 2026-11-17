package messaging

import (
	"encoding/json"
	"fmt"
	"strings"

	"wallet-svc/internal/dto"
	"wallet-svc/internal/model"
)

// WalletMutationCommand is the Orders → wallet credit/debit AMQP message body.
type WalletMutationCommand struct {
	AppID          string         `json:"app_id" example:"my-ott-app"`
	UserID         string         `json:"user_id" example:"aaaaaaaaaaaaaaaaaaaaaaaa"`
	Amount         int64          `json:"amount" example:"100"`
	Reason         string         `json:"reason" example:"TOKEN_PURCHASE" enums:"TOKEN_PURCHASE,SPEND"`
	OrderID        string         `json:"order_id" example:"ord-1"`
	IdempotencyKey string         `json:"idempotency_key" example:"order:ord-1:credit"`
	RequestID      string         `json:"request_id,omitempty" example:"corr-optional"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type MutationOp string

const (
	OpCredit MutationOp = "credit"
	OpDebit  MutationOp = "debit"
)

func ParseCommand(data []byte) (WalletMutationCommand, error) {
	var cmd WalletMutationCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		return WalletMutationCommand{}, fmt.Errorf("invalid json: %w", err)
	}
	return cmd, nil
}

// OpFromReason maps command reason to Credit or Debit.
func OpFromReason(reason string) (MutationOp, error) {
	switch reason {
	case model.ReasonTokenPurchase:
		return OpCredit, nil
	case model.ReasonSpend:
		return OpDebit, nil
	default:
		return "", fmt.Errorf("reason must be TOKEN_PURCHASE or SPEND")
	}
}

func (c WalletMutationCommand) Validate() error {
	if strings.TrimSpace(c.AppID) == "" {
		return fmt.Errorf("app_id is required")
	}
	if strings.TrimSpace(c.UserID) == "" {
		return fmt.Errorf("user_id is required")
	}
	if c.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}
	if strings.TrimSpace(c.OrderID) == "" {
		return fmt.Errorf("order_id is required")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	if _, err := OpFromReason(c.Reason); err != nil {
		return err
	}
	return nil
}

func (c WalletMutationCommand) ToMutationRequest() dto.MutationRequest {
	return dto.MutationRequest{
		Amount:         c.Amount,
		Reason:         c.Reason,
		OrderID:        c.OrderID,
		IdempotencyKey: c.IdempotencyKey,
		Metadata:       c.Metadata,
	}
}

func (c WalletMutationCommand) CorrelationID() string {
	if id := strings.TrimSpace(c.RequestID); id != "" {
		return id
	}
	return c.IdempotencyKey
}
