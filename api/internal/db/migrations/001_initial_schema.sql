-- 001_initial_schema.sql
-- Hardened for Karı 2026 Environment

-- Enable pgcrypto for UUIDv4 generation
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ==============================================================================
-- 1. Updated-At Trigger Function
-- ==============================================================================
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ==============================================================================
-- 2. Identity & Access Management (RBAC)
-- Replacing user_role ENUM with a dynamic Roles system for SOLID compliance.
-- ==============================================================================

CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) UNIQUE NOT NULL, -- e.g., 'superadmin', 'tenant'
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource VARCHAR(100) NOT NULL, -- e.g., 'applications', 'ssl', 'system'
    action VARCHAR(100) NOT NULL,   -- e.g., 'read', 'write', 'deploy'
    UNIQUE(resource, action)
);

CREATE TABLE role_permissions (
    role_id UUID REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- ==============================================================================
-- 3. Core Tables
-- ==============================================================================

-- USERS Table: Linked to the Dynamic RBAC system
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role_id UUID REFERENCES roles(id) ON DELETE RESTRICT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_timestamp_users
BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- DOMAINS Table: Tracks virtual host state
-- Hardened: Added check constraint for ssl_status to allow zero-downtime updates
CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    domain_name VARCHAR(255) UNIQUE NOT NULL,
    ssl_status VARCHAR(50) NOT NULL DEFAULT 'none' 
        CHECK (ssl_status IN ('none', 'active', 'failed', 'renewing')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_domains_user_id ON domains(user_id);
CREATE TRIGGER set_timestamp_domains
BEFORE UPDATE ON domains FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- APPLICATIONS Table: The core state for modern runtime workflows
CREATE TABLE applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    repo_url VARCHAR(1024) NOT NULL,
    branch VARCHAR(100) NOT NULL DEFAULT 'main',
    build_command VARCHAR(500),
    start_command VARCHAR(500),
    
    -- Networking & Jail Identity
    -- Port constrained to non-privileged range
    port INTEGER UNIQUE CHECK (port > 1024 AND port < 65536),
    app_user VARCHAR(100) UNIQUE NOT NULL, -- Matched to kari-app-{id}
    
    env_vars JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(50) NOT NULL DEFAULT 'stopped'
        CHECK (status IN ('running', 'stopped', 'deploying', 'failed')),
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_applications_domain_id ON applications(domain_id);
-- GIN Index for rapid environment variable lookups
CREATE INDEX idx_applications_env_vars ON applications USING GIN (env_vars);

CREATE TRIGGER set_timestamp_applications
BEFORE UPDATE ON applications FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- DEPLOYMENTS Table: Audit log for GitOps
CREATE TABLE deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    trace_id VARCHAR(255) UNIQUE NOT NULL, 
    status VARCHAR(50) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'building', 'success', 'failed')),
    build_logs TEXT, 
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_deployments_trace_id ON deployments(trace_id);

-- ==============================================================================
-- 4. The Action Center (NEW)
-- Centralized alerting with high-performance metadata search
-- ==============================================================================

CREATE TABLE system_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    severity VARCHAR(50) NOT NULL 
        CHECK (severity IN ('info', 'warning', 'critical', 'fatal')),
    category VARCHAR(100) NOT NULL, -- 'ssl', 'system', 'gitops'
    resource_id VARCHAR(255),       -- Optional link to domain_name or app_id
    message TEXT NOT NULL,
    is_resolved BOOLEAN NOT NULL DEFAULT false,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

-- 🛡️ GIN Index: Powers sub-10ms metadata searches for the Action Center
CREATE INDEX idx_system_alerts_metadata ON system_alerts USING GIN (metadata);
-- Compound Index: Optimizes UI filtering for unresolved critical alerts
CREATE INDEX idx_system_alerts_ui ON system_alerts (is_resolved, severity, created_at DESC);
