package routes

import (
	"net/http"
	"strings"

	"gateway-svc/internal/config"
	"gateway-svc/internal/middleware"
	"gateway-svc/internal/proxy"
	"gateway-svc/internal/repository"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ProxyRoute struct {
	router  *gin.Engine
	cfg     *config.Config
	proxy   *proxy.Router
	limiter repository.RateLimiter
	log     *zap.Logger
}

func NewProxyRoute(
	router *gin.Engine,
	cfg *config.Config,
	p *proxy.Router,
	limiter repository.RateLimiter,
	log *zap.Logger,
) *ProxyRoute {
	return &ProxyRoute{router: router, cfg: cfg, proxy: p, limiter: limiter, log: log}
}

func (r *ProxyRoute) Routes() {
	api := r.router.Group(r.cfg.App.RoutePrefix)
	api.Use(middleware.LimitRequestBody(middleware.MaxRequestBody))
	api.Use(middleware.RateLimit(r.limiter, r.cfg.RateLimit, r.log))
	// Single catch-all under /api/v1 — Gin panics if /*path coexists with a /webhooks sibling route.
	api.Use(r.authUnlessPaymentWebhook)
	api.Any("/*path", r.proxy.Handler())
}

func (r *ProxyRoute) authUnlessPaymentWebhook(c *gin.Context) {
	if isPaymentWebhook(c) {
		c.Next()
		return
	}
	if !middleware.EnsureAuth(c, r.cfg.Auth) {
		return
	}
	if !middleware.EnsureAppID(c) {
		return
	}
	c.Next()
}

func isPaymentWebhook(c *gin.Context) bool {
	if c.Request.Method != http.MethodPost {
		return false
	}
	path := strings.TrimSuffix(c.Request.URL.Path, "/")
	return strings.HasSuffix(path, "/webhooks/payment")
}
