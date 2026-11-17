package logger

import (
	"strings"

	"wallet-svc/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger(cfg config.LogConfig, env string) (*zap.Logger, error) {
	level := parseLevel(cfg.Level)

	var zapConfig zap.Config
	if strings.ToLower(cfg.Format) == "json" || strings.ToLower(env) == "production" {
		zapConfig = zap.NewProductionConfig()
		zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		zapConfig.EncoderConfig.TimeKey = "timestamp"
		zapConfig.EncoderConfig.LevelKey = "level"
		zapConfig.EncoderConfig.MessageKey = "message"
		zapConfig.EncoderConfig.CallerKey = "caller"
	} else {
		zapConfig = zap.NewDevelopmentConfig()
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		zapConfig.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")
	}

	zapConfig.Level = zap.NewAtomicLevelAt(level)
	zapConfig.DisableStacktrace = true

	return zapConfig.Build()
}

func parseLevel(lvl string) zapcore.Level {
	switch strings.ToLower(lvl) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
