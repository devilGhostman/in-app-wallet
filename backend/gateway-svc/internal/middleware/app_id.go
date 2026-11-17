package middleware

import (
	"strings"

	"gateway-svc/pkg/response"

	"github.com/gin-gonic/gin"
)

const (
	HeaderAppID     = "X-App-Id"
	ContextKeyAppID = "app_id"
)

// EnsureAppID requires a non-empty X-App-Id. Returns false if aborted.
func EnsureAppID(c *gin.Context) bool {
	appID := strings.TrimSpace(c.GetHeader(HeaderAppID))
	if appID == "" {
		response.Error(c, response.Unprocessable(response.CodeInvalidRequest, "X-App-Id header is required"))
		c.Abort()
		return false
	}
	c.Set(ContextKeyAppID, appID)
	return true
}

func RequireAppID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if EnsureAppID(c) {
			c.Next()
		}
	}
}
