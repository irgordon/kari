// api/internal/db/postgres/application_repo.go
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ApplicationRepo struct {
	pool *pgxpool.Pool
}

func NewApplicationRepo(pool *pgxpool.Pool) domain.ApplicationRepository {
	return &ApplicationRepo{pool: pool}
}

// Create persists the app and the unprivileged OS user identity
func (r *ApplicationRepo) Create(ctx context.Context, app *domain.Application) error {
	query := `
		INSERT INTO applications (domain_id, repo_url, branch, build_command, start_command, env_vars, port, app_user, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`
	err := r.pool.QueryRow(ctx, query,
		app.DomainID, app.RepoURL, app.Branch, app.BuildCommand,
		app.StartCommand, app.EnvVars, app.Port, app.AppUser, app.Status,
	).Scan(&app.ID, &app.CreatedAt, &app.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}
	return nil
}

// GetByIDWithMetadata performs a 3-way join to support Rank-Based Authorization logic.
func (r *ApplicationRepo) GetByIDWithMetadata(ctx context.Context, id uuid.UUID) (*domain.ApplicationMetadata, error) {
	// 🛡️ SLA: Single trip to DB to get everything needed for Authorization
	query := `
		SELECT 
			a.id, a.domain_id, d.domain_name, d.user_id as owner_id, 
			r.rank as owner_rank
		FROM applications a
		JOIN domains d ON a.domain_id = d.id
		JOIN users u ON d.user_id = u.id
		JOIN roles r ON u.role_id = r.id
		WHERE a.id = $1
	`
	var meta domain.ApplicationMetadata
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&meta.ID, &meta.DomainID, &meta.DomainName, &meta.OwnerID, &meta.OwnerRank,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to fetch app metadata: %w", err)
	}
	return &meta, nil
}

// GetByID remains for standard UI lookups with strict ownership filtering
func (r *ApplicationRepo) GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*domain.Application, error) {
	query := `
		SELECT a.id, a.domain_id, a.repo_url, a.branch, a.build_command, a.start_command, a.env_vars, a.port, a.app_user, a.status, a.created_at, a.updated_at
		FROM applications a
		INNER JOIN domains d ON a.domain_id = d.id
		WHERE a.id = $1 AND d.user_id = $2
	`
	// Using pgx.CollectOneRow with RowToStructByName for cleaner mapping
	rows, err := r.pool.Query(ctx, query, id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	app, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[domain.Application])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &app, nil
}

// Delete removes the application record. The Service layer handles the Muscle cleanup first.
func (r *ApplicationRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM applications WHERE id = $1`
	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete application: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ApplicationRepo) UpdateEnvVars(ctx context.Context, id uuid.UUID, envVars map[string]string) error {
	query := `UPDATE applications SET env_vars = $1, updated_at = NOW() WHERE id = $2`
	tag, err := r.pool.Exec(ctx, query, envVars, id)
	if err != nil {
		return fmt.Errorf("failed to update application env vars: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ApplicationRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `UPDATE applications SET status = $1, updated_at = NOW() WHERE id = $2`
	tag, err := r.pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update application status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ApplicationRepo) ListAllActive(ctx context.Context) ([]domain.Application, error) {
	query := `
		SELECT id, domain_id, repo_url, branch, build_command, start_command, env_vars, port, app_user, status, created_at, updated_at
		FROM applications
		WHERE status = 'running'
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list active applications: %w", err)
	}
	defer rows.Close()

	apps, err := pgx.CollectRows(rows, pgx.RowToStructByName[domain.Application])
	if err != nil {
		return nil, fmt.Errorf("failed to collect active applications: %w", err)
	}
	return apps, nil
}
