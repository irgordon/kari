-- 🛡️ SLA: Use UUIDs for public-facing IDs and BigInt for internal ordering
CREATE TABLE deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id UUID NOT NULL,
    domain_name TEXT NOT NULL,
    repo_url TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    build_command TEXT NOT NULL,
    target_port INTEGER NOT NULL,
    encrypted_ssh_key TEXT,
    status TEXT NOT NULL DEFAULT 'PENDING',
    version INTEGER NOT NULL DEFAULT 1, -- 🛡️ Optimistic Concurrency Control
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 🛡️ Performance: Separate logs table to prevent bloating the main table
CREATE TABLE deployment_logs (
    id SERIAL PRIMARY KEY,
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_deployments_status ON deployments(status) WHERE status = 'PENDING';
