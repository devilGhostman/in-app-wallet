package repository

import "errors"

var (
	ErrOrderNotFound        = errors.New("order not found")
	ErrDuplicateIdempotency = errors.New("duplicate idempotency")
	ErrOrderNotPending      = errors.New("order is not pending")
	ErrOrderTypeMismatch    = errors.New("order type mismatch")
	ErrOrderBadTransition   = errors.New("order status transition not allowed")
	ErrInvalidSignature     = errors.New("invalid payment signature")
)
