ALTER TABLE users ADD COLUMN email_verified_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN credential_version INTEGER NOT NULL DEFAULT 0 CHECK (credential_version >= 0);
ALTER TABLE auth_sessions ADD COLUMN credential_version INTEGER NOT NULL DEFAULT 0;

CREATE TABLE account_email_tokens (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL CHECK (purpose IN ('verify', 'reset', 'magic')),
    token_hash TEXT NOT NULL UNIQUE CHECK (length(token_hash) = 64),
    credential_version INTEGER NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    window_start TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    requests INTEGER NOT NULL DEFAULT 1 CHECK (requests BETWEEN 1 AND 5),
    PRIMARY KEY (user_id, purpose)
);
