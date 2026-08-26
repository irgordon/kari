INSERT INTO roles (id, name, description, rank) VALUES
    ('00000000-0000-0000-0000-000000000001', 'superadmin', 'Unrestricted platform administrator', 0),
    ('00000000-0000-0000-0000-000000000002', 'admin', 'Platform administrator', 10),
    ('00000000-0000-0000-0000-000000000003', 'operator', 'Application and domain operator', 50),
    ('00000000-0000-0000-0000-000000000004', 'viewer', 'Read-only operator', 100);

INSERT INTO permissions (id, resource, action, description) VALUES
    ('10000000-0000-0000-0000-000000000001', 'domains', 'read', 'View domains'),
    ('10000000-0000-0000-0000-000000000002', 'domains', 'write', 'Create and update domains'),
    ('10000000-0000-0000-0000-000000000003', 'domains', 'delete', 'Delete domains'),
    ('10000000-0000-0000-0000-000000000004', 'applications', 'read', 'View applications'),
    ('10000000-0000-0000-0000-000000000005', 'applications', 'write', 'Create and update applications'),
    ('10000000-0000-0000-0000-000000000006', 'applications', 'deploy', 'Deploy applications'),
    ('10000000-0000-0000-0000-000000000007', 'applications', 'delete', 'Delete applications'),
    ('10000000-0000-0000-0000-000000000008', 'audit_logs', 'read', 'View audit logs'),
    ('10000000-0000-0000-0000-000000000009', 'server', 'manage', 'Manage server settings');

INSERT INTO role_permissions (role_id, permission_id)
SELECT '00000000-0000-0000-0000-000000000001', id FROM permissions;

INSERT INTO role_permissions (role_id, permission_id)
SELECT '00000000-0000-0000-0000-000000000002', id FROM permissions;

INSERT INTO role_permissions (role_id, permission_id)
SELECT '00000000-0000-0000-0000-000000000003', id
FROM permissions
WHERE (resource, action) IN (
    ('domains', 'read'),
    ('domains', 'write'),
    ('applications', 'read'),
    ('applications', 'write'),
    ('applications', 'deploy'),
    ('audit_logs', 'read')
);

INSERT INTO role_permissions (role_id, permission_id)
SELECT '00000000-0000-0000-0000-000000000004', id
FROM permissions
WHERE (resource, action) IN (
    ('domains', 'read'),
    ('applications', 'read'),
    ('audit_logs', 'read')
);

INSERT INTO system_profiles (
    id,
    default_stack_registry,
    ssl_strategy,
    max_memory_per_app_mb,
    max_cpu_percent_per_app,
    default_firewall_policy,
    app_user_uid_range_start,
    app_user_uid_range_end,
    backup_retention_days
) VALUES (
    '20000000-0000-0000-0000-000000000001',
    '{"nodejs":"node:22","python":"python:3.13","php":"php:8.4","ruby":"ruby:3.4"}',
    'manual',
    512,
    50,
    'deny-by-default',
    20000,
    29999,
    30
);
