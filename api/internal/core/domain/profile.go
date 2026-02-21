package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// SystemProfile dictates the global behavior, resource limits, and safety boundaries.
// 🛡️ SLA: This struct strictly holds INTENT, not OS-level deployment paths.
type SystemProfile struct {
	ID                    uuid.UUID         `json:"id"`
	
	// 📦 Stack & Routing Governance
	DefaultStackRegistry  map[string]string `json:"stack_defaults"`
	SSLStrategy           string            `json:"ssl_strategy"`
	
	// 🛡️ Resource Jailing (SLA Enforcement)
	MaxMemoryPerAppMB     int               `json:"max_memory_per_app_mb"` 
	MaxCPUPercentPerApp   int               `json:"max_cpu_percent_per_app"`
	
	// 🛡️ Security & Identity Policies
	DefaultFirewallPolicy string            `json:"default_firewall_policy"`
	AppUserUIDRangeStart  int               `json:"app_user_uid_range_start"`
	AppUserUIDRangeEnd    int               `json:"app_user_uid_range_end"`
	
	// 💾 Backup & Retention
	BackupRetentionDays   int               `json:"backup_retention_days"`
	
	// 🛡️ Stability: Optimistic Concurrency Control
	// Prevents two admins from accidentally overwriting each other's configuration changes.
	Version               int               `json:"version"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// 🛡️ Domain-Driven Integrity
// Validate ensures the struct contains mathematically and logically sound intent
// before it is ever sent to the database or the Rust Muscle.
func (p *SystemProfile) Validate() error {
	if p.MaxMemoryPerAppMB < 128 {
		return errors.New("domain validation failed: MaxMemoryPerAppMB must be at least 128MB")
	}
	if p.MaxCPUPercentPerApp < 10 || p.MaxCPUPercentPerApp > 100 {
		return errors.New("domain validation failed: MaxCPUPercentPerApp must be between 10 and 100")
	}
	if p.AppUserUIDRangeStart >= p.AppUserUIDRangeEnd {
		return errors.New("domain validation failed: UID range start must be strictly less than range end")
	}
	if p.BackupRetentionDays < 0 {
		return errors.New("domain validation failed: BackupRetentionDays cannot be negative")
	}
	return nil
}

// SystemProfileRepository defines the interface for state persistence.
type SystemProfileRepository interface {
	GetActiveProfile(ctx context.Context) (*SystemProfile, error)
	
	// UpdateProfile mutates the system state. 
	// 🛡️ Implementation detail for adapters: Must check the 'Version' field and 
	// return a concurrency error if the DB version > the struct version.
	UpdateProfile(ctx context.Context, profile *SystemProfile) error
}
