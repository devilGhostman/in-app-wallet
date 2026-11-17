package routes

import (
	"gateway-svc/internal/metrics"

	"github.com/gin-gonic/gin"
)

type MetricsRoute struct {
	router  *gin.RouterGroup
	metrics *metrics.Metrics
}

func NewMetricsRoute(router *gin.RouterGroup, m *metrics.Metrics) *MetricsRoute {
	return &MetricsRoute{router: router, metrics: m}
}

func (r *MetricsRoute) Routes() {
	r.router.GET("/metrics", r.metrics.Handler())
}
