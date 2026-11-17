package messaging

import (
	"context"

	"wallet-svc/internal/dto"
	"wallet-svc/internal/services"
)

// MutationHandler processes a validated credit/debit command.
type MutationHandler interface {
	Handle(ctx context.Context, cmd WalletMutationCommand) (dto.MutationResponse, error)
}

type walletMutationHandler struct {
	svc *services.WalletService
}

func NewWalletMutationHandler(svc *services.WalletService) MutationHandler {
	return &walletMutationHandler{svc: svc}
}

func (h *walletMutationHandler) Handle(ctx context.Context, cmd WalletMutationCommand) (dto.MutationResponse, error) {
	op, err := OpFromReason(cmd.Reason)
	if err != nil {
		return dto.MutationResponse{}, err
	}
	req := cmd.ToMutationRequest()
	requestID := cmd.CorrelationID()
	switch op {
	case OpCredit:
		out, _, err := h.svc.Credit(ctx, cmd.AppID, cmd.UserID, requestID, req)
		return out, err
	case OpDebit:
		out, _, err := h.svc.Debit(ctx, cmd.AppID, cmd.UserID, requestID, req)
		return out, err
	default:
		return dto.MutationResponse{}, nil
	}
}
