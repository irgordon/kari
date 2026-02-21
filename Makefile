# ==============================================================================
# Karı Orchestration Engine - Master Control
# 🛡️ SLA: Single-command lifecycle with mandatory security audits
# ==============================================================================

.PHONY: help gen-secrets audit build up down restart clean logs

# Default target: Shows available commands
help:
	@echo "🛡️  Karı Orchestration Commands"
	@echo "Usage: make [target]"
	@echo ""
	@echo "High-Level Targets:"
	@echo "  deploy          - 🚀 Full Lifecycle: Generate secrets -> Audit -> Build -> Up"
	@echo ""
	@echo "Individual Targets:"
	@echo "  gen-secrets     - 🔐 Generates .env with high-entropy keys"
	@echo "  audit           - 🔍 Validates .env against security_strict.json"
	@echo "  build           - 📦 Build all Docker containers"
	@echo "  up              - ⬆️  Start the stack"
	@echo "  down            - ⬇️  Stop and remove containers"
	@echo "  clean           - 🧹 Hard reset: Remove volumes and .env"

# 🚀 The Master Lifecycle: This is the one-command deployment
deploy: gen-secrets audit build up

# 🔐 Step 1: Generate Secrets
gen-secrets:
	@if [ ! -f .env ]; then \
		echo "🔐 .env missing. Running secure generator..."; \
		chmod +x scripts/gen-secrets.sh && ./scripts/gen-secrets.sh; \
	else \
		echo "✅ .env already exists. Skipping generation."; \
	fi

# 🔍 Step 2: Security Posture Audit
audit:
	@echo "🔍 Running Security Posture Audit..."
	@go run api/cmd/audit/check-posture.go

# 📦 Step 3: Docker Lifecycle
build:
	@echo "📦 Building Docker images..."
	@docker-compose build

up:
	@echo "⬆️  Starting Karı Engine..."
	@docker-compose up -d
	@echo "✅ Stack is live. UI: http://localhost:5173 | API: http://localhost:8080"

down:
	@echo "⬇️  Stopping Karı Engine..."
	@docker-compose down

restart: down up

# 🧹 Maintenance
clean:
	@echo "⚠️  DANGER: Removing all volumes and secrets..."
	@docker-compose down -v
	@rm -f .env
	@echo "🧹 Clean complete."

logs:
	@docker-compose logs -f
