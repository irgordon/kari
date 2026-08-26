BEGIN;

-- 1. Roles Table
CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(50) UNIQUE NOT NULL,
    description TEXT,
    rank INTEGER DEFAULT 10, -- 🛡️ SLA: 0 is highest (Super Admin), 100 is lowest.
    is_system BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 2. Permissions Table
CREATE TABLE IF NOT EXISTS permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource VARCHAR(50) NOT NULL, 
    action VARCHAR(50) NOT NULL,   
    description TEXT,
    UNIQUE(resource, action)
);

-- 3. Role-Permission Mapping
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id UUID REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- 4. User-Role Assignment (Hardened)
-- We add the column, then set a default role for existing users before making it NOT NULL.
ALTER TABLE users ADD COLUMN IF NOT EXISTS role_id UUID REFERENCES roles(id);

-- 🛡️ 5. Updated_at Trigger for Roles
CREATE TRIGGER set_timestamp_roles
BEFORE UPDATE ON roles FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- ==============================================================================
-- Seed Data: The "Super Admin" Safety Net
-- ==============================================================================

-- Create the Super Admin (Rank 0) and Default Tenant (Rank 50)
INSERT INTO roles (name, description, rank, is_system) 
VALUES 
    ('Super Admin', 'Total system authority. Immutable.', 0, true),
    ('Tenant', 'Standard user with access to owned resources.', 50, true)
ON CONFLICT (name) DO NOTHING;

-- Seed baseline permissions
INSERT INTO permissions (resource, action, description) VALUES
    ('applications', 'read', 'View deployment list and status'),
    ('applications', 'write', 'Create and edit application settings'),
    ('applications', 'deploy', 'Trigger manual GitOps deployments'),
    ('domains', 'read', 'View configured virtual hosts'),
    ('domains', 'ssl', 'Provision Let''s Encrypt certificates'),
    ('audit', 'read', 'Access system alerts and tenant logs'),
    ('rbac', 'manage', 'Modify roles and permissions')
ON CONFLICT (resource, action) DO NOTHING;

-- 🛡️ Atomic Permission Granting
-- Grant all permissions to Super Admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'Super Admin'
ON CONFLICT DO NOTHING;

-- Grant limited permissions to Default Tenant
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'Tenant' AND p.resource IN ('applications', 'domains')
ON CONFLICT DO NOTHING;

COMMIT;
