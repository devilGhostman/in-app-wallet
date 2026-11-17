package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	headerXContentTypeOptions     = "X-Content-Type-Options"
	headerXFrameOptions           = "X-Frame-Options"
	headerReferrerPolicy          = "Referrer-Policy"
	headerPermissionsPolicy       = "Permissions-Policy"
	headerContentSecurityPolicy   = "Content-Security-Policy"
	headerStrictTransportSecurity = "Strict-Transport-Security"
	headerCacheControl            = "Cache-Control"

	valueNosniff        = "nosniff"
	valueFrameDeny      = "DENY"
	valueNoReferrer     = "no-referrer"
	valueNoStore        = "no-store"
	valuePermissionsOff = "geolocation=(), microphone=(), camera=()"
	valueHSTS           = "max-age=31536000; includeSubDomains"
	cspAPI              = "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
	cspSwagger          = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'"
)

func SecurityHeaders(enableHSTS bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set(headerXContentTypeOptions, valueNosniff)
		h.Set(headerXFrameOptions, valueFrameDeny)
		h.Set(headerReferrerPolicy, valueNoReferrer)
		h.Set(headerPermissionsPolicy, valuePermissionsOff)

		if isSwaggerPath(c.Request.URL.Path) {
			h.Set(headerContentSecurityPolicy, cspSwagger)
		} else {
			h.Set(headerContentSecurityPolicy, cspAPI)
			h.Set(headerCacheControl, valueNoStore)
		}

		if enableHSTS {
			h.Set(headerStrictTransportSecurity, valueHSTS)
		}

		c.Next()
	}
}

func isSwaggerPath(path string) bool {
	return path == "/swagger" || strings.HasPrefix(path, "/swagger/")
}
