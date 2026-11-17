package health

import (
	"context"
	"net/http"
	"time"

	"order-svc/internal/database"
	"order-svc/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	db     *database.PostgresDB
	redis  *database.RedisClient
	logger *zap.Logger
}

func NewHandler(db *database.PostgresDB, redis *database.RedisClient, logger *zap.Logger) *Handler {
	return &Handler{db: db, redis: redis, logger: logger}
}

func (h *Handler) Liveness(c *gin.Context) {
	response.OK(c, gin.H{"status": "healthy"}, "Application is running")
}

func (h *Handler) Readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	checks := gin.H{
		"postgres": "connected",
		"redis":    "degraded",
	}

	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, response.ErrorEnvelope{
			Success:   false,
			Code:      response.CodeServiceUnavailable,
			Message:   "PostgreSQL is unconfigured",
			Timestamp: time.Now().UTC(),
			Errors:    gin.H{"postgres": "unconfigured"},
		})
		return
	}
	if err := h.db.Ping(ctx); err != nil {
		h.logger.Warn("Readiness check: postgres unavailable", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, response.ErrorEnvelope{
			Success:   false,
			Code:      response.CodeServiceUnavailable,
			Message:   "PostgreSQL is unavailable",
			Timestamp: time.Now().UTC(),
			Errors:    gin.H{"postgres": err.Error()},
		})
		return
	}

	if h.redis != nil {
		if err := h.redis.Ping(ctx); err != nil {
			h.logger.Warn("Readiness check: redis unavailable", zap.Error(err))
			checks["redis"] = "unavailable"
		} else {
			checks["redis"] = "connected"
		}
	}

	response.OK(c, checks, "Service is ready")
}
