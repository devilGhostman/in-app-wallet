package routes

import (
	"order-svc/internal/config"
	"order-svc/internal/handler"
	"order-svc/internal/middleware"
	"order-svc/internal/repository"
	"order-svc/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type orderRoute struct {
	handler *handler.OrderHandler
	router  *gin.RouterGroup
	auth    config.AuthConfig
	limiter repository.RateLimiter
	rlCfg   config.RateLimitConfig
	log     *zap.Logger
}

func NewOrderRoute(
	router *gin.RouterGroup,
	orders *services.OrderService,
	pay *services.PaymentSimulator,
	webhookSecret string,
	auth config.AuthConfig,
	limiter repository.RateLimiter,
	rlCfg config.RateLimitConfig,
	log *zap.Logger,
) *orderRoute {
	return &orderRoute{
		handler: handler.NewOrderHandler(orders, pay, webhookSecret),
		router:  router,
		auth:    auth,
		limiter: limiter,
		rlCfg:   rlCfg,
		log:     log,
	}
}

func (r *orderRoute) Routes() {
	webhooks := r.router.Group("/webhooks")
	webhooks.POST("/payment", r.handler.PaymentWebhook)

	internal := r.router.Group("/internal")
	internal.Use(middleware.Auth(r.auth))
	internal.Use(middleware.RequireAppID())
	internal.Use(middleware.RateLimit(r.limiter, r.rlCfg, r.log))
	internal.Use(middleware.LimitRequestBody(1 << 20))

	internal.POST("/orders", middleware.RequireScope(config.ScopeOrderCreate), r.handler.Create)
	internal.POST("/orders/spend", middleware.RequireScope(config.ScopeOrderSpend), r.handler.Spend)
	internal.GET("/orders/:order_id", middleware.RequireScope(config.ScopeOrderRead), r.handler.Get)
	internal.POST("/payments/simulate", middleware.RequireScope(config.ScopeOrderPaySim), r.handler.SimulatePayment)
}
