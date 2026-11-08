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
		       r.id, r.name, r.rank, u.email_verified_at IS NOT NULL, u.credential_version, u.mfa_secret IS NOT NULL
		FROM users u
		JOIN roles r ON u.role_id = r.id
		WHERE u.id = $1
	`
	var user domain.User
	var role domain.Role

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.IsActive, &user.CreatedAt, &user.UpdatedAt,
		&role.ID, &role.Name, &role.Rank, &user.EmailVerified, &user.CredentialVersion, &user.MFAEnabled,
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
		       r.id, r.name, r.rank, u.email_verified_at IS NOT NULL, u.credential_version, u.mfa_secret IS NOT NULL
		FROM users u
		JOIN roles r ON u.role_id = r.id
		WHERE u.email = $1
	`
	var user domain.User
	var role domain.Role

	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.IsActive, &user.CreatedAt, &user.UpdatedAt,
		&role.ID, &role.Name, &role.Rank, &user.EmailVerified, &user.CredentialVersion, &user.MFAEnabled,
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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = insertSession(ctx, tx, session); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertSession(ctx context.Context, tx pgx.Tx, session *domain.Session) error {
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, session.UserID).Scan(&id); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO auth_sessions (id,user_id,refresh_hash,expires_at,credential_version,mfa_pending,mfa_verified)
		SELECT $1,id,$3,$4,$5,$6,$7 FROM users WHERE id=$2 AND is_active AND credential_version=$5
		AND (mfa_secret IS NULL OR $6 OR $7)
		ON CONFLICT (user_id) WHERE mfa_pending DO UPDATE SET id=EXCLUDED.id,refresh_hash=EXCLUDED.refresh_hash,
		expires_at=EXCLUDED.expires_at,credential_version=EXCLUDED.credential_version,attempts=0,created_at=NOW(),revoked_at=NULL
		WHERE auth_sessions.created_at <= NOW()-INTERVAL '1 minute' OR auth_sessions.expires_at<=NOW()
		OR auth_sessions.revoked_at IS NOT NULL OR auth_sessions.credential_version!=EXCLUDED.credential_version`, session.ID, session.UserID, session.RefreshHash, session.ExpiresAt, session.CredentialVersion, session.MFAPending, session.MFAVerified)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		if session.MFAPending {
			return domain.ErrMFAChallengeBusy
		}
		return domain.ErrInvalidCredentials
	}
	return nil
}

func (r *UserRepo) FindSession(ctx context.Context, hash string) (*domain.Session, error) {
	var session domain.Session
	err := r.pool.QueryRow(ctx, `SELECT id, user_id, refresh_hash, expires_at, credential_version, mfa_verified FROM auth_sessions
		WHERE refresh_hash = $1 AND NOT mfa_pending AND revoked_at IS NULL AND expires_at > NOW()
		AND EXISTS (SELECT 1 FROM users u WHERE u.id = auth_sessions.user_id AND u.is_active
		AND u.credential_version = auth_sessions.credential_version AND (u.mfa_secret IS NULL OR auth_sessions.mfa_verified))`, hash).
		Scan(&session.ID, &session.UserID, &session.RefreshHash, &session.ExpiresAt, &session.CredentialVersion, &session.MFAVerified)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrInvalidCredentials
	}
	return &session, err
}

func (r *UserRepo) RotateSession(ctx context.Context, id uuid.UUID, oldHash, newHash string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE auth_sessions SET refresh_hash = $3
		WHERE id = $1 AND refresh_hash = $2 AND NOT mfa_pending AND revoked_at IS NULL AND expires_at > NOW()
		AND EXISTS (SELECT 1 FROM users WHERE users.id = auth_sessions.user_id AND is_active
		AND users.credential_version = auth_sessions.credential_version AND (users.mfa_secret IS NULL OR auth_sessions.mfa_verified))`, id, oldHash, newHash)
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
		WHERE s.id = $1 AND s.user_id = $2 AND NOT s.mfa_pending AND s.revoked_at IS NULL AND s.expires_at > NOW() AND u.is_active
		AND s.credential_version = u.credential_version AND (u.mfa_secret IS NULL OR s.mfa_verified))`, id, userID).Scan(&active)
	return active, err
}

// One row per account/purpose bounds token storage and enforces resend limits
// across clients and API processes. Only hashes of emailed capabilities persist.
func (r *UserRepo) IssueEmailToken(ctx context.Context, user *domain.User, purpose, hash string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var id uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, user.ID).Scan(&id)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO account_email_tokens
		(user_id, purpose, token_hash, credential_version, expires_at)
		SELECT id, $2, $3, credential_version, NOW() + INTERVAL '15 minutes' FROM users
		WHERE id = $1 AND is_active AND credential_version = $4
		AND ($2 = 'verify' OR email_verified_at IS NOT NULL)
		ON CONFLICT (user_id, purpose) DO UPDATE SET token_hash = EXCLUDED.token_hash,
		credential_version = EXCLUDED.credential_version, expires_at = EXCLUDED.expires_at,
		consumed_at = NULL, requested_at = NOW(),
		requests = CASE WHEN account_email_tokens.window_start <= NOW() - INTERVAL '1 hour'
		THEN 1 ELSE account_email_tokens.requests + 1 END,
		window_start = CASE WHEN account_email_tokens.window_start <= NOW() - INTERVAL '1 hour'
		THEN NOW() ELSE account_email_tokens.window_start END
		WHERE account_email_tokens.requested_at <= NOW() - INTERVAL '1 minute'
		AND (account_email_tokens.window_start <= NOW() - INTERVAL '1 hour' OR account_email_tokens.requests < 5)`,
		user.ID, purpose, hash, user.CredentialVersion)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, tx.Commit(ctx)
}

