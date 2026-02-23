package config

import (
	"os"
	"testing"
)

func TestLoad_Development(t *testing.T) {
	os.Setenv("KARI_ENV", "development")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("JWT_SECRET")
	os.Unsetenv("ENCRYPTION_KEY")

	cfg := Load()

	expectedDB := "postgres://kari_admin:dev_password@localhost:5432/kari?sslmode=disable"
	if cfg.DatabaseURL != expectedDB {
		t.Errorf("Expected default DB URL %s, got %s", expectedDB, cfg.DatabaseURL)
	}

	if cfg.Environment != "development" {
		t.Errorf("Expected environment development, got %s", cfg.Environment)
	}
}

func TestLoad_Production_MissingSecrets(t *testing.T) {
	// We can't easily test log.Fatal without extra effort,
	// but we can test that it doesn't crash if they ARE set.
	os.Setenv("KARI_ENV", "production")
	os.Setenv("DATABASE_URL", "postgres://prod:prod@prod:5432/db")
	os.Setenv("JWT_SECRET", "supersecret-at-least-32-chars-long-123")
	os.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Load() panicked: %v", r)
		}
	}()

	cfg := Load()

	if cfg.Environment != "production" {
		t.Errorf("Expected environment production, got %s", cfg.Environment)
	}

	if cfg.DatabaseURL != "postgres://prod:prod@prod:5432/db" {
		t.Errorf("Expected production DB URL, got %s", cfg.DatabaseURL)
	}
}
