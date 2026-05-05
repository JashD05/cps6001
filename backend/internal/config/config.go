package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config holds all application configuration.
type Config struct {
	Env          string `json:"env"`
	Server       ServerConfig
	Database     DatabaseConfig
	Redis        RedisConfig
	JWT          JWTConfig
	SIEM         SIEMConfig
	Kubernetes   KubernetesConfig
	RateLimit    RateLimitConfig
	Logging      LoggingConfig
	Notification NotificationConfig
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port               string        `json:"port"`
	ReadTimeout        time.Duration `json:"read_timeout"`
	WriteTimeout       time.Duration `json:"write_timeout"`
	IdleTimeout        time.Duration `json:"idle_timeout"`
	Host               string        `json:"host"`
	CORSAllowedOrigins string        `json:"cors_allowed_origins"`
}

// DatabaseConfig holds PostgreSQL connection configuration.
type DatabaseConfig struct {
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	Name            string        `json:"name"`
	User            string        `json:"user"`
	Password        string        `json:"-"`
	SSLMode         string        `json:"sslmode"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time"`
	MigrationsPath  string        `json:"migrations_path"`
}

// RedisConfig holds Redis connection configuration.
type RedisConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"-"`
	DB       int    `json:"db"`
}

// JWTConfig holds JWT authentication configuration.
type JWTConfig struct {
	Secret        string        `json:"-"`
	Expiry        time.Duration `json:"expiry"`
	RefreshExpiry time.Duration `json:"refresh_expiry"`
	Issuer        string        `json:"issuer"`
}

// NotificationConfig holds notification service configuration for
// sending alerts through email (SMTP), Slack, and generic webhooks.
type NotificationConfig struct {
	Enabled         bool   `json:"enabled"`
	SMTPHost        string `json:"smtp_host"`
	SMTPPort        int    `json:"smtp_port"`
	SMTPUsername    string `json:"smtp_username"`
	SMTPPassword    string `json:"-"`
	SMTPFrom        string `json:"smtp_from"`
	SMTPFromName    string `json:"smtp_from_name"`
	SlackWebhookURL string `json:"slack_webhook_url"`
	SlackChannel    string `json:"slack_channel"`
	SlackUsername   string `json:"slack_username"`
	WebhookURL      string `json:"webhook_url"`
	AsyncSend       bool   `json:"async_send"`
	RetryCount      int    `json:"retry_count"`
	TimeoutSec      int    `json:"timeout_sec"`
}

// SIEMConfig holds SIEM integration configuration.
type SIEMConfig struct {
	Enabled    bool          `json:"enabled"`
	Provider   string        `json:"provider"`
	Endpoint   string        `json:"endpoint"`
	APIKey     string        `json:"-"`
	Username   string        `json:"username,omitempty"`
	Password   string        `json:"-"`
	Index      string        `json:"index,omitempty"`
	Timeout    time.Duration `json:"timeout,omitempty"`
	MaxRetries int           `json:"max_retries,omitempty"`
}

// KubernetesConfig holds Kubernetes integration configuration.
type KubernetesConfig struct {
	Namespace      string        `json:"namespace"`
	PodTimeout     time.Duration `json:"pod_timeout"`
	MaxConcurrent  int           `json:"max_concurrent"`
	InCluster      bool          `json:"in_cluster"`
	KubeconfigPath string        `json:"kubeconfig_path"`
}

// RateLimitConfig holds rate limiting configuration.
type RateLimitConfig struct {
	Enabled  bool          `json:"enabled"`
	Requests int           `json:"requests"`
	Window   time.Duration `json:"window"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

// DSN returns the PostgreSQL data source name for connection.
func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

