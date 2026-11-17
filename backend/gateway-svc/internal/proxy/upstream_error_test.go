package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gateway-svc/internal/middleware"
	"gateway-svc/pkg/response"
)

type errRT struct {
	err error
}

func (e errRT) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, e.err
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func serveProxyError(t *testing.T, upstreamErr error, reqID string) *httptest.ResponseRecorder {
	t.Helper()
	target, err := url.Parse("http://upstream.test")
	if err != nil {
		t.Fatal(err)
	}
	p := newReverseProxy(target, errRT{err: upstreamErr})
	req := httptest.NewRequest(http.MethodGet, "http://gateway.test/api/v1/internal/wallets/x", nil)
	if reqID != "" {
		req.Header.Set(middleware.HeaderXRequestID, reqID)
	}
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	return rec
}

func decodeErr(t *testing.T, body io.Reader) response.ErrorEnvelope {
	t.Helper()
	var env response.ErrorEnvelope
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return env
}

func TestErrorHandler_CircuitOpen503(t *testing.T) {
	rec := serveProxyError(t, ErrCircuitOpen, "req-circuit")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", rec.Code)
	}
	if got := rec.Header().Get(middleware.HeaderXRequestID); got != "req-circuit" {
		t.Fatalf("X-Request-ID=%q", got)
	}
	env := decodeErr(t, rec.Body)
	if env.Success {
		t.Fatal("expected success=false")
	}
	if env.Code != response.CodeServiceUnavailable {
		t.Fatalf("code=%s", env.Code)
	}
	if env.Message != "Upstream temporarily unavailable" {
		t.Fatalf("message=%q", env.Message)
	}
}

func TestErrorHandler_Timeout504(t *testing.T) {
	rec := serveProxyError(t, timeoutErr{}, "req-timeout")
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d want 504", rec.Code)
	}
	if got := rec.Header().Get(middleware.HeaderXRequestID); got != "req-timeout" {
		t.Fatalf("X-Request-ID=%q", got)
	}
	env := decodeErr(t, rec.Body)
	if env.Message != "Upstream request timed out" {
		t.Fatalf("message=%q", env.Message)
	}
}

func TestErrorHandler_Other502(t *testing.T) {
	rec := serveProxyError(t, errors.New("connection reset"), "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status=%d want 502", rec.Code)
	}
	if got := rec.Header().Get(middleware.HeaderXRequestID); got != "" {
		t.Fatalf("unexpected X-Request-ID=%q", got)
	}
	env := decodeErr(t, rec.Body)
	if env.Message != "Upstream request failed" {
		t.Fatalf("message=%q", env.Message)
	}
	if time.Since(env.Timestamp) > time.Minute {
		t.Fatalf("timestamp skew: %v", env.Timestamp)
	}
}
