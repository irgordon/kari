package postgres

import (
	"context"
	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
)

type MFAState struct {
	Secret, Pending string
	Version         int
}

func (r *UserRepo) MFAState(ctx context.Context, id uuid.UUID) (MFAState, error) {
	var state MFAState
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(mfa_secret,''),CASE WHEN mfa_pending_until>NOW() THEN COALESCE(mfa_pending_secret,'') ELSE '' END,credential_version FROM users WHERE id=$1 AND is_active`, id).Scan(&state.Secret, &state.Pending, &state.Version)
	return state, err
}

func (r *UserRepo) BeginMFA(ctx context.Context, id uuid.UUID, version int, secret string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE users SET mfa_pending_secret=$3,mfa_pending_until=NOW()+INTERVAL '10 minutes'
		WHERE id=$1 AND credential_version=$2 AND is_active AND mfa_secret IS NULL`, id, version, secret)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrInvalidCredentials
	}
	return nil
}

func (r *UserRepo) PendingMFA(ctx context.Context, hash string) (*domain.Session, error) {
	var s domain.Session
	err := r.pool.QueryRow(ctx, `SELECT id,user_id,credential_version FROM auth_sessions WHERE refresh_hash=$1
		AND mfa_pending AND revoked_at IS NULL AND expires_at>NOW() AND attempts<5`, hash).Scan(&s.ID, &s.UserID, &s.CredentialVersion)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	return &s, nil
}

// Invalid attempts are committed, while successful proof and session promotion
// happen in the same transaction. The account lock also serializes OTP/code reuse.
func (r *UserRepo) CompleteMFA(ctx context.Context, pendingHash string, s *domain.Session, counter int64, recoveryHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var version int
	var last int64
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT credential_version,mfa_last_counter,mfa_secret IS NOT NULL FROM users WHERE id=$1 AND is_active FOR UPDATE`, s.UserID).Scan(&version, &last, &enabled)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE auth_sessions SET attempts=attempts+1 WHERE id=$1 AND user_id=$2 AND refresh_hash=$3
		AND mfa_pending AND revoked_at IS NULL AND expires_at>NOW() AND attempts<5 AND credential_version=$4`, s.ID, s.UserID, pendingHash, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 || version != s.CredentialVersion || !enabled {
		return domain.ErrInvalidCredentials
	}
	valid := counter > last && counter >= 0
	if recoveryHash != "" {
		tag, err = tx.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id=$1 AND code_hash=$2`, s.UserID, recoveryHash)
		if err != nil {
			return err
		}
		valid = tag.RowsAffected() == 1
	}
	if !valid {
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return domain.ErrInvalidCredentials
	}
	if counter >= 0 {
		_, err = tx.Exec(ctx, `UPDATE users SET mfa_last_counter=$2 WHERE id=$1`, s.UserID, counter)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE auth_sessions SET mfa_pending=FALSE,mfa_verified=TRUE,refresh_hash=$2,expires_at=$3 WHERE id=$1`, s.ID, s.RefreshHash, s.ExpiresAt)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *UserRepo) ChangeMFA(ctx context.Context, id uuid.UUID, version int, pending string, counter int64, hashes []string, session *domain.Session) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var last int64
	var current string
	err = tx.QueryRow(ctx, `SELECT mfa_last_counter,COALESCE(mfa_secret,'') FROM users WHERE id=$1 AND credential_version=$2 AND is_active FOR UPDATE`, id, version).Scan(&last, &current)
	if err != nil {
		return domain.ErrInvalidCredentials
	}
	if counter <= last || counter < 0 {
		return domain.ErrInvalidCredentials
	}
	var tagCount int64
	if pending != "" {
		tag, e := tx.Exec(ctx, `UPDATE users SET mfa_secret=$3,mfa_pending_secret=NULL,mfa_pending_until=NULL,mfa_last_counter=$4,credential_version=credential_version+1
			WHERE id=$1 AND credential_version=$2 AND mfa_secret IS NULL AND mfa_pending_secret=$3 AND mfa_pending_until>NOW()`, id, version, pending, counter)
		err = e
		tagCount = tag.RowsAffected()
	} else if current != "" {
		tag, e := tx.Exec(ctx, `UPDATE users SET mfa_secret=NULL,mfa_last_counter=-1,credential_version=credential_version+1 WHERE id=$1`, id)
		err = e
		tagCount = tag.RowsAffected()
	}
	if err != nil {
		return err
	}
	if tagCount != 1 {
		return domain.ErrInvalidCredentials
	}
	if _, err = tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at=NOW() WHERE user_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE account_email_tokens SET consumed_at=NOW() WHERE user_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id=$1`, id); err != nil {
		return err
	}
	for _, hash := range hashes {
		if _, err = tx.Exec(ctx, `INSERT INTO mfa_recovery_codes(user_id,code_hash) VALUES($1,$2)`, id, hash); err != nil {
			return err
		}
	}
	if session != nil {
		if err = insertSession(ctx, tx, session); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Only the local operator CLI exposes this operation, never an HTTP route.
func (r *UserRepo) EmergencyRecover(ctx context.Context, email, passwordHash string, resetMFA bool) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id uuid.UUID
	err = tx.QueryRow(ctx, `SELECT u.id FROM users u JOIN roles r ON r.id=u.role_id WHERE u.email=$1 AND r.rank=0 FOR UPDATE OF u`, email).Scan(&id)
	if err != nil {
		return domain.ErrNotFound
	}
	_, err = tx.Exec(ctx, `UPDATE users SET password_hash=$2,is_active=TRUE,credential_version=credential_version+1,
		mfa_secret=CASE WHEN $3 THEN NULL ELSE mfa_secret END,mfa_last_counter=CASE WHEN $3 THEN -1 ELSE mfa_last_counter END,mfa_pending_secret=NULL,mfa_pending_until=NULL WHERE id=$1`, id, passwordHash, resetMFA)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at=NOW() WHERE user_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE account_email_tokens SET consumed_at=NOW() WHERE user_id=$1`, id); err != nil {
		return err
	}
	if resetMFA {
		if _, err = tx.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id=$1`, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
