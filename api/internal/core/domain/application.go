package domain

import (
	"context"
	"time"
	"github.com/google/uuid"
)

// Application represents the core deployment entity.
type Application struct {
	ID           uuid.UUID         `json:"id"`
	DomainID     uuid.UUID         `json:"domain_id"`
	AppType      string            `json:"app_type"`
	DomainName   string            `json:"domain_name,omitempty"` // Eagerly loaded for Agent gRPC
	OwnerID      uuid.UUID         `json:"owner_id"`              // For IDOR & Rank checks
	AppUser      string            `json:"app_user"`             // OS-level jail identity
	RepoURL      string            `json:"repo_url"`
	Branch       string            `json:"branch"`
	BuildCommand string            `json:"build_command"`
	StartCommand string            `json:"start_command"`
	EnvVars      map[string]string `json:"env_vars"`             // JSONB GIN-indexed
	Port         int               `json:"port"`
	Status       string            `json:"status"`               // enum: stopped, starting, running, failed
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// ApplicationMetadata is a "Value Object" used specifically for high-performance 
// Authorization checks in the Service layer.
type ApplicationMetadata struct {
	ID         uuid.UUID
	Name       string
	DomainID   uuid.UUID
	DomainName string
	OwnerID    uuid.UUID
	OwnerRank  int // 🛡️ Injected via SQL Join for Rank-based security
}

// ApplicationRepository defines the platform-agnostic contract.
type ApplicationRepository interface {
	Create(ctx context.Context, app *Application) error
	
	// GetByID handles standard tenant-isolated lookups
	GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*Application, error)
	
	// GetByIDWithMetadata supports the Rank-Based Deletion flow
	GetByIDWithMetadata(ctx context.Context, id uuid.UUID) (*ApplicationMetadata, error)
	
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	UpdateEnvVars(ctx context.Context, id uuid.UUID, envVars map[string]string) error
	
	// Delete handles the atomic removal of the record
	Delete(ctx context.Context, id uuid.UUID) error
}
