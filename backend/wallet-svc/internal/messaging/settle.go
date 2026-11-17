package messaging

import (
	"errors"
	"net/http"

	"wallet-svc/pkg/response"
)

// SettleAction tells the consumer how to disposition an AMQP delivery.
type SettleAction int

const (
	SettleAccept SettleAction = iota
	SettleRelease
	SettleDeadLetter
)

// ClassifyProcessError maps WalletService errors to AMQP settle behavior.
// Permanent client/domain errors go to the DLQ (then accept source).
// Transient errors (lock timeout, 5xx) are released for redelivery.
func ClassifyProcessError(err error) SettleAction {
	if err == nil {
		return SettleAccept
	}
	var appErr *response.AppError
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case response.CodeLockTimeout, response.CodeServiceUnavailable:
			return SettleRelease
		}
		if appErr.HTTPStatus >= http.StatusInternalServerError {
			return SettleRelease
		}
		return SettleDeadLetter
	}
	return SettleRelease
}
