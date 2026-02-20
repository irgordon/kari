package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"kari/api/internal/core/domain"
)

// ApplicationRepo implements domain.ApplicationRepository for PostgreSQL
type ApplicationRepo struct {
	DB *sql.DB // The injected connection pool
}

// NewApplicationRepo is the factory function
func NewApplicationRepo(db *sql.DB) domain.ApplicationRepository {
	return &ApplicationRepo{DB: db}
}

// Create inserts a new application and scans the generated UUID and Timestamps back into the struct
func (r *ApplicationRepo) Create(ctx context.Context, app *domain.Application) error {
	query := `
		INSERT INTO applications (domain_id, app_type, repo_url, branch, build_command, start_command, env_vars, port)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, status, created_at, updated_at
	`

	// Convert the Go map into a JSON byte array for Postgres JSONB
	envVarsJSON, err := json.Marshal(app.EnvVars)
	if err != nil {
		return err
	}

	// Execute the query and map the database-generated defaults (UUID, timestamps) back to the pointer
	err = r.DB.QueryRowContext(ctx, query,
		app.DomainID,
		app.AppType,
		app.RepoURL,
		app.Branch,
		app.BuildCommand,
		app.StartCommand,
		envVarsJSON,
		app.Port,
	).Scan(&app.ID, &app.Status, &app.CreatedAt, &app.UpdatedAt)

	return err
}

// GetByID fetches an application, ensuring the Tenant actually owns the associated domain
func (r *ApplicationRepo) GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*domain.Application, error) {
	// We do an INNER JOIN on domains to enforce tenant isolation at the database level.
	// This physically prevents IDOR attacks.
	query := `
		SELECT a.id, a.domain_id, a.app_type, a.repo_url, a.branch, a.build_command, a.start_command, a.env_vars, a.port, a.status, a.created_at, a.updated_at
		FROM applications a
		INNER JOIN domains d ON a.domain_id = d.id
		WHERE a.id = $1 AND d.user_id = $2
	`

	var app domain.Application
	var envVarsJSON []byte

	err := r.DB.QueryRowContext(ctx, query, id, userID).Scan(
		&app.ID, &app.DomainID, &app.AppType, &app.RepoURL, &app.Branch,
		&app.BuildCommand, &app.StartCommand, &envVarsJSON, &app.Port,
		&app.Status, &app.CreatedAt, &app.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound // Return a domain-specific error, not a SQL error
		}
		return nil, err
	}

	// Unmarshal the JSONB back into the Go map
	if len(envVarsJSON) > 0 {
		if err := json.Unmarshal(envVarsJSON, &app.EnvVars); err != nil {
			return nil, err
		}
	}

	return &app, nil
}
