-- api/internal/db/migrations/002_gitops_observability.sql
-- Focus: GitOps State Tracking and Action Center Observability

BEGIN;

-- ==============================================================================
-- 1. Deployment History (The GitOps Ledger)
-- ==============================================================================

CREATE TABLE IF NOT EXISTS deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    
    -- Traceability: Maps to the gRPC stream and frontend xterm.js session
    trace_id VARCHAR(255) UNIQUE NOT NULL,
    
    -- Git Metadata
    commit_hash VARCHAR(40),
    branch_name VARCHAR(100) NOT NULL,
    
    -- State Management
    -- 🛡️ SLA: Strict constraints to match Go Domain logic
    status VARCHAR(50) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'building', 'success', 'failed', 'cancelled')),
    
    -- Execution Timing for Performance Metrics
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for rapid history lookup per application
CREATE INDEX idx_deployments_app_id ON deployments(application_id);
-- Index for WebSocket lookups during active builds
CREATE INDEX idx_deployments_trace_id ON deployments(trace_id);

-- ==============================================================================
-- 2. System Alerts (The Action Center)
-- ==============================================================================

CREATE TABLE IF NOT EXISTS system_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- 🛡️ Severity & Categorization
    severity VARCHAR(50) NOT NULL 
        CHECK (severity IN ('info', 'warning', 'critical', 'fatal')),
    category VARCHAR(100) NOT NULL, -- e.g., 'ssl_expiry', 'disk_full', 'deploy_failed'
    
    -- Resource Linking: Optional UUIDs to link alert to specific objects
    domain_id UUID REFERENCES domains(id) ON DELETE SET NULL,
    app_id UUID REFERENCES applications(id) ON DELETE SET NULL,
    
    message TEXT NOT NULL,
    
    -- Observability Details
    -- 🛡️ Roadmap Feature: GIN-indexed JSONB for sub-10ms metadata searching
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    
    -- Resolution State
    is_resolved BOOLEAN NOT NULL DEFAULT false,
    resolved_at TIMESTAMPTZ,
    resolved_by UUID REFERENCES users(id) ON DELETE SET NULL,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 🛡️ Performance Optimization
-- This GIN index allows the Brain to search for specific trace_ids or error codes 
-- inside the metadata blob instantly.
CREATE INDEX idx_alerts_metadata_gin ON system_alerts USING GIN (metadata);
-- Index for the UI: Latest unresolved critical alerts first
CREATE INDEX idx_alerts_dashboard_priority ON system_alerts (is_resolved, severity, created_at DESC);

-- ==============================================================================
-- 3. Update Triggers
-- ==============================================================================

CREATE TRIGGER set_timestamp_deployments
BEFORE UPDATE ON deployments FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

CREATE TRIGGER set_timestamp_system_alerts
BEFORE UPDATE ON system_alerts FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

COMMIT;
