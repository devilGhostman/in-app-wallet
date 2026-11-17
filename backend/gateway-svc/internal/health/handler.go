package health

import (
	"context"
	"net/http"
	"time"

	"gateway-svc/internal/database"
	"gateway-svc/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	redis  *database.RedisClient
	logger *zap.Logger
}

func NewHandler(redis *database.RedisClient, logger *zap.Logger) *Handler {
	return &Handler{redis: redis, logger: logger}
}

func (h *Handler) Liveness(c *gin.Context) {
	response.OK(c, gin.H{"status": "healthy"}, "Application is running")
}

func (h *Handler) Readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if h.redis == nil {
		c.JSON(http.StatusServiceUnavailable, response.ErrorEnvelope{
			Success:   false,
			Code:      response.CodeServiceUnavailable,
			Message:   "Redis is unconfigured",
			Timestamp: time.Now().UTC(),
			Errors:    gin.H{"redis": "unconfigured"},
		})
		return
	}
	if err := h.redis.Ping(ctx); err != nil {
		h.logger.Warn("Readiness check: redis unavailable", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, response.ErrorEnvelope{
			Success:   false,
			Code:      response.CodeServiceUnavailable,
			Message:   "Redis is unavailable",
			Timestamp: time.Now().UTC(),
			Errors:    gin.H{"redis": err.Error()},
		})
		return
	}

	response.OK(c, gin.H{"redis": "connected"}, "Service is ready")
}
