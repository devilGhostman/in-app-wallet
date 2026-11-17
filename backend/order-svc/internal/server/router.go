package server

import (
	"fmt"
	"net/http"

	_ "order-svc/docs"
	"order-svc/internal/config"
	"order-svc/internal/database"
	"order-svc/internal/middleware"
	"order-svc/internal/server/routes"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
)

func SetupRouter(
	cfg *config.Config,
	log *zap.Logger,
	db *database.PostgresDB,
	redis *database.RedisClient,
	deps OrderDeps,
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
	router.Use(middleware.SecurityHeaders(cfg.IsProduction()))
	router.Use(middleware.RequestLogger(log))
	router.Use(middleware.Recovery(log))
	router.Use(middleware.CORS(cfg.CORS))

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})

	routes.NewHealthRoute(&router.RouterGroup, db, redis, log).Routes()

	apiV1 := router.Group(cfg.App.RoutePrefix)
	routes.NewHealthRoute(apiV1, db, redis, log).Routes()
	routes.NewOrderRoute(apiV1, deps.Orders, deps.PaySim, cfg.Payment.WebhookSecret, cfg.Auth, deps.Limiter, cfg.RateLimit, log).Routes()

	return router, nil
}
