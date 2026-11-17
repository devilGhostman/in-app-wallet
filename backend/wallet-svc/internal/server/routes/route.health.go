package routes

import (
	"wallet-svc/internal/database"
	"wallet-svc/internal/health"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type healthRoute struct {
	handler *health.Handler
	router  *gin.RouterGroup
}

func NewHealthRoute(router *gin.RouterGroup, db *database.PostgresDB, redis *database.RedisClient, logger *zap.Logger) *healthRoute {
	return &healthRoute{
		handler: health.NewHandler(db, redis, logger),
		router:  router,
	}
}

func (r *healthRoute) Routes() {
	r.router.GET("/healthz", r.handler.Liveness)
	r.router.GET("/readyz", r.handler.Readiness)
}
