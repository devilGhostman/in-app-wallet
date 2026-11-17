package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App      AppConfig
	Auth     AuthConfig
	Redis    RedisConfig
	RateLimit RateLimitConfig
	Proxy    ProxyConfig
	Breaker  BreakerConfig
	CORS     CORSConfig
	Log      LogConfig
}

type AppConfig struct {
	Name           string
	Env            string
	Port           int
	RoutePrefix    string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	ShutdownTimeout time.Duration
	TrustedProxies []string
}

type AuthConfig struct {
	Secrets []string
}

type RedisConfig struct {
	URL          string
	PoolSize     int
	MinIdleConns int
}

type RateLimitConfig struct {
	Capacity int
	Refill   time.Duration
}

type ProxyConfig struct {
	WalletUpstream string
	OrderUpstream  string
	Timeout        time.Duration
	MaxRetries     int
}

type BreakerConfig struct {
	Failures    uint32
	OpenTimeout time.Duration
}

type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
}

type LogConfig struct {
	Level  string
	Format string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	port, err := strconv.Atoi(getEnv("PORT", "8081"))
	if err != nil {
		return nil, fmt.Errorf("invalid PORT: %w", err)
	}

	secrets := []string{}
	if s := strings.TrimSpace(os.Getenv("JWT_SECRET")); s != "" {
		secrets = append(secrets, s)
	}
	if s := strings.TrimSpace(os.Getenv("JWT_SECRET_PREV")); s != "" {
		secrets = append(secrets, s)
	}
	if len(secrets) == 0 {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	capacity, err := strconv.Atoi(getEnv("RATE_LIMIT_CAPACITY", "60"))
	if err != nil || capacity < 1 {
		return nil, fmt.Errorf("invalid RATE_LIMIT_CAPACITY")
	}
	refill, err := time.ParseDuration(getEnv("RATE_LIMIT_REFILL", "1s"))
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_LIMIT_REFILL: %w", err)
	}

	proxyTimeout, err := time.ParseDuration(getEnv("PROXY_TIMEOUT", "5s"))
	if err != nil {
		return nil, fmt.Errorf("invalid PROXY_TIMEOUT: %w", err)
	}
	maxRetries, err := strconv.Atoi(getEnv("PROXY_MAX_RETRIES", "2"))
	if err != nil || maxRetries < 0 {
		return nil, fmt.Errorf("invalid PROXY_MAX_RETRIES")
	}

	breakerFailures, err := strconv.ParseUint(getEnv("BREAKER_FAILURES", "5"), 10, 32)
	if err != nil || breakerFailures < 1 {
		return nil, fmt.Errorf("invalid BREAKER_FAILURES")
	}
	breakerOpen, err := time.ParseDuration(getEnv("BREAKER_OPEN_TIMEOUT", "10s"))
	if err != nil {
		return nil, fmt.Errorf("invalid BREAKER_OPEN_TIMEOUT: %w", err)
	}

	readTimeout, err := time.ParseDuration(getEnv("SERVER_READ_TIMEOUT", "15s"))
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_READ_TIMEOUT: %w", err)
	}
	writeTimeout, err := time.ParseDuration(getEnv("SERVER_WRITE_TIMEOUT", "30s"))
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_WRITE_TIMEOUT: %w", err)
	}
	idleTimeout, err := time.ParseDuration(getEnv("SERVER_IDLE_TIMEOUT", "60s"))
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_IDLE_TIMEOUT: %w", err)
	}
	shutdownTimeout, err := time.ParseDuration(getEnv("SERVER_SHUTDOWN_TIMEOUT", "10s"))
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_SHUTDOWN_TIMEOUT: %w", err)
	}

	poolSize, _ := strconv.Atoi(getEnv("REDIS_POOL_SIZE", "10"))
	minIdle, _ := strconv.Atoi(getEnv("REDIS_MIN_IDLE_CONNS", "2"))

	cfg := &Config{
		App: AppConfig{
			Name:            getEnv("APP_NAME", "gateway-svc"),
			Env:             getEnv("APP_ENV", "development"),
			Port:            port,
			RoutePrefix:     getEnv("ROUTE_PREFIX", "/api/v1"),
			ReadTimeout:     readTimeout,
			WriteTimeout:    writeTimeout,
			IdleTimeout:     idleTimeout,
			ShutdownTimeout: shutdownTimeout,
			TrustedProxies:  splitCSV(getEnv("TRUSTED_PROXIES", "")),
		},
		Auth: AuthConfig{Secrets: secrets},
		Redis: RedisConfig{
			URL:          getEnv("REDIS_URL", "redis://localhost:6379/0"),
			PoolSize:     poolSize,
			MinIdleConns: minIdle,
		},
		RateLimit: RateLimitConfig{
			Capacity: capacity,
			Refill:   refill,
		},
		Proxy: ProxyConfig{
			WalletUpstream: strings.TrimRight(getEnv("WALLET_UPSTREAM", "http://localhost:8080"), "/"),
			OrderUpstream:  strings.TrimRight(getEnv("ORDER_UPSTREAM", "http://localhost:8082"), "/"),
			Timeout:        proxyTimeout,
			MaxRetries:     maxRetries,
		},
		Breaker: BreakerConfig{
			Failures:    uint32(breakerFailures),
			OpenTimeout: breakerOpen,
		},
		CORS: CORSConfig{
			AllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "*")),
			AllowedMethods: splitCSV(getEnv("CORS_ALLOWED_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS")),
			AllowedHeaders: splitCSV(getEnv("CORS_ALLOWED_HEADERS", "Authorization,Content-Type,X-Request-ID,X-App-Id,X-Scopes")),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "console"),
		},
	}
	return cfg, nil
}

func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.App.Env, "production")
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
