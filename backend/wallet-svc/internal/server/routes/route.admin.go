package routes

import (
	"wallet-svc/internal/config"
	"wallet-svc/internal/handler"
	"wallet-svc/internal/metrics"
	"wallet-svc/internal/middleware"
	"wallet-svc/internal/repository"
	"wallet-svc/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type adminRoute struct {
	handler *handler.AdminHandler
	auth    config.AuthConfig
	limiter repository.RateLimiter
	limits  config.RateLimitConfig
	log     *zap.Logger
	router  *gin.RouterGroup
}

func NewAdminRoute(
	router *gin.RouterGroup,
	svc *services.WalletService,
	counters *metrics.Counters,
	auth config.AuthConfig,
	limiter repository.RateLimiter,
	rateCfg config.RateLimitConfig,
	log *zap.Logger,
) *adminRoute {
	return &adminRoute{
		handler: handler.NewAdminHandler(svc, counters),
		auth:    auth,
		limiter: limiter,
		limits:  rateCfg,
		log:     log,
		router:  router,
	}
}

func (r *adminRoute) Routes() {
	admin := r.router.Group("/admin")
	admin.Use(middleware.Auth(r.auth))
	admin.Use(middleware.RequireScope(config.ScopeAdmin))
	admin.Use(middleware.RateLimit(r.limiter, r.limits, r.log))
	admin.Use(middleware.LimitRequestBody(middleware.MaxRequestBody))

	admin.GET("/reconciliation", r.handler.Reconcile)
	admin.GET("/metrics", r.handler.Metrics)

	withApp := admin.Group("")
	withApp.Use(middleware.RequireAppID())
	withApp.POST("/wallets/:user_id/freeze", r.handler.Freeze)
	withApp.POST("/wallets/:user_id/unfreeze", r.handler.Unfreeze)
	withApp.POST("/wallets/:user_id/close", r.handler.Close)
}
