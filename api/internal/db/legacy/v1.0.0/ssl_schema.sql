CREATE TABLE ssl_certificates (
    id UUID PRIMARY KEY,
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    
    -- 🛡️ Metadata (Public Info Only)
    issuer TEXT NOT NULL,           -- e.g., "Let's Encrypt"
    common_name TEXT NOT NULL,      -- The domain name the cert is valid for
    
    -- 🛡️ SLA: Renewal Tracking
    issued_at TIMESTAMP WITH TIME ZONE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    last_renewal_attempt TIMESTAMP WITH TIME ZONE,
    
    -- 🛡️ Zero-Trust Status
    -- 'active', 'expiring', 'failed', 'revoked'
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'expiring', 'failed', 'revoked')),
    
    -- 🛡️ Error Telemetry
    last_error TEXT,                -- Captures ACME challenge failures for the Action Center
    
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- 🛡️ Performance: Index for the Auto-Renewal Worker
-- Allows the Brain to instantly find certs expiring in the next 30 days.
CREATE INDEX idx_ssl_expiry ON ssl_certificates(expires_at) WHERE status = 'active';
CREATE INDEX idx_ssl_domain_id ON ssl_certificates(domain_id);
