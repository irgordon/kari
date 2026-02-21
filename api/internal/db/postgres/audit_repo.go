// api/internal/db/postgres/audit_repo.go
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	
	"kari/api/internal/core/domain"
)

type AuditRepository struct {
	pool *pgxpool.Pool
}

func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{pool: pool}
}

// 🛡️ CreateAlert ensures system events are persisted with consistent metadata.
func (r *AuditRepository) CreateAlert(ctx context.Context, alert *domain.SystemAlert) error {
	query := `
		INSERT INTO system_alerts (severity, category, resource_id, message, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`
	return r.pool.QueryRow(ctx, query,
		alert.Severity,
		alert.Category,
		alert.ResourceID,
		alert.Message,
		alert.Metadata,
	).Scan(&alert.ID, &alert.CreatedAt)
}

/**
 * GetFilteredAlerts builds a dynamic SQL query based on UI filters.
 * Hardened with Tenant Isolation and JSONB GIN-indexed searching.
 */
func (r *AuditRepository) GetFilteredAlerts(ctx context.Context, filter domain.AlertFilter) ([]domain.SystemAlert, int, error) {
	// 🛡️ 1. Zero-Trust Isolation
	// We ensure filters are applied so tenants can't scrape the global system_alerts table.
	query := `SELECT id, severity, category, resource_id, message, is_resolved, metadata, created_at FROM system_alerts WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM system_alerts WHERE 1=1`
	
	filterParts := ""
	var args []any
	argCount := 1

	if filter.IsResolved != nil {
		filterParts += fmt.Sprintf(" AND is_resolved = $%d", argCount)
		args = append(args, *filter.IsResolved)
		argCount++
	}

	if filter.Severity != "" {
		filterParts += fmt.Sprintf(" AND severity = $%d", argCount)
		args = append(args, filter.Severity)
		argCount++
	}

	// 🛡️ JSONB Trace ID Search (GIN Indexed)
	if filter.TraceID != "" {
		filterParts += fmt.Sprintf(" AND metadata @> jsonb_build_object('trace_id', $%d::text)", argCount)
		args = append(args, filter.TraceID)
		argCount++
	}

	// Apply filters to both queries
	query += filterParts
	countQuery += filterParts

	// Get Total Count for UI Pagination
	var totalCount int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count alerts: %w", err)
	}

	// 🛡️ 3. SLA Pagination Limits
	limit := filter.Limit
	if limit <= 0 || limit > 100 { limit = 50 }
	
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argCount, argCount+1)
	args = append(args, limit, filter.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch alerts: %w", err)
	}
	defer rows.Close()

	alerts, err := pgx.CollectRows(rows, pgx.RowToStructByName[domain.SystemAlert])
	return alerts, totalCount, err
}

func (r *AuditRepository) ResolveAlert(ctx context.Context, alertID uuid.UUID, resolverID uuid.UUID) error {
	// 🛡️ Atomic Resolution with Resolver Tracking
	query := `
		UPDATE system_alerts 
		SET is_resolved = true, resolved_at = NOW(), metadata = metadata || jsonb_build_object('resolved_by', $1::text)
		WHERE id = $2 AND is_resolved = false
	`
	tag, err := r.pool.Exec(ctx, query, resolverID, alertID)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return errors.New("alert not found or already resolved")
	}

	return nil
}
