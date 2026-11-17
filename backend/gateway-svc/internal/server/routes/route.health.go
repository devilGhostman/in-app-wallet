package routes

import (
	"gateway-svc/internal/database"
	"gateway-svc/internal/health"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type HealthRoute struct {
	router  *gin.RouterGroup
	handler *health.Handler
}

func NewHealthRoute(router *gin.RouterGroup, redis *database.RedisClient, log *zap.Logger) *HealthRoute {
	return &HealthRoute{
		router:  router,
		handler: health.NewHandler(redis, log),
	}
}

func (r *HealthRoute) Routes() {
	r.router.GET("/healthz", r.handler.Liveness)
	r.router.GET("/readyz", r.handler.Readiness)
}
