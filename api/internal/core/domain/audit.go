package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type SystemAlert struct {
	ID         uuid.UUID      `json:"id"`
	Severity   string         `json:"severity"`
	Category   string         `json:"category"`
	ResourceID uuid.UUID      `json:"resource_id"`
	Message    string         `json:"message"`
	Metadata   map[string]any `json:"metadata"`
	IsResolved bool           `json:"is_resolved"`
	CreatedAt  time.Time      `json:"created_at"`
}

type AlertFilter struct {
	ResourceID uuid.UUID
	Severity   string
	IsResolved *bool
	TraceID    string
	Limit      int
	Offset     int
}

type AuditRepository interface {
	CreateAlert(ctx context.Context, alert *SystemAlert) error
	GetFilteredAlerts(ctx context.Context, filter AlertFilter) ([]SystemAlert, int, error)
	ResolveAlert(ctx context.Context, alertID uuid.UUID, resolverID uuid.UUID) error
}

type AuditService interface {
	LogSystemAlert(ctx context.Context, event string, category string, resourceID uuid.UUID, err error, severity string)
}
