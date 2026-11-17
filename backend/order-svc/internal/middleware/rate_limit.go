package middleware

import (
	"fmt"
	"math"
	"strconv"

	"order-svc/internal/config"
	"order-svc/internal/repository"
	"order-svc/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func rateLimitKey(ip string) string {
	if ip == "" {
		ip = "unknown"
	}
	return fmt.Sprintf("order-svc:ratelimit:%s", ip)
}

// RateLimit applies a per-IP token bucket. Redis errors fail open.
func RateLimit(limiter repository.RateLimiter, cfg config.RateLimitConfig, log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limiter == nil {
			c.Next()
			return
		}
		key := rateLimitKey(c.ClientIP())
		allowed, retryAfter, err := limiter.Take(c.Request.Context(), key, cfg.Capacity, cfg.Refill)
		if err != nil {
			log.Error("rate limit check failed", zap.Error(err), zap.String("key", key))
			c.Next()
			return
		}
		if allowed {
			c.Next()
			return
		}

		seconds := int(math.Ceil(retryAfter.Seconds()))
		if seconds < 1 {
			seconds = 1
		}
		c.Header("Retry-After", strconv.Itoa(seconds))
		response.Error(c, response.TooManyRequests(response.CodeTooManyRequests, "Too many requests; try again later"))
		c.Abort()
	}
}
