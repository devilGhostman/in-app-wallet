package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"wallet-svc/internal/config"
	"wallet-svc/internal/database"

	"go.uber.org/zap"
)

type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
}

func NewServer(
	cfg *config.Config,
	logger *zap.Logger,
	db *database.PostgresDB,
	redis *database.RedisClient,
	deps WalletDeps,
) (*Server, error) {
	router, err := SetupRouter(cfg, logger, db, redis, deps)
	if err != nil {
		return nil, err
	}

	return &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.App.Port),
			Handler:      router,
			ReadTimeout:  cfg.App.ReadTimeout,
			WriteTimeout: cfg.App.WriteTimeout,
			IdleTimeout:  cfg.App.IdleTimeout,
		},
		logger: logger,
	}, nil
}

func (s *Server) Start() <-chan error {
	errChan := make(chan error, 1)
	go func() {
		s.logger.Info("HTTP server starting", zap.String("addr", "http://localhost"+s.httpServer.Addr))
		s.logger.Info("Swagger docs", zap.String("addr", "http://localhost"+s.httpServer.Addr+"/swagger"))
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("http server failure: %w", err)
			return
		}
		close(errChan)
	}()
	return errChan
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("HTTP server shutting down gracefully...")
	return s.httpServer.Shutdown(ctx)
}
