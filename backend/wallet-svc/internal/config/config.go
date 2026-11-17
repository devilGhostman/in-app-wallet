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
	minSecretBytes      = 32
	defaultDevJWTSecret = "dev-only-jwt-secret-change-me-now!!!!!!"
	ScopeCreateWallet   = "wallet:create"
	ScopeRead           = "wallet:read"
	ScopeCredit         = "wallet:credit"
	ScopeDebit          = "wallet:debit"
	ScopeLookup         = "wallet:lookup"
	ScopeAdmin          = "wallet:admin"
)

// Config aggregates all application configuration values.
type Config struct {
	App       AppConfig
	DB        DBConfig
	Redis     RedisConfig
	Auth      AuthConfig
	Limits    LimitsConfig
	Log       LogConfig
	CORS      CORSConfig
	RateLimit RateLimitConfig
	RabbitMQ  RabbitMQConfig
}

// RabbitMQConfig holds AMQP 1.0 consumer + lifecycle outbox publisher settings.
type RabbitMQConfig struct {
	Enabled            bool
	URL                string
	Exchange           string
	QueuePrefix        string
	ShardTotal         int
	ShardIndex         int
	ShardCount         int
	CommandsDLQ        string
	EventsExchange     string
	OutboxPollInterval time.Duration
	PublishTimeout     time.Duration
	Prefetch           int
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
	// AppWalletShards is the number of APP pool shards per app_id (sticky by user_id hash).
	AppWalletShards int
}

type DBConfig struct {
	URL              string
	MigrateURL       string
	MaxOpenConns     int
	MaxIdleConns     int
	ConnMaxLifetime  time.Duration
	ConnMaxIdleTime  time.Duration
	LockTimeout      time.Duration
	StatementTimeout time.Duration
}

type RedisConfig struct {
	URL          string
	PoolSize     int
	MinIdleConns int
}

// AuthConfig holds HMAC secrets used to verify service JWTs (HS256).
// Optional JWT_SECRET_PREV supports key rotation. Scopes come from the X-Scopes header.
type AuthConfig struct {
	Secrets []string
}

