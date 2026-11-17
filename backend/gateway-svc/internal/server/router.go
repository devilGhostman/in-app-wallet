package server

import (
	"fmt"
	"net/http"

	_ "gateway-svc/docs"
	"gateway-svc/internal/config"
	"gateway-svc/internal/database"
	"gateway-svc/internal/metrics"
	"gateway-svc/internal/middleware"
	"gateway-svc/internal/proxy"
	"gateway-svc/internal/repository"
	"gateway-svc/internal/server/routes"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
)

func SetupRouter(
	cfg *config.Config,
	log *zap.Logger,
	redis *database.RedisClient,
	limiter repository.RateLimiter,
	m *metrics.Metrics,
	proxyRouter *proxy.Router,
) (*gin.Engine, error) {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	router := gin.New()
	if err := router.SetTrustedProxies(cfg.App.TrustedProxies); err != nil {
		return nil, fmt.Errorf("set trusted proxies: %w", err)
	}

	router.Use(middleware.RequestID())
	router.Use(middleware.RequestLogger(log))
	router.Use(middleware.Recovery(log))
	router.Use(middleware.SecurityHeaders(cfg.IsProduction()))
	router.Use(middleware.CORS(cfg.CORS))

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})

	routes.NewHealthRoute(&router.RouterGroup, redis, log).Routes()
	routes.NewMetricsRoute(&router.RouterGroup, m).Routes()
	routes.NewProxyRoute(router, cfg, proxyRouter, limiter, log).Routes()

	return router, nil
}
