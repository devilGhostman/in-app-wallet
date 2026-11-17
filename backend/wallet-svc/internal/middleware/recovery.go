package middleware

import (
	"net/http"
	"runtime/debug"
	"time"

	"wallet-svc/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				reqID := GetRequestID(c)
				stack := string(debug.Stack())

				logger.Error("Server panic recovered",
					zap.Any("error", r),
					zap.String("request_id", reqID),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
					zap.String("stack", stack),
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, response.ErrorEnvelope{
					Success:   false,
					Code:      response.CodeInternalServerError,
					Message:   "Internal server error",
					Timestamp: time.Now().UTC(),
				})
			}
		}()

		c.Next()
	}
}