type LimitsConfig struct {
	MaxCreditAmount  int64
	MaxDebitAmount   int64
	MaxWalletBalance int64
	DailyCreditCap   int64
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

func Load() (*Config, error) {
	_ = godotenv.Load()

	env := getEnv("APP_ENV", "development")

	jwtSecret, err := resolveSecret(env, "JWT_SECRET", getEnv("JWT_SECRET", ""), defaultDevJWTSecret)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		App: AppConfig{
			Name:            getEnv("APP_NAME", "wallet-svc"),
			Env:             env,
			Port:            getEnvAsInt("PORT", 8080),
			RoutePrefix:     getEnv("ROUTE_PREFIX", "/api/v1"),
			ShutdownTimeout: getEnvAsDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
			ReadTimeout:     getEnvAsDuration("READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    getEnvAsDuration("WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:     getEnvAsDuration("IDLE_TIMEOUT", 60*time.Second),
			TrustedProxies:  getEnvAsSlice("TRUSTED_PROXIES", []string{}),
			UserIDPattern:   getEnv("USER_ID_PATTERN", `^[a-fA-F0-9]{24}$`),
			AppWalletShards: clampInt(getEnvAsInt("APP_WALLET_SHARDS", 16), 1, 64),
		},
		DB: DBConfig{
			URL:              getEnv("DATABASE_URL", "postgresql://wallet_app:wallet_app_password@localhost:5432/wallet?sslmode=disable"),
			MigrateURL:       getEnv("MIGRATE_DATABASE_URL", getEnv("DATABASE_URL", "postgresql://wallet_migrate:wallet_migrate_password@localhost:5432/wallet?sslmode=disable")),
			MaxOpenConns:     getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:     getEnvAsInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime:  getEnvAsDuration("DB_CONN_MAX_LIFETIME", 15*time.Minute),
			ConnMaxIdleTime:  getEnvAsDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
			LockTimeout:      getEnvAsDuration("DB_LOCK_TIMEOUT", 2*time.Second),
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
		Limits: LimitsConfig{
			MaxCreditAmount:  getEnvAsInt64("MAX_CREDIT_AMOUNT", 1_000_000),
			MaxDebitAmount:   getEnvAsInt64("MAX_DEBIT_AMOUNT", 1_000_000),
			MaxWalletBalance: getEnvAsInt64("MAX_WALLET_BALANCE", 10_000_000),
			DailyCreditCap:   getEnvAsInt64("DAILY_CREDIT_CAP", 2_000_000),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", defaultLogFormat(env)),
		},
		CORS: CORSConfig{
			AllowedOrigins: getEnvAsSlice("CORS_ALLOWED_ORIGINS", []string{"*"}),
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID", "X-Scopes", "X-App-Id"},
		},
		RateLimit: RateLimitConfig{
			Capacity: getEnvAsInt("RATE_LIMIT_CAPACITY", 30),
			Refill:   getEnvAsDuration("RATE_LIMIT_REFILL", 1*time.Second),
		},
		RabbitMQ: RabbitMQConfig{
			Enabled:            getEnvAsBool("RABBITMQ_ENABLED", false),
			URL:                getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672"),
			Exchange:           getEnv("RABBITMQ_EXCHANGE", "wallet.commands"),
			QueuePrefix:        getEnv("RABBITMQ_QUEUE_PREFIX", "wallet.commands"),
			ShardTotal:         getEnvAsInt("RABBITMQ_SHARD_TOTAL", 16),
			ShardIndex:         getEnvAsInt("RABBITMQ_SHARD_INDEX", 0),
			ShardCount:         getEnvAsInt("RABBITMQ_SHARD_COUNT", 1),
			CommandsDLQ:        getEnv("RABBITMQ_COMMANDS_DLQ", "wallet.commands.dlq"),
			EventsExchange:     getEnv("RABBITMQ_EVENTS_EXCHANGE", "wallet.events"),
			OutboxPollInterval: getEnvAsDuration("RABBITMQ_OUTBOX_POLL_INTERVAL", 1*time.Second),
			PublishTimeout:     getEnvAsDuration("RABBITMQ_PUBLISH_TIMEOUT", 5*time.Second),
			Prefetch:           clampInt(getEnvAsInt("RABBITMQ_PREFETCH", 10), 1, 1000),
		},
	}

	if err := validateRabbitMQShards(cfg.RabbitMQ); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateRabbitMQShards(mq RabbitMQConfig) error {
	if !mq.Enabled {
		return nil
	}
	if mq.ShardTotal <= 0 {
		return fmt.Errorf("RABBITMQ_SHARD_TOTAL must be > 0")
	}
	if mq.ShardCount <= 0 {
		return fmt.Errorf("RABBITMQ_SHARD_COUNT must be > 0")
	}
	if mq.ShardCount > mq.ShardTotal {
		return fmt.Errorf("RABBITMQ_SHARD_COUNT (%d) must be <= RABBITMQ_SHARD_TOTAL (%d)", mq.ShardCount, mq.ShardTotal)
	}
	if mq.ShardIndex < 0 || mq.ShardIndex >= mq.ShardCount {
		return fmt.Errorf("RABBITMQ_SHARD_INDEX (%d) must be in [0, %d)", mq.ShardIndex, mq.ShardCount)
	}
	if strings.TrimSpace(mq.QueuePrefix) == "" {
		return fmt.Errorf("RABBITMQ_QUEUE_PREFIX is required")
	}
	if strings.TrimSpace(mq.CommandsDLQ) == "" {
		return fmt.Errorf("RABBITMQ_COMMANDS_DLQ is required")
	}
	if strings.TrimSpace(mq.EventsExchange) == "" {
		return fmt.Errorf("RABBITMQ_EVENTS_EXCHANGE is required")
	}
	return nil
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
