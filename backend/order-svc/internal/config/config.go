package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	minSecretBytes       = 32
	defaultDevJWTSecret  = "dev-only-jwt-secret-change-me-now!!!!!!"
	defaultWebhookSecret = "dev-only-payment-webhook-secret!!!!!"

	ScopeOrderCreate = "order:create"
	ScopeOrderRead   = "order:read"
	ScopeOrderSpend  = "order:spend"
	ScopeOrderPaySim = "order:pay_simulate"
	ScopeOrderAdmin  = "order:admin"
)

// Config aggregates all application configuration values.
type Config struct {
	App       AppConfig
	DB        DBConfig
	Redis     RedisConfig
	Auth      AuthConfig
	Payment   PaymentConfig
	Limits    LimitsConfig
	Log       LogConfig
	CORS      CORSConfig
	RateLimit RateLimitConfig
	RabbitMQ  RabbitMQConfig
}

type AppConfig struct {
	Name            string
	Env             string
	Port            int
	RoutePrefix     string
	ShutdownTimeout time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	TrustedProxies  []string
	UserIDPattern   string
}

type DBConfig struct {
	URL              string
	MigrateURL       string
	MaxOpenConns     int
	MaxIdleConns     int
	ConnMaxLifetime  time.Duration
	ConnMaxIdleTime  time.Duration
	StatementTimeout time.Duration
}

type RedisConfig struct {
	URL          string
	PoolSize     int
	MinIdleConns int
}

type AuthConfig struct {
	Secrets []string
}

type PaymentConfig struct {
	WebhookSecret string
	WebhookURL    string
}

type LimitsConfig struct {
	MaxOrderAmount int64
}

type LogConfig struct {
	Level  string
	Format string
}

type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
}

type RateLimitConfig struct {
	Capacity int
	Refill   time.Duration
}

