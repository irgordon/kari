ALTER TABLE users ADD COLUMN mfa_secret TEXT;
ALTER TABLE users ADD COLUMN mfa_pending_secret TEXT;
ALTER TABLE users ADD COLUMN mfa_pending_until TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN mfa_last_counter BIGINT NOT NULL DEFAULT -1;
ALTER TABLE auth_sessions ADD COLUMN mfa_pending BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE auth_sessions ADD COLUMN mfa_verified BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE auth_sessions ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE auth_sessions ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;
CREATE UNIQUE INDEX one_pending_mfa_per_user ON auth_sessions(user_id) WHERE mfa_pending;
CREATE TABLE mfa_recovery_codes (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL CHECK (length(code_hash) = 64),
    PRIMARY KEY (user_id, code_hash)
);
