package metrics

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	RetriesTotal    *prometheus.CounterVec
	CircuitState    *prometheus.GaugeVec
}

func New() *Metrics {
	return &Metrics{
		RequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_http_requests_total",
			Help: "Total HTTP requests proxied by the gateway",
		}, []string{"upstream", "method", "status"}),
		RequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_http_request_duration_seconds",
			Help:     "Gateway proxy request duration in seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"upstream"}),
		RetriesTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_upstream_retries_total",
			Help: "Total upstream retries performed by the gateway",
		}, []string{"upstream"}),
		CircuitState: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gateway_circuit_state",
			Help: "Circuit breaker state: 0=closed, 1=open, 2=half-open",
		}, []string{"upstream"}),
	}
}

func (m *Metrics) Handler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func StatusCode(code int) string {
	if code <= 0 {
		return "0"
	}
	return strconv.Itoa(code)
}
