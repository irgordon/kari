package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"kari/api/internal/core/domain"
)

type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

// HasPermission utilizes a 3-way join to verify access in a single atomic query.
func (r *UserRepo) HasPermission(ctx context.Context, userID uuid.UUID, resource string, action string) (bool, error) {
	// 🛡️ SLA: The query is structured to fail-fast. 
	// If the user is inactive or the role lacks the perm, the result is false.
	query := `
		SELECT EXISTS (
			SELECT 1 
			FROM users u
			JOIN roles r ON u.role_id = r.id
			JOIN role_permissions rp ON r.id = rp.role_id
			JOIN permissions p ON rp.permission_id = p.id
			WHERE u.id = $1 
			  AND u.is_active = true 
			  AND p.resource = $2 
			  AND p.action = $3
		)
	`

	var exists bool
	err := r.pool.QueryRow(ctx, query, userID, resource, action).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to verify permissions: %w", err)
	}

	return exists, nil
}

// GetByID fetches the user and eagerly loads their role metadata.
func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `
		SELECT u.id, u.email, u.password_hash, u.is_active, u.created_at, u.updated_at,
		       r.id, r.name, r.rank
		FROM users u
		JOIN roles r ON u.role_id = r.id
		WHERE u.id = $1
	`

	var user domain.User
	var role domain.Role

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.IsActive, &user.CreatedAt, &user.UpdatedAt,
		&role.ID, &role.Name, &role.Rank,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	user.Role = role
	return &user, nil
}
