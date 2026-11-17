package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wallet-svc/internal/config"
	"wallet-svc/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

const testSecret = "shared-service-token-aaaaaaaaaaaaaaaa"

func testAuth() config.AuthConfig {
	return config.AuthConfig{
		Secrets: []string{testSecret},
	}
}

func signToken(t *testing.T, secret string, sub string, exp time.Time) string {
	t.Helper()
	claims := ServiceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	require.NoError(t, err)
	return s
}

func TestAuthJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := testAuth()
	valid := signToken(t, testSecret, "orders", time.Now().Add(time.Hour))
	expired := signToken(t, testSecret, "orders", time.Now().Add(-time.Hour))
	wrongKey := signToken(t, "different-secret-bbbbbbbbbbbbbbbbbb", "orders", time.Now().Add(time.Hour))

	cases := []struct {
		name   string
		token  string
		scopes string
		scope  string
		status int
	}{
		{"jwt can credit", valid, config.ScopeCredit + "," + config.ScopeAdmin + "," + config.ScopeCreateWallet, config.ScopeCredit, http.StatusOK},
		{"jwt can create", valid, config.ScopeCreateWallet, config.ScopeCreateWallet, http.StatusOK},
		{"jwt can admin", valid, config.ScopeAdmin, config.ScopeAdmin, http.StatusOK},
		{"missing scope", valid, config.ScopeCredit, config.ScopeDebit, http.StatusForbidden},
		{"missing token", "", config.ScopeRead, config.ScopeRead, http.StatusUnauthorized},
		{"raw secret rejected", testSecret, config.ScopeRead, config.ScopeRead, http.StatusUnauthorized},
		{"wrong signature", wrongKey, config.ScopeRead, config.ScopeRead, http.StatusUnauthorized},
		{"expired", expired, config.ScopeRead, config.ScopeRead, http.StatusUnauthorized},
		{"missing scopes header", valid, "", config.ScopeRead, http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.POST("/x", Auth(auth), RequireScope(tc.scope), func(c *gin.Context) {
				response.OK(c, gin.H{"ok": true})
			})
			req := httptest.NewRequest(http.MethodPost, "/x", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.scopes != "" {
				req.Header.Set(HeaderScopes, tc.scopes)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
		})
	}
}
