package proxy

import (
	"errors"
	"testing"
	"time"

	"gateway-svc/internal/config"
)

func TestBreaker_OpensAfterConsecutiveFailures(t *testing.T) {
	r := NewBreakerRegistry(config.BreakerConfig{
		Failures:    2,
		OpenTimeout: time.Minute,
	}, nil, UpstreamWallet)

	for i := 0; i < 2; i++ {
		done, err := r.Allow(UpstreamWallet)
		if err != nil {
			t.Fatalf("allow #%d: %v", i+1, err)
		}
		done(false)
	}

	_, err := r.Allow(UpstreamWallet)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("want ErrCircuitOpen after trip, got %v", err)
	}
}

func TestBreaker_HalfOpenProbeSuccessCloses(t *testing.T) {
	r := NewBreakerRegistry(config.BreakerConfig{
		Failures:    1,
		OpenTimeout: 50 * time.Millisecond,
	}, nil, UpstreamOrder)

	done, err := r.Allow(UpstreamOrder)
	if err != nil {
		t.Fatal(err)
	}
	done(false)

	_, err = r.Allow(UpstreamOrder)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("want open, got %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	var probeDone func(bool)
	for {
		probeDone, err = r.Allow(UpstreamOrder)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for half-open: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	probeDone(true)

	done, err = r.Allow(UpstreamOrder)
	if err != nil {
		t.Fatalf("want closed after successful probe: %v", err)
	}
	done(true)
}

func TestBreaker_HalfOpenProbeFailureReopens(t *testing.T) {
	r := NewBreakerRegistry(config.BreakerConfig{
		Failures:    1,
		OpenTimeout: 50 * time.Millisecond,
	}, nil, UpstreamWallet)

	done, err := r.Allow(UpstreamWallet)
	if err != nil {
		t.Fatal(err)
	}
	done(false)

	deadline := time.Now().Add(500 * time.Millisecond)
	var probeDone func(bool)
	for {
		probeDone, err = r.Allow(UpstreamWallet)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for half-open: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	probeDone(false)

	_, err = r.Allow(UpstreamWallet)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("want re-open after failed probe, got %v", err)
	}
}

func TestBreaker_UnknownUpstreamAlwaysAllows(t *testing.T) {
	r := NewBreakerRegistry(config.BreakerConfig{
		Failures:    1,
		OpenTimeout: time.Minute,
	}, nil, UpstreamWallet)
	done, err := r.Allow("unknown")
	if err != nil {
		t.Fatal(err)
	}
	done(false)
	done2, err := r.Allow("unknown")
	if err != nil {
		t.Fatal(err)
	}
	done2(false)
}
