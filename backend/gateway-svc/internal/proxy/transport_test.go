package proxy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"syscall"
	"testing"
	"time"

	"gateway-svc/internal/config"
)

type countingRT struct {
	n  int
	fn func(n int, req *http.Request) (*http.Response, error)
}

func (c *countingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	c.n++
	return c.fn(c.n, req)
}

func newTestTransport(t *testing.T, maxRetries int, base http.RoundTripper) *resilientTransport {
	t.Helper()
	breakers := NewBreakerRegistry(config.BreakerConfig{
		Failures:    100,
		OpenTimeout: time.Minute,
	}, nil, UpstreamWallet)
	return &resilientTransport{
		base:       base,
		timeout:    2 * time.Second,
		maxRetries: maxRetries,
		breakers:   breakers,
		upstream:   UpstreamWallet,
	}
}

func jsonBody(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}
}

func TestRoundTrip_POSTDoesNotRetryOn503(t *testing.T) {
	rt := &countingRT{fn: func(n int, _ *http.Request) (*http.Response, error) {
		return jsonBody(http.StatusServiceUnavailable, "down"), nil
	}}
	tr := newTestTransport(t, 2, rt)

	req, err := http.NewRequest(http.MethodPost, "http://wallet.test/api", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if rt.n != 1 {
		t.Fatalf("POST dials=%d want 1", rt.n)
	}
}

func TestRoundTrip_GETRetries503ThenSucceeds(t *testing.T) {
	rt := &countingRT{fn: func(n int, _ *http.Request) (*http.Response, error) {
		if n == 1 {
			return jsonBody(http.StatusServiceUnavailable, "down"), nil
		}
		return jsonBody(http.StatusOK, `{"ok":true}`), nil
	}}
	tr := newTestTransport(t, 2, rt)

	req, err := http.NewRequest(http.MethodGet, "http://wallet.test/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if rt.n != 2 {
		t.Fatalf("GET dials=%d want 2 (maxRetries+1 with early success)", rt.n)
	}
}

func TestRoundTrip_GETRetriesRetryableNetErr(t *testing.T) {
	rt := &countingRT{fn: func(n int, _ *http.Request) (*http.Response, error) {
		if n == 1 {
			return nil, syscall.ECONNREFUSED
		}
		return jsonBody(http.StatusOK, "ok"), nil
	}}
	tr := newTestTransport(t, 2, rt)

	req, err := http.NewRequest(http.MethodGet, "http://wallet.test/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if rt.n != 2 {
		t.Fatalf("dials=%d want 2", rt.n)
	}
}

func TestRoundTrip_GETExhaustsRetriesOn5xx(t *testing.T) {
	rt := &countingRT{fn: func(n int, _ *http.Request) (*http.Response, error) {
		return jsonBody(http.StatusBadGateway, "bad"), nil
	}}
	tr := newTestTransport(t, 2, rt)

	req, err := http.NewRequest(http.MethodGet, "http://wallet.test/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if rt.n != 3 {
		t.Fatalf("dials=%d want maxRetries+1=3", rt.n)
	}
}

func TestRoundTrip_CircuitOpenSkipsDial(t *testing.T) {
	breakers := NewBreakerRegistry(config.BreakerConfig{
		Failures:    1,
		OpenTimeout: time.Minute,
	}, nil, UpstreamWallet)
	done, err := breakers.Allow(UpstreamWallet)
	if err != nil {
		t.Fatal(err)
	}
	done(false)

	rt := &countingRT{fn: func(n int, _ *http.Request) (*http.Response, error) {
		return jsonBody(http.StatusOK, "ok"), nil
	}}
	tr := &resilientTransport{
		base:       rt,
		timeout:    time.Second,
		maxRetries: 2,
		breakers:   breakers,
		upstream:   UpstreamWallet,
	}

	req, err := http.NewRequest(http.MethodGet, "http://wallet.test/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.RoundTrip(req)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("want ErrCircuitOpen, got %v", err)
	}
	if rt.n != 0 {
		t.Fatalf("dials=%d want 0 while circuit open", rt.n)
	}
}
