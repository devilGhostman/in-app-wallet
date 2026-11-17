package middleware

import (
	"strings"

	"wallet-svc/pkg/response"

	"github.com/gin-gonic/gin"
)

const (
	HeaderAppID     = "X-App-Id"
	ContextKeyAppID = "app_id"
)

// RequireAppID rejects requests missing a non-empty X-App-Id header.
func RequireAppID() gin.HandlerFunc {
	return func(c *gin.Context) {
		appID := strings.TrimSpace(c.GetHeader(HeaderAppID))
		if appID == "" {
			response.Error(c, response.Unprocessable(response.CodeInvalidRequest, "X-App-Id header is required"))
			c.Abort()
			return
		}
		c.Set(ContextKeyAppID, appID)
		c.Next()
	}
}

func GetAppID(c *gin.Context) string {
	if val, exists := c.Get(ContextKeyAppID); exists {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return strings.TrimSpace(c.GetHeader(HeaderAppID))
}