// MigrateDSN returns the PostgreSQL URL for golang-migrate, properly escaping
// special characters in the user and password (e.g. @, :, /, ?).
func (d *DatabaseConfig) MigrateDSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(d.User, d.Password),
		Host:   fmt.Sprintf("%s:%d", d.Host, d.Port),
		Path:   d.Name,
	}
	q := u.Query()
	q.Set("sslmode", d.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

// RedisAddr returns the Redis address string.
func (r *RedisConfig) RedisAddr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

// Load reads configuration from environment variables with sensible defaults.
// Environment variables are prefixed with CHAOS_ (e.g., CHAOS_SERVER_PORT).
func Load() (*Config, error) {
	cfg := &Config{
		Env: getEnv("CHAOS_ENV", "development"),
		Server: ServerConfig{
			Host:               getEnv("CHAOS_SERVER_HOST", "0.0.0.0"),
			Port:               getEnv("CHAOS_SERVER_PORT", "8080"),
			ReadTimeout:        getDurationEnv("CHAOS_SERVER_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:       getDurationEnv("CHAOS_SERVER_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:        getDurationEnv("CHAOS_SERVER_IDLE_TIMEOUT", 60*time.Second),
			CORSAllowedOrigins: getEnv("CHAOS_CORS_ALLOWED_ORIGINS", ""),
		},
		Database: DatabaseConfig{
			Host:            getEnv("CHAOS_DB_HOST", "localhost"),
			Port:            getIntEnv("CHAOS_DB_PORT", 5432),
			Name:            getEnv("CHAOS_DB_NAME", "chaossec"),
			User:            getEnv("CHAOS_DB_USER", "chaossec_admin"),
			Password:        getEnv("CHAOS_DB_PASSWORD", "chaossec_local_dev_password"),
			SSLMode:         getEnv("CHAOS_DB_SSLMODE", "disable"),
			MaxOpenConns:    getIntEnv("CHAOS_DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getIntEnv("CHAOS_DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getDurationEnv("CHAOS_DB_CONN_MAX_LIFETIME", 5*time.Minute),
			ConnMaxIdleTime: getDurationEnv("CHAOS_DB_CONN_MAX_IDLE_TIME", 1*time.Minute),
			MigrationsPath:  getEnv("CHAOS_DB_MIGRATIONS_PATH", "file://migrations"),
		},
		Redis: RedisConfig{
			Host:     getEnv("CHAOS_REDIS_HOST", "localhost"),
			Port:     getIntEnv("CHAOS_REDIS_PORT", 6379),
			Password: getEnv("CHAOS_REDIS_PASSWORD", "chaossec_redis_local_password"),
			DB:       getIntEnv("CHAOS_REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Secret:        getEnv("CHAOS_JWT_SECRET", ""),
			Expiry:        getDurationEnv("CHAOS_JWT_EXPIRY", 1*time.Hour),
			RefreshExpiry: getDurationEnv("CHAOS_JWT_REFRESH_EXPIRY", 7*24*time.Hour),
			Issuer:        getEnv("CHAOS_JWT_ISSUER", "chaos-sec"),
		},
		SIEM: SIEMConfig{
			Enabled:    getBoolEnv("CHAOS_SIEM_ENABLED", false),
			Provider:   getEnv("CHAOS_SIEM_PROVIDER", "mock"),
			Endpoint:   getEnv("CHAOS_SIEM_ENDPOINT", ""),
			APIKey:     getEnv("CHAOS_SIEM_API_KEY", ""),
			Username:   getEnv("CHAOS_SIEM_USERNAME", ""),
			Password:   getEnv("CHAOS_SIEM_PASSWORD", ""),
			Index:      getEnv("CHAOS_SIEM_INDEX", ""),
			Timeout:    getDurationEnv("CHAOS_SIEM_TIMEOUT", 30*time.Second),
			MaxRetries: getIntEnv("CHAOS_SIEM_MAX_RETRIES", 3),
		},
		Kubernetes: KubernetesConfig{
			Namespace:      getEnv("CHAOS_K8S_NAMESPACE", "chaos-sec"),
			PodTimeout:     getDurationEnv("CHAOS_K8S_POD_TIMEOUT", 5*time.Minute),
			MaxConcurrent:  getIntEnv("CHAOS_K8S_MAX_CONCURRENT", 10),
			InCluster:      getBoolEnv("CHAOS_K8S_IN_CLUSTER", false),
			KubeconfigPath: getEnv("CHAOS_K8S_KUBECONFIG", ""),
		},
		RateLimit: RateLimitConfig{
			Enabled:  getBoolEnv("CHAOS_RATE_LIMIT_ENABLED", true),
			Requests: getIntEnv("CHAOS_RATE_LIMIT_REQUESTS", 100),
			Window:   getDurationEnv("CHAOS_RATE_LIMIT_WINDOW", 1*time.Minute),
		},
		Logging: LoggingConfig{
			Level:  getEnv("CHAOS_LOG_LEVEL", "info"),
			Format: getEnv("CHAOS_LOG_FORMAT", "json"),
		},
		Notification: NotificationConfig{
			Enabled:         getBoolEnv("CHAOS_NOTIFICATION_ENABLED", false),
			SMTPHost:        getEnv("CHAOS_NOTIFICATION_SMTP_HOST", ""),
			SMTPPort:        getIntEnv("CHAOS_NOTIFICATION_SMTP_PORT", 587),
			SMTPUsername:    getEnv("CHAOS_NOTIFICATION_SMTP_USERNAME", ""),
			SMTPPassword:    getEnv("CHAOS_NOTIFICATION_SMTP_PASSWORD", ""),
			SMTPFrom:        getEnv("CHAOS_NOTIFICATION_SMTP_FROM", ""),
			SMTPFromName:    getEnv("CHAOS_NOTIFICATION_SMTP_FROM_NAME", "Chaos-Sec"),
			SlackWebhookURL: getEnv("CHAOS_NOTIFICATION_SLACK_WEBHOOK_URL", ""),
			SlackChannel:    getEnv("CHAOS_NOTIFICATION_SLACK_CHANNEL", ""),
			SlackUsername:   getEnv("CHAOS_NOTIFICATION_SLACK_USERNAME", "Chaos-Sec"),
			WebhookURL:      getEnv("CHAOS_NOTIFICATION_WEBHOOK_URL", ""),
			AsyncSend:       getBoolEnv("CHAOS_NOTIFICATION_ASYNC_SEND", true),
			RetryCount:      getIntEnv("CHAOS_NOTIFICATION_RETRY_COUNT", 3),
			TimeoutSec:      getIntEnv("CHAOS_NOTIFICATION_TIMEOUT_SEC", 30),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that the configuration is valid and safe for use.
func (c *Config) Validate() error {
	// Validate environment setting.
	validEnvs := map[string]bool{"development": true, "production": true}
	if !validEnvs[c.Env] {
		return fmt.Errorf("invalid environment %q: must be \"development\" or \"production\"", c.Env)
	}

	if c.Server.Port == "" {
		return fmt.Errorf("server port is required")
	}

	if c.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Database.Name == "" {
		return fmt.Errorf("database name is required")
	}
	if c.Database.User == "" {
		return fmt.Errorf("database user is required")
	}

	validSSLModes := map[string]bool{
		"disable": true, "allow": true, "prefer": true,
		"require": true, "verify-ca": true, "verify-full": true,
	}
	if !validSSLModes[c.Database.SSLMode] {
		return fmt.Errorf("invalid database SSL mode: %s", c.Database.SSLMode)
	}

	if c.Database.MaxOpenConns < 1 {
		return fmt.Errorf("database max open connections must be at least 1")
	}
	if c.Database.MaxIdleConns < 1 {
		return fmt.Errorf("database max idle connections must be at least 1")
	}

	if c.Redis.Host == "" {
		return fmt.Errorf("redis host is required")
	}

	// JWT secret validation: production must have it set; development falls back to hardcoded secret with warning.
	if c.JWT.Secret == "" {
		if c.IsProduction() {
			return fmt.Errorf("JWT secret (CHAOS_JWT_SECRET) is required in production")
		}
		c.JWT.Secret = "dev-only-insecure-jwt-secret-change-me-in-production"
		fmt.Fprintf(os.Stderr, "WARNING: Using hardcoded dev JWT secret. Set CHAOS_JWT_SECRET for secure operation.\n")
	}

	if c.JWT.Expiry < 1*time.Minute {
		return fmt.Errorf("JWT expiry must be at least 1 minute")
	}
	if c.JWT.RefreshExpiry < 1*time.Hour {
		return fmt.Errorf("JWT refresh expiry must be at least 1 hour")
	}

	if c.Kubernetes.MaxConcurrent < 1 {
		return fmt.Errorf("kubernetes max concurrent must be at least 1")
	}

	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true, "fatal": true,
	}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	validLogFormats := map[string]bool{
		"json": true, "console": true,
	}
	if !validLogFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}

	return nil
}

// Addr returns the server listen address.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}

// BuildLogger creates a zap.Logger based on the logging configuration.
func (c *Config) BuildLogger() (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(c.Logging.Level)
	if err != nil {
		return nil, fmt.Errorf("failed to parse log level %q: %w", c.Logging.Level, err)
	}

	var zapConfig zap.Config
	if c.Logging.Format == "console" {
		zapConfig = zap.NewDevelopmentConfig()
	} else {
		zapConfig = zap.NewProductionConfig()
	}

	zapConfig.Level = zap.NewAtomicLevelAt(level)

	logger, err := zapConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return logger, nil
}

// IsDevelopment returns true if the application is running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

// IsProduction returns true if the application is running in production mode.
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

// --- Environment variable helpers ---

// getEnv reads an environment variable or returns the provided default value.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return strings.TrimSpace(value)
	}
	return defaultValue
}

// getIntEnv reads an integer environment variable or returns the provided default value.
func getIntEnv(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

// getBoolEnv reads a boolean environment variable or returns the provided default value.
// Accepts: 1, t, T, TRUE, true, True, 0, f, F, FALSE, false, False.
func getBoolEnv(key string, defaultValue bool) bool {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

// getDurationEnv reads a duration environment variable or returns the provided default value.
// Accepts Go duration strings (e.g., "5s", "10m", "1h30m").
func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return defaultValue
	}
	value, err := time.ParseDuration(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}
