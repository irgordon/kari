package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

// HasPermission verifies access via indexed 3-way join.
func (r *UserRepo) HasPermission(ctx context.Context, userID uuid.UUID, resource string, action string) (bool, error) {
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
	return exists, err
}

// GetByID fetches user + role metadata.
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

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT u.id, u.email, u.password_hash, u.is_active, u.created_at, u.updated_at,
		       r.id, r.name, r.rank
		FROM users u
		JOIN roles r ON u.role_id = r.id
		WHERE u.email = $1
	`
	var user domain.User
	var role domain.Role

	err := r.pool.QueryRow(ctx, query, email).Scan(
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
	user.RoleID = role.ID
	user.Rank = fmt.Sprintf("%d", role.Rank)
	user.Permissions = []string{}
	return &user, nil
}

// 🛡️ UpdateRefreshToken persists high-entropy tokens for session rotation.
func (r *UserRepo) UpdateRefreshToken(ctx context.Context, id uuid.UUID, token string) error {
	query := `UPDATE users SET refresh_token = $1, updated_at = NOW() WHERE id = $2`
	tag, err := r.pool.Exec(ctx, query, token, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// 🛡️ GetRoleByID allows RoleService to verify ranks before assignment.
func (r *UserRepo) GetRoleByID(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
	query := `SELECT id, name, rank FROM roles WHERE id = $1`
	var role domain.Role
	err := r.pool.QueryRow(ctx, query, id).Scan(&role.ID, &role.Name, &role.Rank)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	return &role, err
}

// 🛡️ CountAdmins provides a fail-fast check for the "Last Admin" protection logic.
func (r *UserRepo) CountAdmins(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM users u JOIN roles r ON u.role_id = r.id WHERE r.rank = 0 AND u.is_active = true`
	var count int
	err := r.pool.QueryRow(ctx, query).Scan(&count)
	return count, err
}

// 🛡️ UpdateUserRole handles the actual promotion/demotion after service-layer rank checks.
func (r *UserRepo) UpdateUserRole(ctx context.Context, userID uuid.UUID, roleID uuid.UUID) error {
	query := `UPDATE users SET role_id = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.pool.Exec(ctx, query, roleID, userID)
	return err
}
