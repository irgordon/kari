#!/bin/bash
# ==============================================================================
# 🛡️ Kari Hardened Idempotent Installer
# ==============================================================================
set -euo pipefail # 🛡️ SLA: Exit on error, unset variables, or pipe failure

# ... [Brand Colors & ASCII Art] ...

# 1. 🛡️ Identity & Directory Provisioning
echo -e "${GRAY}[1/5] Provisioning secure users and paths...${NC}"

# Create the Brain's identity
if ! id "kari-api" &>/dev/null; then
    useradd -r -s /bin/false kari-api
fi

# Ensure 027 umask so new files are never world-readable by default
umask 027

# Setup secure directories with strict ownership
mkdir -p /etc/kari/ssl
mkdir -p /var/run/kari
mkdir -p /var/www/kari # 🛡️ SLA: Consistent with our config.rs refactor
mkdir -p /opt/kari/bin

# 2. 🛡️ Permission Hardening
# Muscle (Rust) owns the SSL storage
chown root:root /etc/kari/ssl
chmod 700 /etc/kari/ssl

# Brain (Go) must own its runtime socket dir to manage the .sock lifecycle
# We give the group to kari-api so it can create/delete the socket file
chown kari-api:root /var/run/kari
chmod 750 /var/run/kari

# 3. 🛡️ Hardened Systemd Units
echo -e "${GRAY}[2/5] Deploying hardened service units...${NC}"

# Brain (Go API) Service - Hardened to minimize API exploit surface
cat <<EOF > /etc/systemd/system/kari-api.service
[Unit]
Description=Kari Go API Orchestrator
After=postgresql.service kari-agent.service

[Service]
ExecStart=/opt/kari/bin/kari-api
Restart=always
User=kari-api
Group=kari-api
EnvironmentFile=/etc/kari/api.env

# 🛡️ Kari Security Sandbox
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
NoNewPrivileges=true
CapabilityBoundingSet=~CAP_SYS_ADMIN CAP_NET_ADMIN

[Install]
WantedBy=multi-user.target
EOF

# Muscle (Rust Agent) Service
cat <<EOF > /etc/systemd/system/kari-agent.service
[Unit]
Description=Kari Rust System Agent
After=network.target

[Service]
ExecStart=/opt/kari/bin/kari-agent
Restart=always
User=root
Group=root

# 🛡️ CAP_CHOWN and CAP_DAC_OVERRIDE are all the Muscle needs
# We restrict it from unnecessary kernel modules
ProtectKernelModules=true

[Install]
WantedBy=multi-user.target
EOF

# 4. 🛡️ Binary Integrity
# Ensure binaries are immutable by anyone except root
chown -R root:root /opt/kari/bin
chmod 755 /opt/kari/bin
chmod 700 /opt/kari/bin/kari-agent # Only root should execute the Muscle
chmod 755 /opt/kari/bin/kari-api   # Go Brain needs to be executable

# ... [Daemon Reload & Start] ...

echo -e "${TEAL}------------------------------------------------"
echo "✅ Kari Hardened Installation Complete!"
echo -e "------------------------------------------------${NC}"
