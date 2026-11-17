package routes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gateway-svc/internal/config"
	"gateway-svc/internal/proxy"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

const testJWTSecret = "dev-only-jwt-secret-change-me-now!!!!!!"

func mintTestJWT(t *testing.T) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	s, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newTestProxyServer(t *testing.T, upstream http.Handler) *httptest.Server {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)

	cfg := &config.Config{
		App:  config.AppConfig{RoutePrefix: "/api/v1"},
		Auth: config.AuthConfig{Secrets: []string{testJWTSecret}},
		RateLimit: config.RateLimitConfig{
			Capacity: 1000,
			Refill:   time.Second,
		},
		Proxy: config.ProxyConfig{
			WalletUpstream: up.URL,
			OrderUpstream:  up.URL,
			Timeout:        2 * time.Second,
			MaxRetries:     0,
		},
		Breaker: config.BreakerConfig{
			Failures:    50,
			OpenTimeout: time.Minute,
		},
	}
	p, err := proxy.NewRouter(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewProxyRoute(r, cfg, p, nil, zap.NewNop()).Routes()
	// Serve via httptest so ReverseProxy gets a real ResponseWriter (CloseNotifier).
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func TestAuth_PaymentWebhookSkipsJWT(t *testing.T) {
	var hit bool
	srv := newTestProxyServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	resp, err := http.Post(srv.URL+"/api/v1/webhooks/payment", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if !hit {
		t.Fatal("expected upstream hit without JWT")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
}

func TestAuth_OtherPathsRequireJWT(t *testing.T) {
	var hit bool
	srv := newTestProxyServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))

	resp, err := http.Get(srv.URL + "/api/v1/internal/orders/abc")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if hit {
		t.Fatal("upstream must not be dialed without JWT")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", resp.StatusCode, body)
	}
}

func TestAuth_AuthenticatedPathProxies(t *testing.T) {
	var hit bool
	srv := newTestProxyServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/internal/orders/abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+mintTestJWT(t))
	req.Header.Set("X-Scopes", "order:read")
	req.Header.Set("X-App-Id", "6a6899644aef7603d8dd7e1a")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if !hit {
		t.Fatal("expected upstream hit with valid auth")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
}
