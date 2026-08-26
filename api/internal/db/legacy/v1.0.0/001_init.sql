-- ==============================================================================
-- 1. IAM Refinements
-- ==============================================================================

-- Add Rank to Roles (0 = SuperAdmin, higher = lower power)
ALTER TABLE roles ADD COLUMN rank INTEGER NOT NULL DEFAULT 100;

-- Add Session Management to Users
ALTER TABLE users ADD COLUMN refresh_token TEXT;

-- Add Ownership to Domains (Tenant Isolation)
ALTER TABLE domains ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE CASCADE;

-- ==============================================================================
-- 2. Application Logic Refinements
-- ==============================================================================

-- Link Apps to Owners directly for sub-10ms Rank-checks
ALTER TABLE applications ADD COLUMN owner_id UUID REFERENCES users(id) ON DELETE CASCADE;

-- Add Status constraints to applications (Matching our Go domain model)
ALTER TABLE applications ADD COLUMN status VARCHAR(50) NOT NULL DEFAULT 'stopped'
    CHECK (status IN ('stopped', 'starting', 'running', 'failed', 'deleting'));

-- ==============================================================================
-- 3. Advanced Observability (Action Center)
-- ==============================================================================

-- Ensure we can search alerts by the user who resolved them (for accountability)
-- This supports: metadata -> 'resolved_by'
CREATE INDEX idx_system_alerts_category ON system_alerts (category, created_at DESC);
