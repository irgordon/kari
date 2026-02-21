// api/internal/core/domain/profile.go
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SystemProfile dictates the global defaults for the Karı control panel.
// By loading this as data, the Go API never hardcodes business rules.
type SystemProfile struct {
	ID                    uuid.UUID `json:"id"`
	DefaultPHPVersion     string    `json:"default_php_version"`     // e.g., "8.3"
	DefaultFirewallPolicy string    `json:"default_firewall_policy"` // e.g., "deny_all_inbound"
	BackupRetentionDays   int       `json:"backup_retention_days"`   // e.g., 30
	SSLStrategy           string    `json:"ssl_strategy"`            // e.g., "letsencrypt", "zerossl", "custom_pki"
	MailRoutingEngine     string    `json:"mail_routing_engine"`     // e.g., "postfix", "exim"
	UpdatedAt             time.Time `json:"updated_at"`
}

// SystemProfileRepository defines the SLA for fetching the active profile.
type SystemProfileRepository interface {
	GetActiveProfile(ctx context.Context) (*SystemProfile, error)
	UpdateProfile(ctx context.Context, profile *SystemProfile) error
}
