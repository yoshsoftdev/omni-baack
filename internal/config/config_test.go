package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_ValidatesDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test?sslmode=disable")
	t.Setenv("APP_ENV", "development")
	t.Setenv("HTTP_PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTP.Port != 9090 {
		t.Errorf("HTTP.Port = %d, want 9090", cfg.HTTP.Port)
	}
	if cfg.HTTP.ReadTimeout != 10*time.Second {
		t.Errorf("HTTP.ReadTimeout = %v", cfg.HTTP.ReadTimeout)
	}
	if cfg.Storage.Driver != "local" {
		t.Errorf("Storage.Driver = %q, want local", cfg.Storage.Driver)
	}
}

func TestLoad_MinIOValidation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test?sslmode=disable")
	t.Setenv("STORAGE_DRIVER", "minio")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for minio without credentials")
	}
}

func TestHTTPConfig_Addr(t *testing.T) {
	cfg := HTTPConfig{Host: "127.0.0.1", Port: 8080}
	if got := cfg.Addr(); got != "127.0.0.1:8080" {
		t.Errorf("Addr() = %q", got)
	}
}

func TestGetEnv(t *testing.T) {
	key := "OMNI_CHAN_TEST_ENV"
	os.Unsetenv(key)
	if got := getEnv(key, "fallback"); got != "fallback" {
		t.Errorf("getEnv unset = %q", got)
	}
	t.Setenv(key, "value")
	if got := getEnv(key, "fallback"); got != "value" {
		t.Errorf("getEnv set = %q", got)
	}
}
