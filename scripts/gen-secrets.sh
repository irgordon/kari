#!/bin/bash

# Karı Orchestration Engine - Secure Secret Generator
# 🛡️ SLA: Generate 2026-grade entropy for Zero-Trust boundaries

set -e

ENV_FILE=".env"

if [ -f "$ENV_FILE" ]; then
    echo "⚠️  $ENV_FILE already exists. Overwrite? (y/N)"
    read -r response
    if [[ ! "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        echo "Aborting."
        exit 1
    fi
fi

echo "🔐 Generating high-entropy secrets..."

# 1. Generate 32 bytes (256 bits) of random data and convert to 64 hex characters
# This is for the AES-GCM CryptoService.
ENC_KEY=$(openssl rand -hex 32)

# 2. Generate a 64-character URL-safe string for JWT signing
JWT_SEC=$(openssl rand -base64 48 | tr -d /=+ | cut -c1-64)

# 3. Generate a random password for the Database
DB_PASS=$(openssl rand -base64 24 | tr -d /=+)

# Write to .env
cat <<EOF > $ENV_FILE
# --- Auto-Generated Secrets ---
DB_PASSWORD=$DB_PASS
ENCRYPTION_KEY=$ENC_KEY
JWT_SECRET=$JWT_SEC

# --- System Defaults ---
KARI_EXPECTED_API_UID=1001
AGENT_SOCKET=/var/run/kari/agent.sock
KARI_WEB_ROOT=/var/www/kari
KARI_SYSTEMD_DIR=/etc/systemd/system
INTERNAL_API_URL=http://api:8080
PUBLIC_API_URL=http://localhost:8080
EOF

chmod 600 $ENV_FILE

echo "✅ $ENV_FILE generated and hardened (chmod 600)."
echo "🚀 You are now ready to run 'docker-compose up --build'."
