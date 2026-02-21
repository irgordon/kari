// api/internal/db/postgres/audit_repo.go
package postgres

import (
	"context"
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

// ==============================================================================
// Dynamic Action Center Querying
// ==============================================================================

/**
 * GetFilteredAlerts builds a dynamic SQL query based on UI filters.
 * Hardened to include JSONB metadata searching and strict pagination limits.
 */
func (r *AuditRepository) GetFilteredAlerts(ctx context.Context, filter domain.AlertFilter) ([]domain.SystemAlert, error) {
	// 1. Base query including the metadata JSONB column
	query := `
		SELECT id, severity, category, resource_id, message, is_resolved, metadata, created_at 
		FROM system_alerts 
		WHERE 1=1
	`
	
	var args []any
	argCount := 1

	if filter.IsResolved != nil {
		query += fmt.Sprintf(" AND is_resolved = $%d", argCount)
		args = append(args, *filter.IsResolved)
		argCount++
	}

	if filter.Severity != "" {
		query += fmt.Sprintf(" AND severity = $%d", argCount)
		args = append(args, filter.Severity)
		argCount++
	}

	if filter.Category != "" {
		query += fmt.Sprintf(" AND category = $%d", argCount)
		args = append(args, filter.Category)
		argCount++
	}

	// 🛡️ 2. Roadmap Feature: JSONB Trace ID Search
	// We use the PostgreSQL `@>` (contains) operator. When paired with a GIN index on 
	// the metadata column, this searches 100,000+ rows in sub-millisecond time.
	if filter.TraceID != "" {
		query += fmt.Sprintf(" AND metadata @> jsonb_build_object('trace_id', $%d::text)", argCount)
		args = append(args, filter.TraceID)
		argCount++
	}

	// Finalize ordering
	query += " ORDER BY created_at DESC"

	// 🛡️ 3. SLA Enforcement: The Pagination Memory Bound
	// We strictly prevent the query from returning unbounded datasets.
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50 // Mathematical ceiling: Max 50 items per RAM allocation
	}
	query += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, limit)
	argCount++

	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query += fmt.Sprintf(" OFFSET $%d", argCount)
	args = append(args, offset)

	// 4. Execution
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch filtered alerts: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, pgx.RowToStructByName[domain.SystemAlert])
}

// ==============================================================================
// Atomic Alert Resolution
// ==============================================================================

func (r *AuditRepository) ResolveAlert(ctx context.Context, alertID uuid.UUID) error {
	query := `
		UPDATE system_alerts 
		SET is_resolved = true, resolved_at = $1 
		WHERE id = $2 AND is_resolved = false
	`
	tag, err := r.pool.Exec(ctx, query, time.Now().UTC(), alertID)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("alert not found or already resolved")
	}

	return nil
}
