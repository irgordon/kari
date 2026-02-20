#!/usr/bin/env bash
# Karı - Local Development Bootstrapper
# Usage: ./scripts/dev.sh

set -euo pipefail

# --- Color formatting ---
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${CYAN}======================================================${NC}"
echo -e "${CYAN}🚀 Starting Karı Local Development Environment...${NC}"
echo -e "${CYAN}======================================================${NC}"

# -----------------------------------------------------------------------------
# 1. Pre-Flight Dependency Checks
# -----------------------------------------------------------------------------
check_cmd() {
    if ! command -v "$1" &> /dev/null; then
        echo -e "${RED}❌ Missing dependency: $1 is required to run Karı locally.${NC}"
        exit 1
    fi
}

echo -e "${YELLOW}🔍 Checking prerequisites...${NC}"
check_cmd "docker"
check_cmd "go"
check_cmd "cargo"
check_cmd "npm"
echo -e "${GREEN}✅ All dependencies found.${NC}"

# -----------------------------------------------------------------------------
# 2. Local Environment Setup
# -----------------------------------------------------------------------------
# We create a local-dev directory to hold our fake Unix sockets and logs 
# so the Rust agent doesn't need root permissions to bind to /run/kari/
DEV_DIR=".local-dev"
mkdir -p "$DEV_DIR/tmp" "$DEV_DIR/logs"

# Ensure frontend dependencies are installed
if [ ! -d "frontend/node_modules" ]; then
    echo -e "${YELLOW}📦 Installing SvelteKit dependencies...${NC}"
    (cd frontend && npm install)
fi

# -----------------------------------------------------------------------------
# 3. Database Bootstrapping (Docker Compose)
# -----------------------------------------------------------------------------
echo -e "${YELLOW}🗄️ Starting PostgreSQL via Docker...${NC}"
# Assuming your docker-compose.yml has a service named 'postgres'
docker compose up -d

# Wait for Postgres to be ready
echo -e "${YELLOW}⏳ Waiting for database to accept connections...${NC}"
sleep 3 # Give docker a moment to map ports
until docker exec kari-postgres pg_isready -U kari_user &>/dev/null; do
    sleep 1
done
echo -e "${GREEN}✅ Database is ready.${NC}"

# -----------------------------------------------------------------------------
# 4. Process Management & Cleanup (The Trap)
# -----------------------------------------------------------------------------
# This array holds the Process IDs (PIDs) of our microservices
declare -a PIDS=()

cleanup() {
    echo -e "\n${RED}🛑 Shutting down Karı Development Environment...${NC}"
    for pid in "${PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid"
        fi
    done
    docker compose stop
    echo -e "${GREEN}✅ All processes terminated gracefully. Goodbye!${NC}"
    exit 0
}

# Catch Ctrl+C (SIGINT) and system termination (SIGTERM)
trap cleanup SIGINT SIGTERM

# -----------------------------------------------------------------------------
# 5. Launch the Monorepo Services
# -----------------------------------------------------------------------------
# We pass KARI_SOCKET_PATH as an environment variable override so Rust and Go
# use our local ./tmp folder instead of the root-locked /run/kari directory.
export KARI_SOCKET_PATH="$(pwd)/$DEV_DIR/tmp/agent.sock"

# A. Start the Rust Agent (The Muscle)
echo -e "${YELLOW}⚙️ Starting Rust Agent...${NC}"
(cd agent && cargo run > "../$DEV_DIR/logs/agent.log" 2>&1) &
PIDS+=($!)

# B. Start the Go API (The Brain)
echo -e "${YELLOW}🧠 Starting Go API...${NC}"
# Note: we pass a fake development JWT secret and DB URL via env vars
(cd api && \
 DATABASE_URL="postgres://kari_user:kari_pass@localhost:5432/kari_db?sslmode=disable" \
 JWT_SECRET="dev_secret_key_12345" \
 go run ./cmd/kari-api/main.go > "../$DEV_DIR/logs/api.log" 2>&1) &
PIDS+=($!)

# C. Start the SvelteKit Frontend (The UI)
echo -e "${YELLOW}🎨 Starting SvelteKit UI...${NC}"
(cd frontend && npm run dev > "../$DEV_DIR/logs/frontend.log" 2>&1) &
PIDS+=($!)

# -----------------------------------------------------------------------------
# 6. Stream Logs to the Developer
# -----------------------------------------------------------------------------
echo -e "${GREEN}======================================================${NC}"
echo -e "${GREEN}🎉 Karı is running locally!${NC}"
echo -e "   🌐 UI:       ${CYAN}http://localhost:5173${NC}"
echo -e "   🔌 API:      ${CYAN}http://localhost:8080${NC}"
echo -e "   🛡️ Socket:   ${CYAN}$KARI_SOCKET_PATH${NC}"
echo -e "${GREEN}======================================================${NC}"
echo -e "Streaming logs... (Press ${RED}Ctrl+C${NC} to stop all services)"
echo ""

# Use 'tail' to multiplex the logs from all three services into the current terminal
tail -f "$DEV_DIR/logs/agent.log" "$DEV_DIR/logs/api.log" "$DEV_DIR/logs/frontend.log"
