package middleware

import (
	"fmt"
	"strings"

	"gateway-svc/internal/config"
	"gateway-svc/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	HeaderScopes = "X-Scopes"
	ctxKeyScopes = "scopes"
)

type ServiceClaims struct {
	jwt.RegisteredClaims
}

// EnsureAuth validates a Bearer JWT (HS256) and non-empty X-Scopes.
// Returns false if the request was aborted with an error response.
func EnsureAuth(c *gin.Context, auth config.AuthConfig) bool {
	raw, ok := bearerToken(c.GetHeader("Authorization"))
	if !ok {
		response.Error(c, response.Unauthorized(response.CodeNotAuthorized, "Missing or invalid Authorization header"))
		c.Abort()
		return false
	}

	if _, err := parseServiceJWT(raw, auth.Secrets); err != nil {
		response.Error(c, response.Unauthorized(response.CodeNotAuthorized, "Unknown service credentials"))
		c.Abort()
		return false
	}

	scopes := parseScopesHeader(c.GetHeader(HeaderScopes))
	if len(scopes) == 0 {
		response.Error(c, response.Unauthorized(response.CodeNotAuthorized, "Missing X-Scopes header"))
		c.Abort()
		return false
	}

	c.Set(ctxKeyScopes, scopes)
	return true
}

// Auth validates a Bearer JWT signed with JWT_SECRET (HS256).
// Scopes come from the X-Scopes header (comma or space separated).
func Auth(auth config.AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if EnsureAuth(c, auth) {
			c.Next()
		}
	}
}

func parseScopesHeader(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseServiceJWT(raw string, secrets []string) (*ServiceClaims, error) {
	if len(secrets) == 0 {
		return nil, fmt.Errorf("no jwt secrets configured")
	}

	var lastErr error
	for _, secret := range secrets {
		claims := &ServiceClaims{}
		token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
			if t.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(secret), nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
		if err != nil {
			lastErr = err
			continue
		}
		if !token.Valid {
			lastErr = fmt.Errorf("invalid token")
			continue
		}
		return claims, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("invalid token")
	}
	return nil, lastErr
}

func bearerToken(header string) (string, bool) {
	if header == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}