// RabbitMQConfig is AMQP 1.0 publisher + wallet lifecycle consumer settings.
type RabbitMQConfig struct {
	Enabled        bool
	URL            string
	Exchange       string
	EventsQueue    string
	PollInterval   time.Duration
	PublishTimeout time.Duration
	Prefetch       int
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	env := getEnv("APP_ENV", "development")

	jwtSecret, err := resolveSecret(env, "JWT_SECRET", getEnv("JWT_SECRET", ""), defaultDevJWTSecret)
	if err != nil {
		return nil, err
	}
	webhookSecret, err := resolveSecret(env, "PAYMENT_WEBHOOK_SECRET", getEnv("PAYMENT_WEBHOOK_SECRET", ""), defaultWebhookSecret)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		App: AppConfig{
			Name:            getEnv("APP_NAME", "order-svc"),
			Env:             env,
			Port:            getEnvAsInt("PORT", 8082),
			RoutePrefix:     getEnv("ROUTE_PREFIX", "/api/v1"),
			ShutdownTimeout: getEnvAsDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
			ReadTimeout:     getEnvAsDuration("READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    getEnvAsDuration("WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:     getEnvAsDuration("IDLE_TIMEOUT", 60*time.Second),
			TrustedProxies:  getEnvAsSlice("TRUSTED_PROXIES", []string{}),
			UserIDPattern:   getEnv("USER_ID_PATTERN", `^[a-fA-F0-9]{24}$`),
		},
		DB: DBConfig{
			URL:              getEnv("DATABASE_URL", "postgresql://order_app:order_app_password@localhost:5432/orders?sslmode=disable"),
			MigrateURL:       getEnv("MIGRATE_DATABASE_URL", getEnv("DATABASE_URL", "postgresql://order_migrate:order_migrate_password@localhost:5432/orders?sslmode=disable")),
			MaxOpenConns:     getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:     getEnvAsInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime:  getEnvAsDuration("DB_CONN_MAX_LIFETIME", 15*time.Minute),
			ConnMaxIdleTime:  getEnvAsDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
			StatementTimeout: getEnvAsDuration("DB_STATEMENT_TIMEOUT", 5*time.Second),
		},
		Redis: RedisConfig{
			URL:          getEnv("REDIS_URL", "redis://localhost:6379"),
			PoolSize:     getEnvAsInt("REDIS_POOL_SIZE", 20),
			MinIdleConns: getEnvAsInt("REDIS_MIN_IDLE_CONNS", 5),
		},
		Auth: AuthConfig{
			Secrets: compactSecrets(jwtSecret, getEnv("JWT_SECRET_PREV", "")),
		},
		Payment: PaymentConfig{
			WebhookSecret: webhookSecret,
			WebhookURL:    getEnv("PAYMENT_WEBHOOK_URL", "http://localhost:8082/api/v1/webhooks/payment"),
		},
		Limits: LimitsConfig{
			MaxOrderAmount: getEnvAsInt64("MAX_ORDER_AMOUNT", 1_000_000),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", defaultLogFormat(env)),
		},
		CORS: CORSConfig{
			AllowedOrigins: getEnvAsSlice("CORS_ALLOWED_ORIGINS", []string{"*"}),
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID", "X-Scopes", "X-App-Id", "X-Payment-Signature"},
		},
		RateLimit: RateLimitConfig{
			Capacity: getEnvAsInt("RATE_LIMIT_CAPACITY", 30),
			Refill:   getEnvAsDuration("RATE_LIMIT_REFILL", 1*time.Second),
		},
		RabbitMQ: RabbitMQConfig{
			Enabled:        getEnvAsBool("RABBITMQ_ENABLED", false),
			URL:            getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672"),
			Exchange:       getEnv("RABBITMQ_EXCHANGE", "wallet.commands"),
			EventsQueue:    getEnv("RABBITMQ_EVENTS_QUEUE", "order.wallet.lifecycle"),
			PollInterval:   getEnvAsDuration("RABBITMQ_OUTBOX_POLL_INTERVAL", 1*time.Second),
			PublishTimeout: getEnvAsDuration("RABBITMQ_PUBLISH_TIMEOUT", 5*time.Second),
			Prefetch:       clampInt(getEnvAsInt("RABBITMQ_PREFETCH", 10), 1, 1000),
		},
	}

	if cfg.RabbitMQ.Enabled {
		if strings.TrimSpace(cfg.RabbitMQ.Exchange) == "" {
			return nil, fmt.Errorf("RABBITMQ_EXCHANGE is required when RABBITMQ_ENABLED=true")
		}
		if strings.TrimSpace(cfg.RabbitMQ.EventsQueue) == "" {
			return nil, fmt.Errorf("RABBITMQ_EVENTS_QUEUE is required when RABBITMQ_ENABLED=true")
		}
	}
	if strings.TrimSpace(cfg.Payment.WebhookURL) == "" {
		return nil, fmt.Errorf("PAYMENT_WEBHOOK_URL is required")
	}

	return cfg, nil
}

func (c *Config) IsProduction() bool {
	return strings.ToLower(c.App.Env) == "production"
}

func compactSecrets(primary, previous string) []string {
	out := make([]string, 0, 2)
	if primary != "" {
		out = append(out, primary)
	}
	if previous != "" && previous != primary {
		out = append(out, previous)
	}
	return out
}

func resolveSecret(env, name, value, devDefault string) (string, error) {
	if value == "" {
		if strings.EqualFold(env, "development") {
			return devDefault, nil
		}
		return "", fmt.Errorf("%s is required when APP_ENV is not development", name)
	}
	if len(value) < minSecretBytes {
		return "", fmt.Errorf("%s must be at least %d bytes", name, minSecretBytes)
	}
	return value, nil
}

func defaultLogFormat(env string) string {
	if strings.ToLower(env) == "production" {
		return "json"
	}
	return "console"
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	valStr := getEnv(key, "")
	if val, err := strconv.Atoi(valStr); err == nil {
		return val
	}
	return defaultVal
}

func getEnvAsBool(key string, defaultVal bool) bool {
	valStr := getEnv(key, "")
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		return defaultVal
	}
	return val
}

func getEnvAsInt64(key string, defaultVal int64) int64 {
	valStr := getEnv(key, "")
	if val, err := strconv.ParseInt(valStr, 10, 64); err == nil {
		return val
	}
	return defaultVal
}

func getEnvAsDuration(key string, defaultVal time.Duration) time.Duration {
	valStr := getEnv(key, "")
	if val, err := time.ParseDuration(valStr); err == nil {
		return val
	}
	return defaultVal
}

func getEnvAsSlice(key string, defaultVal []string) []string {
	valStr := getEnv(key, "")
	if valStr == "" {
		return defaultVal
	}
	parts := strings.Split(valStr, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
