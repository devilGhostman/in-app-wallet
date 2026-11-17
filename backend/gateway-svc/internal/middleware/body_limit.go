package middleware

import (
	"net/http"

	"gateway-svc/pkg/response"

	"github.com/gin-gonic/gin"
)

const MaxRequestBody = 16 << 10

func LimitRequestBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > n {
			response.Error(c, response.BadRequest(response.CodeInvalidBody, "Request body is too large"))
			c.Abort()
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		}
		c.Next()
	}
}
