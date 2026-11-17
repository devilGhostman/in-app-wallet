package proxy

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"gateway-svc/internal/config"
	"gateway-svc/internal/metrics"
	"gateway-svc/internal/middleware"
	"gateway-svc/pkg/response"

	"github.com/gin-gonic/gin"
)

const (
	UpstreamWallet = "wallet"
	UpstreamOrder  = "order"
)

type routeEntry struct {
	prefix   string
	upstream string
	proxy    *httputil.ReverseProxy
}

type Router struct {
	routes []routeEntry
}

func NewRouter(cfg *config.Config, m *metrics.Metrics) (*Router, error) {
	walletURL, err := url.Parse(cfg.Proxy.WalletUpstream)
	if err != nil {
		return nil, err
	}
	orderURL, err := url.Parse(cfg.Proxy.OrderUpstream)
	if err != nil {
		return nil, err
	}

	breakers := NewBreakerRegistry(cfg.Breaker, m, UpstreamWallet, UpstreamOrder)

	walletTransport := NewResilientTransport(UpstreamWallet, cfg.Proxy.Timeout, cfg.Proxy.MaxRetries, breakers, m)
	orderTransport := NewResilientTransport(UpstreamOrder, cfg.Proxy.Timeout, cfg.Proxy.MaxRetries, breakers, m)

	routes := []routeEntry{
		{prefix: "/api/v1/internal/wallets", upstream: UpstreamWallet, proxy: newReverseProxy(walletURL, walletTransport)},
		{prefix: "/api/v1/internal/apps", upstream: UpstreamWallet, proxy: newReverseProxy(walletURL, walletTransport)},
		{prefix: "/api/v1/admin", upstream: UpstreamWallet, proxy: newReverseProxy(walletURL, walletTransport)},
		{prefix: "/api/v1/internal/orders", upstream: UpstreamOrder, proxy: newReverseProxy(orderURL, orderTransport)},
		{prefix: "/api/v1/internal/payments", upstream: UpstreamOrder, proxy: newReverseProxy(orderURL, orderTransport)},
		{prefix: "/api/v1/webhooks", upstream: UpstreamOrder, proxy: newReverseProxy(orderURL, orderTransport)},
	}

	return &Router{routes: routes}, nil
}

func newReverseProxy(target *url.URL, transport http.RoundTripper) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// Match NewSingleHostReverseProxy: keep inbound Host on the outbound request.
			pr.Out.Host = pr.In.Host
			// Rewrite strips X-Forwarded-* from Out; restore/append from In then set host/proto.
			pr.Out.Header["X-Forwarded-For"] = pr.In.Header["X-Forwarded-For"]
			pr.SetXForwarded()
		},
		Transport:     transport,
		FlushInterval: 100 * time.Millisecond,
		// Drop upstream CORS so the browser only sees gateway CORS (avoids duplicate ACAO).
		ModifyResponse: func(res *http.Response) error {
			for k := range res.Header {
				if strings.HasPrefix(strings.ToLower(k), "access-control-") {
					res.Header.Del(k)
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			code := http.StatusBadGateway
			msg := "Upstream request failed"
			appCode := response.CodeServiceUnavailable
			switch {
			case errors.Is(err, ErrCircuitOpen):
				code = http.StatusServiceUnavailable
				msg = "Upstream temporarily unavailable"
			default:
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					code = http.StatusGatewayTimeout
					msg = "Upstream request timed out"
				}
			}
			w.Header().Set("Content-Type", "application/json")
			if reqID := r.Header.Get(middleware.HeaderXRequestID); reqID != "" {
				w.Header().Set(middleware.HeaderXRequestID, reqID)
			}
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(response.ErrorEnvelope{
				Success:   false,
				Code:      appCode,
				Message:   msg,
				Timestamp: time.Now().UTC(),
			})
		},
	}
}

func (r *Router) match(path string) *routeEntry {
	for i := range r.routes {
		entry := &r.routes[i]
		if path == entry.prefix || strings.HasPrefix(path, entry.prefix+"/") {
			return entry
		}
	}
	return nil
}

// Handler proxies matched /api/v1 paths. Unmatched → 404.
func (r *Router) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		entry := r.match(c.Request.URL.Path)
		if entry == nil {
			response.Error(c, response.NotFound(response.CodeNotFound, "No upstream route for path"))
			return
		}

		c.Set(middleware.ContextKeyUpstream, entry.upstream)

		reqID := middleware.GetRequestID(c)
		if reqID != "" {
			c.Request.Header.Set(middleware.HeaderXRequestID, reqID)
		}

		if clientIP := c.ClientIP(); clientIP != "" {
			prior := c.Request.Header.Get("X-Forwarded-For")
			if prior == "" {
				c.Request.Header.Set("X-Forwarded-For", clientIP)
			} else {
				c.Request.Header.Set("X-Forwarded-For", prior+", "+clientIP)
			}
		}
		if c.Request.TLS != nil {
			c.Request.Header.Set("X-Forwarded-Proto", "https")
		} else if proto := c.Request.Header.Get("X-Forwarded-Proto"); proto == "" {
			c.Request.Header.Set("X-Forwarded-Proto", "http")
		}

		entry.proxy.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}
