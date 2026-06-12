package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Storage  StorageConfig
	JWT      JWTConfig
	Meta     MetaConfig
	OTP      OTPConfig
	NATS     NATSConfig
}

type NATSConfig struct {
	URL           string
	StreamName    string
	InternalToken string
	ConsumerName  string
	Enabled       bool
}

type OTPConfig struct {
	DefaultDeviceSlug string
}

type MetaConfig struct {
	AppID       string
	AppSecret   string
	ConfigID    string
	VerifyToken string
	APIVersion  string
}

type AppConfig struct {
	Name string
	Env  string
}

type HTTPConfig struct {
	Host         string
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	URL      string
	Password string
	DB       int
}

type StorageConfig struct {
	Driver          string
	LocalPath       string
	MinIOEndpoint   string
	MinIOAccessKey  string
	MinIOSecretKey  string
	MinIOBucket     string
	MinIOUseSSL     bool
}

type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		App: AppConfig{
			Name: getEnv("APP_NAME", "omni-chan"),
			Env:  getEnv("APP_ENV", "development"),
		},
		HTTP: HTTPConfig{
			Host:         getEnv("HTTP_HOST", "0.0.0.0"),
			Port:         getEnvInt("HTTP_PORT", 8080),
			ReadTimeout:  getEnvDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getEnvDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
		},
		Database: DatabaseConfig{
			URL:             getEnv("DATABASE_URL", ""),
			MaxOpenConns:    getEnvInt("DATABASE_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DATABASE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("DATABASE_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Redis: RedisConfig{
			URL:      getEnv("REDIS_URL", "redis://localhost:6379/0"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Storage: StorageConfig{
			Driver:          getEnv("STORAGE_DRIVER", "local"),
			LocalPath:       getEnv("STORAGE_LOCAL_PATH", "./storage"),
			MinIOEndpoint:   getEnv("MINIO_ENDPOINT", "localhost:9000"),
			MinIOAccessKey:  getEnv("MINIO_ACCESS_KEY", ""),
			MinIOSecretKey:  getEnv("MINIO_SECRET_KEY", ""),
			MinIOBucket:     getEnv("MINIO_BUCKET", "omni-chan"),
			MinIOUseSSL:     getEnvBool("MINIO_USE_SSL", false),
		},
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", ""),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", ""),
			AccessTTL:     getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL:    getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		Meta: MetaConfig{
			AppID:       getEnv("META_APP_ID", ""),
			AppSecret:   getEnv("META_APP_SECRET", ""),
			ConfigID:    getEnv("META_EMBEDDED_CONFIG_ID", ""),
			VerifyToken: getEnv("META_VERIFY_TOKEN", ""),
			APIVersion:  getEnv("META_API_VERSION", "v21.0"),
		},
		OTP: OTPConfig{
			DefaultDeviceSlug: getEnv("OTP_DEFAULT_DEVICE_SLUG", "dgos-waba"),
		},
		NATS: NATSConfig{
			URL:           getEnv("NATS_URL", "nats://localhost:4222"),
			StreamName:    getEnv("NATS_STREAM_NAME", "WEBHOOK_EVENTS"),
			InternalToken: getEnv("INTERNAL_SERVICE_TOKEN", ""),
			ConsumerName:  getEnv("NATS_CONSUMER_NAME", "backend-service"),
			Enabled:       getEnvBool("NATS_CONSUMER_ENABLED", true),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.Storage.Driver != "local" && c.Storage.Driver != "minio" {
		return fmt.Errorf("STORAGE_DRIVER must be local or minio")
	}
	if c.Storage.Driver == "minio" {
		if c.Storage.MinIOAccessKey == "" || c.Storage.MinIOSecretKey == "" {
			return fmt.Errorf("MINIO_ACCESS_KEY and MINIO_SECRET_KEY are required when STORAGE_DRIVER=minio")
		}
	}
	if c.JWT.AccessSecret == "" || c.JWT.RefreshSecret == "" {
		if c.App.Env == "development" {
			c.JWT.AccessSecret = "dev-access-secret-change-in-production"
			c.JWT.RefreshSecret = "dev-refresh-secret-change-in-production"
		} else {
			return fmt.Errorf("JWT_ACCESS_SECRET and JWT_REFRESH_SECRET are required")
		}
	}
	if c.Meta.VerifyToken == "" && c.App.Env == "development" {
		c.Meta.VerifyToken = "dev-meta-verify-token"
	}
	if c.NATS.InternalToken == "" && c.App.Env == "development" {
		c.NATS.InternalToken = "dev-internal-token"
	}
	return nil
}

func (c *HTTPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}
