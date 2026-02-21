package domain

import "context"

// WebServerManager defines the SLA for controlling the ingress layer (Nginx/Caddy).
// This interface abstracts the gRPC calls sent to the Rust Muscle.
type WebServerManager interface {
	// ApplyConfig generates and reloads the web server configuration.
	ApplyConfig(ctx context.Context, config WebServerConfig) error

	// RemoveConfig purges the configuration and cleans up associated VHost files.
	RemoveConfig(ctx context.Context, domainName string) error

	// Reload triggers a zero-downtime configuration hot-reload.
	Reload(ctx context.Context) error
}

// WebServerConfig holds the intent for a virtual host.
type WebServerConfig struct {
	DomainName     string
	LocalPort      int    // The internal port where the Systemd Jail is listening
	AppType        string // e.g., "proxy", "static", "php-fpm"
	
	// 🛡️ Performance & Security
	MaxBodySizeMB  int    // Maps to client_max_body_size
	ProxyTimeout   int    // Seconds before timing out upstream
	
	// 🛡️ SSL/TLS Metadata
	// These paths are provided by the Go Brain but managed by the Rust Agent.
	SSLCertPath    string 
	SSLKeyPath     string
	EnforceHSTS    bool
	
	// 🛡️ Filesystem Context
	// Required for "static" or "php-fpm" types to locate the web root.
	RootDirectory  string 
}
