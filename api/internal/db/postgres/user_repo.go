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
		SELECT u.id, u.username, u.email, u.password_hash, u.is_active, u.created_at, u.updated_at,
		       r.id, r.name, r.rank
		FROM users u
		JOIN roles r ON u.role_id = r.id
		WHERE u.id = $1
	`
	var user domain.User
	var role domain.Role

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.IsActive, &user.CreatedAt, &user.UpdatedAt,
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
	return r.withPermissions(ctx, &user)
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT u.id, u.username, u.email, u.password_hash, u.is_active, u.created_at, u.updated_at,
		       r.id, r.name, r.rank
		FROM users u
		JOIN roles r ON u.role_id = r.id
		WHERE u.email = $1
	`
	var user domain.User
	var role domain.Role

	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.IsActive, &user.CreatedAt, &user.UpdatedAt,
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
	return r.withPermissions(ctx, &user)
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
	tag, err := r.pool.Exec(ctx, query, roleID, userID)
	if err != nil {
		return err
	}
	return requireAffectedRow(tag.RowsAffected())
}

func (r *UserRepo) withPermissions(ctx context.Context, user *domain.User) (*domain.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT p.resource || ':' || p.action
		FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = $1 ORDER BY p.resource, p.action`, user.RoleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	user.Permissions = []string{}
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		user.Permissions = append(user.Permissions, permission)
	}
	return user, rows.Err()
}

func (r *UserRepo) IsConfigured(ctx context.Context) (bool, error) {
	var configured bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM installation) OR EXISTS (SELECT 1 FROM users)`).Scan(&configured)
	return configured, err
}

// CompleteSetup serializes competing installers and commits the administrator and
// permanent completion marker together. No filesystem state controls setup.
func (r *UserRepo) CompleteSetup(ctx context.Context, email, passwordHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(126283426)`); err != nil {
		return err
	}
	var configured bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM installation) OR EXISTS (SELECT 1 FROM users)`).Scan(&configured); err != nil {
		return err
	}
	if configured {
		return domain.ErrConflict
	}
	tag, err := tx.Exec(ctx, `INSERT INTO users (username, email, password_hash, role_id)
		SELECT 'admin', $1, $2, id FROM roles WHERE name = 'superadmin'`, email, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	if _, err = tx.Exec(ctx, `INSERT INTO installation (singleton) VALUES (TRUE)`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *UserRepo) CreateSession(ctx context.Context, session *domain.Session) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO auth_sessions (id, user_id, refresh_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		session.ID, session.UserID, session.RefreshHash, session.ExpiresAt)
	return err
}

func (r *UserRepo) FindSession(ctx context.Context, hash string) (*domain.Session, error) {
	var session domain.Session
	err := r.pool.QueryRow(ctx, `SELECT id, user_id, refresh_hash, expires_at FROM auth_sessions
		WHERE refresh_hash = $1 AND revoked_at IS NULL AND expires_at > NOW()`, hash).
		Scan(&session.ID, &session.UserID, &session.RefreshHash, &session.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrInvalidCredentials
	}
	return &session, err
}

func (r *UserRepo) RotateSession(ctx context.Context, id uuid.UUID, oldHash, newHash string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE auth_sessions SET refresh_hash = $3
		WHERE id = $1 AND refresh_hash = $2 AND revoked_at IS NULL AND expires_at > NOW()
		AND EXISTS (SELECT 1 FROM users WHERE users.id = auth_sessions.user_id AND is_active)`, id, oldHash, newHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrInvalidCredentials
	}
	return nil
}

func (r *UserRepo) RevokeSession(ctx context.Context, hash string, sessionID, userID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at = NOW()
		WHERE refresh_hash = $1 OR (id = $2 AND user_id = $3)`, hash, sessionID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrInvalidCredentials
	}
	return nil
}

func (r *UserRepo) SessionActive(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	var active bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM auth_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = $1 AND s.user_id = $2 AND s.revoked_at IS NULL AND s.expires_at > NOW() AND u.is_active)`, id, userID).Scan(&active)
	return active, err
}
