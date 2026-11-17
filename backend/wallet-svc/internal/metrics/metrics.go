package metrics

import "sync/atomic"

type Counters struct {
	Credits            atomic.Int64
	Debits             atomic.Int64
	Replays            atomic.Int64
	KeyPayloadMismatch atomic.Int64
	InsufficientFunds  atomic.Int64
	LockTimeouts       atomic.Int64
	LimitExceeded      atomic.Int64
	LedgerWriteErrors  atomic.Int64
}

func (c *Counters) Snapshot() map[string]int64 {
	return map[string]int64{
		"credits":              c.Credits.Load(),
		"debits":               c.Debits.Load(),
		"replays":              c.Replays.Load(),
		"key_payload_mismatch": c.KeyPayloadMismatch.Load(),
		"insufficient_funds":   c.InsufficientFunds.Load(),
		"lock_timeouts":        c.LockTimeouts.Load(),
		"limit_exceeded":       c.LimitExceeded.Load(),
		"ledger_write_errors":  c.LedgerWriteErrors.Load(),
	}
}
