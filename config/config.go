package config

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App               AppConfig
	HTTP              ServerConfig
	GRPC              ServerConfig
	MySQL             MySQLConfig
	Log               LogConfig
	InternalEndpoints InternalEndpointsConfig
	Stripe            StripeConfig
	Payments          PaymentsConfig
	Jobs              JobsConfig
}

type AppConfig struct {
	ServiceName string
	APIKey      string
}

type ServerConfig struct {
	Host string
	Port string
}

type MySQLConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type LogConfig struct {
	Level string
}

type InternalEndpointsConfig struct {
	AuthGRPCAddr string
}

type StripeConfig struct {
	SecretKey                 string
	WebhookSecret             string
	ProviderCallbackBaseURL   string
	SignatureToleranceSeconds int64
	HTTPTimeout               time.Duration
}

type PaymentsConfig struct {
	CallbackMaxAttempts   int32
	CallbackRetryInterval time.Duration
	CallbackHTTPTimeout   time.Duration
	PendingTimeout        time.Duration
	ReconcileStaleAfter   time.Duration
	JobBatchSize          int32
}

type JobsConfig struct {
	ReconcileInterval       time.Duration
	CallbackDispatchInterval time.Duration
	ExpirePendingInterval    time.Duration
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	mysqlDSN := os.Getenv("MYSQL_DSN")
	if mysqlDSN == "" {
		return nil, errors.New("MYSQL_DSN environment variable is required")
	}

	return &Config{
		App: AppConfig{
			ServiceName: getEnv("APP_SERVICE_NAME", "payments-service"),
			APIKey:      getEnv("APP_API_KEY", ""),
		},
		HTTP: ServerConfig{
			Host: getEnv("HTTP_HOST", "0.0.0.0"),
			Port: getEnv("HTTP_PORT", "8080"),
		},
		GRPC: ServerConfig{
			Host: getEnv("GRPC_HOST", "0.0.0.0"),
			Port: getEnv("GRPC_PORT", "9090"),
		},
		MySQL: MySQLConfig{
			DSN:             mysqlDSN,
			MaxOpenConns:    getIntEnv("MYSQL_MAX_OPEN_CONNS", 10),
			MaxIdleConns:    getIntEnv("MYSQL_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getMinutesEnv("MYSQL_CONN_MAX_LIFETIME_MINUTES", 30*time.Minute),
		},
		Log: LogConfig{
			Level: getEnv("LOG_LEVEL", "info"),
		},
		InternalEndpoints: InternalEndpointsConfig{
			AuthGRPCAddr: getEnv("AUTH_SERVICE_GRPC_ADDR", "localhost:9090"),
		},
		Stripe: StripeConfig{
			SecretKey:                 getEnv("STRIPE_SECRET_KEY", ""),
			WebhookSecret:             getEnv("STRIPE_WEBHOOK_SECRET", ""),
			ProviderCallbackBaseURL:   getEnv("PAYMENTS_PROVIDER_CALLBACK_BASE_URL", ""),
			SignatureToleranceSeconds: int64(getIntEnv("STRIPE_SIGNATURE_TOLERANCE_SECONDS", 300)),
			HTTPTimeout:               getSecondsEnv("STRIPE_HTTP_TIMEOUT_SECONDS", 10*time.Second),
		},
		Payments: PaymentsConfig{
			CallbackMaxAttempts:   int32(getIntEnv("PAYMENTS_CALLBACK_MAX_ATTEMPTS", 10)),
			CallbackRetryInterval: getMinutesEnv("PAYMENTS_CALLBACK_RETRY_INTERVAL_MINUTES", 5*time.Minute),
			CallbackHTTPTimeout:   getSecondsEnv("PAYMENTS_CALLBACK_HTTP_TIMEOUT_SECONDS", 10*time.Second),
			PendingTimeout:        getMinutesEnv("PAYMENTS_PENDING_TIMEOUT_MINUTES", 60*time.Minute),
			ReconcileStaleAfter:   getMinutesEnv("PAYMENTS_RECONCILE_STALE_AFTER_MINUTES", 15*time.Minute),
			JobBatchSize:          int32(getIntEnv("PAYMENTS_JOB_BATCH_SIZE", 100)),
		},
		Jobs: JobsConfig{
			ReconcileInterval:        getMinutesEnv("PAYMENTS_RECONCILE_INTERVAL_MINUTES", 2*time.Minute),
			CallbackDispatchInterval: getMinutesEnv("PAYMENTS_CALLBACK_DISPATCH_INTERVAL_MINUTES", time.Minute),
			ExpirePendingInterval:    getMinutesEnv("PAYMENTS_EXPIRE_PENDING_INTERVAL_MINUTES", 5*time.Minute),
		},
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return defaultValue
}

func getMinutesEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if minutes, err := strconv.Atoi(value); err == nil {
			return time.Duration(minutes) * time.Minute
		}
	}
	return defaultValue
}

func getSecondsEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultValue
}
