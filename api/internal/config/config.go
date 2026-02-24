package config

import (
	"log"
	"os"
	"strings"
)

// Config holds all dynamic configuration for the Brain.
// 🛡️ SLA: It knows NOTHING about the host operating system's filesystem.
type Config struct {
	Environment string // "development" or "production"
	DatabaseURL string
	Port        string
	AllowedOrigins []string

	// 🛡️ Zero-Trust Identity
	JWTSecret    string
	MasterKeyHex string

	// 🛡️ The Execution Boundary
	AgentSocket string // e.g., "/var/run/kari/agent.sock"
}

// Load parses the environment and applies sensible default fallbacks.
func Load() *Config {
	env := getEnv("KARI_ENV", "production")

	// 1. 🛡️ Zero-Trust: Fail Fast on Missing Secrets
	jwtSecret := getEnv("JWT_SECRET", "")
	if jwtSecret == "" && env == "production" {
		// Never boot securely without a cryptographic signing key
		log.Fatal("🚨 [FATAL] JWT_SECRET environment variable is required in production.")
	}

	dbURL := getEnv("DATABASE_URL", "")
	if dbURL == "" {
		if env == "production" {
			log.Fatal("🚨 [FATAL] DATABASE_URL environment variable is required in production.")
		}
		// Sensible default for local development ONLY
		dbURL = "postgres://kari_admin:dev_password@localhost:5432/kari?sslmode=disable"
	}

	// 3. 🛡️ Strict CORS: Must be explicitly defined in Production
	corsOrigins := getEnv("CORS_ALLOWED_ORIGINS", "")
	if corsOrigins == "" {
		if env == "production" {
			log.Fatal("🚨 [FATAL] CORS_ALLOWED_ORIGINS environment variable is required in production.")
		}
		corsOrigins = "http://localhost:5173"
	}

	return &Config{
		Environment:    env,
		DatabaseURL:    dbURL,
		Port:           getEnv("PORT", "8080"),
		AllowedOrigins: strings.Split(corsOrigins, ","),
		JWTSecret:      jwtSecret,
		MasterKeyHex:   getEnv("ENCRYPTION_KEY", ""),

		// 2. 🛡️ Network Agnosticism: The only way the Brain talks to the Muscle
		AgentSocket: getEnv("AGENT_SOCKET", "/var/run/kari/agent.sock"),
	}
}

// getEnv retrieves an environment variable or returns a fallback value.
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
