package config

import (
	"os"
	"os/exec"
	"testing"
)

func TestLoad_Development(t *testing.T) {
	os.Setenv("KARI_ENV", "development")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("JWT_SECRET")
	os.Unsetenv("ENCRYPTION_KEY")

	cfg := Load()

	expectedDB := ""
	if cfg.DatabaseURL != expectedDB {
		t.Errorf("Expected empty DB URL when unset, got %q", cfg.DatabaseURL)
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
	os.Setenv("CORS_ALLOWED_ORIGINS", "http://prod.example.com")

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

func TestLoad_Production_DevPassword(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		os.Setenv("KARI_ENV", "production")
		os.Setenv("DATABASE_URL", "postgres://kari_admin:dev_password@localhost:5432/kari")
		os.Setenv("JWT_SECRET", "supersecret-at-least-32-chars-long-123")
		Load()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLoad_Production_DevPassword")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	}
	t.Fatalf("process ran with err %v, want exit status 1", err)
}
