// api/internal/config/config.go
package config

import "os"

// Config holds all dynamic configuration, ensuring no hardcoded values exist in the business logic.
type Config struct {
	DatabaseURL string
	Port        string
	
	// Dynamic System Paths
	WebRootPath   string // e.g., "/var/www"
	NginxConfPath string // e.g., "/etc/nginx/sites-available"
	SSLStorageDir string // e.g., "/etc/kari/ssl"
}

// Load parses the environment and applies sensible default fallbacks.
func Load() *Config {
	return &Config{
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://kari:kari@localhost:5432/kari?sslmode=disable"),
		Port:          getEnv("PORT", "8080"),
		
		// System paths now driven entirely by data/environment
		WebRootPath:   getEnv("KARI_WEB_ROOT", "/var/www"),
		NginxConfPath: getEnv("KARI_NGINX_CONF", "/etc/nginx/sites-available"),
		SSLStorageDir: getEnv("KARI_SSL_DIR", "/etc/kari/ssl"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
