package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/irgordon/kari/api/internal/core/domain"
)

type ApplicationRepo struct {
	pool *pgxpool.Pool
}

func NewApplicationRepo(pool *pgxpool.Pool) domain.ApplicationRepository {
	return &ApplicationRepo{pool: pool}
}

func (r *ApplicationRepo) Create(ctx context.Context, app *domain.Application) error {
	const query = `
		INSERT INTO applications (
			name, domain_id, app_type, owner_id, app_user, repo_url, branch,
			build_command, start_command, env_vars, port, status, webhook_secret
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, updated_at`
	err := r.pool.QueryRow(ctx, query,
		app.Name, app.DomainID, app.AppType, app.OwnerID, app.AppUser,
		app.RepoURL, app.Branch, app.BuildCommand, app.StartCommand,
		app.EnvVars, app.Port, app.Status, app.WebhookSecret,
	).Scan(&app.ID, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create application: %w", err)
	}
	return nil
}

func (r *ApplicationRepo) GetByIDWithMetadata(ctx context.Context, id uuid.UUID) (*domain.ApplicationMetadata, error) {
	const query = `
		SELECT a.id, a.name, a.domain_id, d.domain_name, d.user_id, owner_role.rank
		FROM applications a
		JOIN domains d ON a.domain_id = d.id
		JOIN users owner_user ON d.user_id = owner_user.id
		JOIN roles owner_role ON owner_user.role_id = owner_role.id
		WHERE a.id = $1`
	var metadata domain.ApplicationMetadata
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&metadata.ID, &metadata.Name, &metadata.DomainID, &metadata.DomainName,
		&metadata.OwnerID, &metadata.OwnerRank,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get application metadata: %w", err)
	}
	return &metadata, nil
}

func (r *ApplicationRepo) GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*domain.Application, error) {
	const query = applicationSelect + ` WHERE a.id = $1 AND d.user_id = $2`
	app, err := scanApplication(r.pool.QueryRow(ctx, query, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get application: %w", err)
	}
	return app, nil
}

func (r *ApplicationRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM applications WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete application: %w", err)
	}
	return requireAffectedRow(tag.RowsAffected())
}

func (r *ApplicationRepo) UpdateEnvVars(ctx context.Context, id uuid.UUID, envVars map[string]string) error {
	tag, err := r.pool.Exec(ctx,
		"UPDATE applications SET env_vars = $1 WHERE id = $2",
		envVars,
		id,
	)
	if err != nil {
		return fmt.Errorf("update application environment: %w", err)
	}
	return requireAffectedRow(tag.RowsAffected())
}

func (r *ApplicationRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	tag, err := r.pool.Exec(ctx,
		"UPDATE applications SET status = $1 WHERE id = $2",
		status,
		id,
	)
	if err != nil {
		return fmt.Errorf("update application status: %w", err)
	}
	return requireAffectedRow(tag.RowsAffected())
}

func (r *ApplicationRepo) ListAllActive(ctx context.Context) ([]domain.Application, error) {
	rows, err := r.pool.Query(ctx, applicationSelect+` WHERE a.status = 'running' ORDER BY a.created_at`)
	if err != nil {
		return nil, fmt.Errorf("list active applications: %w", err)
	}
	defer rows.Close()
	return collectApplications(rows)
}

const applicationSelect = `
	SELECT a.id, a.name, a.domain_id, a.app_type, d.domain_name, a.owner_id,
	       a.app_user, a.repo_url, a.branch, a.build_command, a.start_command,
	       a.env_vars, a.port, a.status, a.webhook_secret, a.created_at, a.updated_at
	FROM applications a
	JOIN domains d ON a.domain_id = d.id`

type applicationRow interface {
	Scan(dest ...any) error
}

func scanApplication(row applicationRow) (*domain.Application, error) {
	var app domain.Application
	err := row.Scan(
		&app.ID, &app.Name, &app.DomainID, &app.AppType, &app.DomainName,
		&app.OwnerID, &app.AppUser, &app.RepoURL, &app.Branch,
		&app.BuildCommand, &app.StartCommand, &app.EnvVars, &app.Port,
		&app.Status, &app.WebhookSecret, &app.CreatedAt, &app.UpdatedAt,
	)
	return &app, err
}

func collectApplications(rows pgx.Rows) ([]domain.Application, error) {
	applications := make([]domain.Application, 0)
	for rows.Next() {
		app, err := scanApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}
		applications = append(applications, *app)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applications: %w", err)
	}
	return applications, nil
}

func requireAffectedRow(rowsAffected int64) error {
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
