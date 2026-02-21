package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"kari/api/internal/core/domain"
)

type AuditRepository struct {
	pool *pgxpool.Pool
}

func NewAuditRepository(pool *pgxpool.Pool) domain.AuditRepository {
	return &AuditRepository{pool: pool}
}

// CreateAlert ensures system events are persisted with consistent metadata.
func (r *AuditRepository) CreateAlert(ctx context.Context, alert *domain.SystemAlert) error {
	// 🛡️ Zero-Trust: Default to unresolved on creation
	query := `
		INSERT INTO system_alerts (severity, category, resource_id, message, metadata, is_resolved)
		VALUES ($1, $2, $3, $4, $5, false)
		RETURNING id, created_at
	`
	// Ensure metadata is never nil to satisfy Postgres JSONB constraints
	if alert.Metadata == nil {
		alert.Metadata = make(map[string]any)
	}

	return r.pool.QueryRow(ctx, query,
		alert.Severity,
		alert.Category,
		alert.ResourceID,
		alert.Message,
		alert.Metadata,
	).Scan(&alert.ID, &alert.CreatedAt)
}

// GetFilteredAlerts builds a dynamic query for the Action Center UI.
func (r *AuditRepository) GetFilteredAlerts(ctx context.Context, filter domain.AlertFilter) ([]domain.SystemAlert, int, error) {
	// Base queries
	baseQuery := `SELECT id, severity, category, resource_id, message, is_resolved, metadata, created_at FROM system_alerts WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM system_alerts WHERE 1=1`
	
	filterSQL := ""
	args := []any{}
	argIdx := 1

	// 🛡️ Tenant Isolation: Enforce resource-level scoping
	if filter.ResourceID != uuid.Nil {
		filterSQL += fmt.Sprintf(" AND resource_id = $%d", argIdx)
		args = append(args, filter.ResourceID.String())
		argIdx++
	}

	if filter.Severity != "" {
		filterSQL += fmt.Sprintf(" AND severity = $%d", argIdx)
		args = append(args, filter.Severity)
		argIdx++
	}

	if filter.IsResolved != nil {
		filterSQL += fmt.Sprintf(" AND is_resolved = $%d", argIdx)
		args = append(args, *filter.IsResolved)
		argIdx++
	}

	// 🛡️ JSONB Deep Search: Utilize GIN index for trace_id searches
	if filter.TraceID != "" {
		filterSQL += fmt.Sprintf(" AND metadata @> jsonb_build_object('trace_id', $%d::text)", argIdx)
		args = append(args, filter.TraceID)
		argIdx++
	}

	// Get total count for UI pagination
	var totalCount int
	err := r.pool.QueryRow(ctx, countQuery+filterSQL, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count alerts: %w", err)
	}

	// 🛡️ SLA: Strict Pagination Limits
	limit := filter.Limit
	if limit <= 0 || limit > 100 { limit = 50 }
	
	finalQuery := fmt.Sprintf("%s%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", 
		baseQuery, filterSQL, argIdx, argIdx+1)
	
	args = append(args, limit, filter.Offset)

	rows, err := r.pool.Query(ctx, finalQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch alerts: %w", err)
	}
	defer rows.Close()

	// 🛡️ Performance: Scan directly into domain structs
	alerts, err := pgx.CollectRows(rows, pgx.RowToStructByName[domain.SystemAlert])
	return alerts, totalCount, err
}

// ResolveAlert marks an issue as fixed and logs the resolver identity.
func (r *AuditRepository) ResolveAlert(ctx context.Context, alertID uuid.UUID, resolverID uuid.UUID) error {
	// 🛡️ Atomic JSONB Update: Append resolver info to metadata without overwriting existing data
	query := `
		UPDATE system_alerts 
		SET is_resolved = true, 
		    resolved_at = NOW(), 
		    metadata = metadata || jsonb_build_object('resolved_by', $1::text)
		WHERE id = $2 AND is_resolved = false
	`
	tag, err := r.pool.Exec(ctx, query, resolverID.String(), alertID)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return errors.New("alert not found or already resolved")
	}

	return nil
}
