package middleware

import (
	"net/http"
	"strings"

	"gateway-svc/internal/config"

	"github.com/gin-gonic/gin"
)

func CORS(cfg config.CORSConfig) gin.HandlerFunc {
	allowedMethods := strings.Join(cfg.AllowedMethods, ", ")
	allowedHeaders := strings.Join(cfg.AllowedHeaders, ", ")
	allowAll := len(cfg.AllowedOrigins) == 1 && cfg.AllowedOrigins[0] == "*"

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		if origin != "" {
			allowOrigin := ""
			if allowAll {
				// Browsers reject ACAO "*" when credentials are true — echo the request origin.
				allowOrigin = origin
			} else {
				for _, o := range cfg.AllowedOrigins {
					if o == origin || o == "*" {
						allowOrigin = origin
						break
					}
				}
			}

			if allowOrigin != "" {
				h := c.Writer.Header()
				h.Set("Access-Control-Allow-Origin", allowOrigin)
				h.Set("Access-Control-Allow-Methods", allowedMethods)
				h.Set("Access-Control-Allow-Headers", allowedHeaders)
				h.Set("Access-Control-Expose-Headers", "Content-Length, X-Request-ID")
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Max-Age", "86400")
				h.Add("Vary", "Origin")
			}
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
