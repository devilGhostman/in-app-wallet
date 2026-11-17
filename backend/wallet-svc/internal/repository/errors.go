package repository

import "errors"

var (
	ErrWalletNotFound       = errors.New("wallet not found")
	ErrWalletExists         = errors.New("wallet already exists")
	ErrKeyPayloadMismatch   = errors.New("key payload mismatch")
	ErrInsufficientFunds    = errors.New("insufficient funds")
	ErrWalletFrozen         = errors.New("wallet frozen")
	ErrWalletClosed         = errors.New("wallet closed")
	ErrLimitExceeded        = errors.New("limit exceeded")
	ErrLockTimeout          = errors.New("lock timeout")
	ErrDuplicateIdempotency = errors.New("duplicate idempotency")
)
