-- 001_initial_schema.sql

-- Enable the pgcrypto extension for UUID generation (if not using PG 13+ native gen_random_uuid())
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ==============================================================================
-- 1. ENUM Definitions
-- Enforcing strict states at the database level prevents bad data from the API
-- ==============================================================================
CREATE TYPE user_role AS ENUM ('admin', 'tenant');
CREATE TYPE user_status AS ENUM ('active', 'suspended');

CREATE TYPE ssl_status AS ENUM ('none', 'active', 'failed', 'renewing');

CREATE TYPE app_type AS ENUM ('nodejs', 'python', 'php', 'ruby', 'static');
CREATE TYPE app_status AS ENUM ('running', 'stopped', 'deploying', 'failed');

CREATE TYPE deployment_status AS ENUM ('pending', 'building', 'success', 'failed');

-- ==============================================================================
-- 2. Trigger Function for updated_at
-- Automatically bumps the updated_at timestamp on row modification
-- ==============================================================================
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ==============================================================================
-- 3. Core Tables
-- ==============================================================================

-- USERS Table: Manages tenant access and high-level limits
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role user_role NOT NULL DEFAULT 'tenant',
    status user_status NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_timestamp_users
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- DOMAINS Table: Maps virtual hosts and SSL state
CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    domain_name VARCHAR(255) UNIQUE NOT NULL,
    document_root VARCHAR(512) NOT NULL,
    ssl_status ssl_status NOT NULL DEFAULT 'none',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_domains_user_id ON domains(user_id);

CREATE TRIGGER set_timestamp_domains
BEFORE UPDATE ON domains
FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- APPLICATIONS Table: The core state for modern runtime workflows
CREATE TABLE applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    app_type app_type NOT NULL,
    repo_url VARCHAR(512),
    branch VARCHAR(100) DEFAULT 'main',
    build_command VARCHAR(255),
    start_command VARCHAR(255),
    env_vars JSONB DEFAULT '{}'::jsonb,
    port INTEGER UNIQUE, -- The internal loopback port Nginx reverse proxies to
    status app_status NOT NULL DEFAULT 'stopped',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_applications_domain_id ON applications(domain_id);

CREATE TRIGGER set_timestamp_applications
BEFORE UPDATE ON applications
FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- DEPLOYMENTS Table: Audit log and state tracking for GitOps workflows
CREATE TABLE deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    commit_hash VARCHAR(40),
    status deployment_status NOT NULL DEFAULT 'pending',
    build_logs TEXT, -- Stores final logs after stream completes
    trace_id VARCHAR(255) UNIQUE NOT NULL, -- Maps to the gRPC trace for WebSocket streaming
    deployed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_deployments_application_id ON deployments(application_id);
CREATE INDEX idx_deployments_trace_id ON deployments(trace_id);
