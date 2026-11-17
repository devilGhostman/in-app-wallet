package routes

import (
	"wallet-svc/internal/config"
	"wallet-svc/internal/handler"
	"wallet-svc/internal/middleware"
	"wallet-svc/internal/repository"
	"wallet-svc/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type walletRoute struct {
	handler *handler.WalletHandler
	auth    config.AuthConfig
	limiter repository.RateLimiter
	limits  config.RateLimitConfig
	log     *zap.Logger
	router  *gin.RouterGroup
}

func NewWalletRoute(
	router *gin.RouterGroup,
	svc *services.WalletService,
	auth config.AuthConfig,
	limiter repository.RateLimiter,
	rateCfg config.RateLimitConfig,
	log *zap.Logger,
) *walletRoute {
	return &walletRoute{
		handler: handler.NewWalletHandler(svc),
		auth:    auth,
		limiter: limiter,
		limits:  rateCfg,
		log:     log,
		router:  router,
	}
}

func (r *walletRoute) Routes() {
	internal := r.router.Group("/internal")
	internal.Use(middleware.Auth(r.auth))
	internal.Use(middleware.RequireAppID())
	internal.Use(middleware.LimitRequestBody(middleware.MaxRequestBody))

	rateLimit := middleware.RateLimit(r.limiter, r.limits, r.log)

	// Hot path for Orders / callers — no rate limit.
	internal.POST("/wallets/:user_id/credit", middleware.RequireScope(config.ScopeCredit), r.handler.Credit)
	internal.POST("/wallets/:user_id/debit", middleware.RequireScope(config.ScopeDebit), r.handler.Debit)

	// Read/lookup surfaces — rate limited per IP.
	internal.POST("/wallets", rateLimit, middleware.RequireScope(config.ScopeCreateWallet), r.handler.Create)
	internal.POST("/apps/fund", rateLimit, middleware.RequireScope(config.ScopeAdmin), r.handler.FundApp)
	internal.GET("/wallets/:user_id", rateLimit, middleware.RequireScope(config.ScopeRead), r.handler.Get)
	internal.GET("/wallets/:user_id/ledger", rateLimit, middleware.RequireScope(config.ScopeRead), r.handler.Ledger)
	internal.GET("/ledger/by-order/:order_id", rateLimit, middleware.RequireScope(config.ScopeLookup), r.handler.ByOrder)
}