func (r *UserRepo) EmailTokenUser(ctx context.Context, hash, purpose string) (*domain.User, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM account_email_tokens
		WHERE token_hash = $1 AND purpose = $2 AND consumed_at IS NULL AND expires_at > NOW()`, hash, purpose).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

// Lock the account before consuming its capability, so reset, verification, and
// magic login serialize. Reset also invalidates sessions minted from old credentials.
func (r *UserRepo) ConsumeEmailToken(ctx context.Context, hash, purpose string, actor uuid.UUID, passwordHash string, session *domain.Session) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id uuid.UUID
	err = tx.QueryRow(ctx, `SELECT user_id FROM account_email_tokens WHERE token_hash = $1 AND purpose = $2`, hash, purpose).Scan(&id)
	if err == pgx.ErrNoRows {
		return domain.ErrInvalidCredentials
	}
	if err != nil {
		return err
	}
	var version int
	var active, verified bool
	err = tx.QueryRow(ctx, `SELECT credential_version, is_active, email_verified_at IS NOT NULL FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&version, &active, &verified)
	if err != nil {
		return err
	}
	if !active || (purpose == "verify" && actor != id) || (purpose != "verify" && !verified) {
		return domain.ErrInvalidCredentials
	}
	tag, err := tx.Exec(ctx, `UPDATE account_email_tokens SET consumed_at = NOW()
		WHERE token_hash = $1 AND purpose = $2 AND credential_version = $3
		AND consumed_at IS NULL AND expires_at > NOW()`, hash, purpose, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrInvalidCredentials
	}
	switch purpose {
	case "verify":
		_, err = tx.Exec(ctx, `UPDATE users SET email_verified_at = NOW() WHERE id = $1`, id)
	case "reset":
		_, err = tx.Exec(ctx, `UPDATE users SET password_hash = $2, credential_version = credential_version + 1,mfa_pending_secret=NULL,mfa_pending_until=NULL WHERE id = $1`, id, passwordHash)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at = NOW() WHERE user_id = $1`, id)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE account_email_tokens SET consumed_at = NOW() WHERE user_id = $1`, id)
		}
	case "magic":
		if session == nil || session.UserID != id || session.CredentialVersion != version {
			return domain.ErrInvalidCredentials
		}
		err = insertSession(ctx, tx, session)
	default:
		return domain.ErrInvalidCredentials
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
