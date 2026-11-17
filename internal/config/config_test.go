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
	if cfg.StorageService.URL != "http://127.0.0.1:8082" {
		t.Errorf("StorageService.URL = %q, want default http://127.0.0.1:8082", cfg.StorageService.URL)
	}
}

// Storage drivers (local/minio) live in storage-service; backend only needs its URL.
func TestLoad_RequiresStorageServiceURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test?sslmode=disable")
	t.Setenv("APP_ENV", "development")
	t.Setenv("STORAGE_SERVICE_URL", "   ")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when STORAGE_SERVICE_URL is blank")
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

// Channel access tokens go to the Graph host, so production must never point it elsewhere.
func TestValidate_GraphBaseURLForbiddenInProduction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test?sslmode=disable")
	t.Setenv("JWT_ACCESS_SECRET", "a")
	t.Setenv("JWT_REFRESH_SECRET", "b")
	t.Setenv("MEDIA_TOKEN_SECRET", "c")
	t.Setenv("META_GRAPH_BASE_URL", "http://fake-graph:8080")

	t.Setenv("APP_ENV", "production")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for META_GRAPH_BASE_URL in production")
	}

	t.Setenv("APP_ENV", "staging")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("staging Load() error = %v", err)
	}
	if cfg.Meta.GraphBaseURL != "http://fake-graph:8080" {
		t.Errorf("GraphBaseURL = %q", cfg.Meta.GraphBaseURL)
	}
}
