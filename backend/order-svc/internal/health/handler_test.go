package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestReadinessFailsWithoutPostgres(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, nil, zap.NewNop())
	r := gin.New()
	r.GET("/readyz", h.Readiness)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, false, body["success"])
}

func TestLivenessAlwaysOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, nil, zap.NewNop())
	r := gin.New()
	r.GET("/healthz", h.Liveness)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusOK, w.Code)
}
