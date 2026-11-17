package proxy

import (
	"fmt"

	"gateway-svc/internal/config"
	"gateway-svc/internal/metrics"

	"github.com/sony/gobreaker/v2"
)

const (
	circuitClosed   = 0
	circuitOpen     = 1
	circuitHalfOpen = 2
)

type BreakerRegistry struct {
	breakers map[string]*gobreaker.TwoStepCircuitBreaker[any]
	metrics  *metrics.Metrics
}

func NewBreakerRegistry(cfg config.BreakerConfig, m *metrics.Metrics, names ...string) *BreakerRegistry {
	r := &BreakerRegistry{
		breakers: make(map[string]*gobreaker.TwoStepCircuitBreaker[any], len(names)),
		metrics:  m,
	}
	for _, name := range names {
		n := name
		settings := gobreaker.Settings{
			Name:        n,
			MaxRequests: 1,
			Interval:    0,
			Timeout:     cfg.OpenTimeout,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= cfg.Failures
			},
			OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
				if m != nil {
					m.CircuitState.WithLabelValues(name).Set(float64(stateValue(to)))
				}
			},
		}
		r.breakers[n] = gobreaker.NewTwoStepCircuitBreaker[any](settings)
		if m != nil {
			m.CircuitState.WithLabelValues(n).Set(circuitClosed)
		}
	}
	return r
}

// Allow returns a done(success) callback, or ErrCircuitOpen when the breaker is open.
func (r *BreakerRegistry) Allow(name string) (done func(success bool), err error) {
	cb, ok := r.breakers[name]
	if !ok {
		return func(bool) {}, nil
	}
	done, err = cb.Allow()
	if err != nil {
		return nil, fmt.Errorf("%w: upstream %s circuit open", ErrCircuitOpen, name)
	}
	return done, nil
}

func stateValue(s gobreaker.State) int {
	switch s {
	case gobreaker.StateOpen:
		return circuitOpen
	case gobreaker.StateHalfOpen:
		return circuitHalfOpen
	default:
		return circuitClosed
	}
}
