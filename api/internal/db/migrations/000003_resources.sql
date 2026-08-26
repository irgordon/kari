CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_id UUID,
    name TEXT GENERATED ALWAYS AS (domain_name) STORED,
    domain_name TEXT NOT NULL UNIQUE,
    document_root TEXT NOT NULL,
    ssl_status TEXT NOT NULL DEFAULT 'none'
        CHECK (ssl_status IN ('none', 'pending', 'active', 'renewing', 'failed', 'expired')),
    status TEXT NOT NULL DEFAULT 'provisioning'
        CHECK (status IN ('provisioning', 'active', 'failed', 'disabled')),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE RESTRICT,
    app_type TEXT NOT NULL CHECK (app_type IN ('nodejs', 'python', 'php', 'ruby', 'static')),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_user TEXT NOT NULL UNIQUE,
    repo_url TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    build_command TEXT NOT NULL,
    start_command TEXT NOT NULL,
    env_vars JSONB NOT NULL DEFAULT '{}'::jsonb,
    port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
    status TEXT NOT NULL DEFAULT 'stopped'
        CHECK (status IN ('stopped', 'starting', 'running', 'failed')),
    webhook_secret TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE domains
    ADD CONSTRAINT domains_app_id_fkey
    FOREIGN KEY (app_id) REFERENCES applications(id) ON DELETE SET NULL;

CREATE TABLE deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    domain_name TEXT NOT NULL,
    repo_url TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    build_command TEXT NOT NULL,
    target_port INTEGER NOT NULL CHECK (target_port BETWEEN 1 AND 65535),
    encrypted_ssh_key TEXT,
    env_vars JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'success', 'failed')),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE deployment_logs (
    id BIGSERIAL PRIMARY KEY,
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE system_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    severity TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'error', 'critical')),
    category TEXT NOT NULL,
    resource_id UUID NOT NULL,
    message TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_resolved BOOLEAN NOT NULL DEFAULT FALSE,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE system_profiles (
    id UUID PRIMARY KEY,
    singleton BOOLEAN NOT NULL DEFAULT TRUE UNIQUE CHECK (singleton),
    default_stack_registry JSONB NOT NULL,
    ssl_strategy TEXT NOT NULL,
    max_memory_per_app_mb INTEGER NOT NULL CHECK (max_memory_per_app_mb >= 128),
    max_cpu_percent_per_app INTEGER NOT NULL CHECK (max_cpu_percent_per_app BETWEEN 10 AND 100),
    default_firewall_policy TEXT NOT NULL,
    app_user_uid_range_start INTEGER NOT NULL,
    app_user_uid_range_end INTEGER NOT NULL,
    backup_retention_days INTEGER NOT NULL CHECK (backup_retention_days >= 0),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (app_user_uid_range_start < app_user_uid_range_end)
);

CREATE INDEX domains_user_id_idx ON domains(user_id);
CREATE INDEX domains_app_id_idx ON domains(app_id);
CREATE INDEX domains_ssl_renewal_idx ON domains(expires_at) WHERE ssl_status = 'active';
CREATE INDEX applications_domain_id_idx ON applications(domain_id);
CREATE INDEX applications_owner_id_idx ON applications(owner_id);
CREATE INDEX applications_env_vars_idx ON applications USING GIN (env_vars);
CREATE INDEX deployments_status_created_at_idx ON deployments(status, created_at);
CREATE INDEX deployments_app_id_idx ON deployments(app_id);
CREATE INDEX system_alerts_metadata_idx ON system_alerts USING GIN (metadata);
CREATE INDEX system_alerts_dashboard_idx ON system_alerts(is_resolved, severity, created_at DESC);

CREATE TRIGGER domains_set_updated_at
BEFORE UPDATE ON domains
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER applications_set_updated_at
BEFORE UPDATE ON applications
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER deployments_set_updated_at
BEFORE UPDATE ON deployments
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
