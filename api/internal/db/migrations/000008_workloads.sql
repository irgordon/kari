ALTER TABLE applications ADD COLUMN operation TEXT NOT NULL DEFAULT ''
    CHECK (operation IN ('', 'runtime', 'environment', 'database', 'files', 'sftp', 'backups'));
CREATE TABLE customer_databases (
    app_id UUID PRIMARY KEY REFERENCES applications(id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('provisioning', 'ready', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
