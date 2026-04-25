# ==============================================================================
# Karı Orchestration Engine - Master Control
# 🛡️ SLA: Single-command lifecycle with mandatory security audits
# ==============================================================================

.PHONY: help gen-secrets audit build build-prod up down restart clean logs proto proto-check verify reproducible dev

# Default target: Shows available commands
help:
	@echo "🛡️  Karı Orchestration Commands"
	@echo "Usage: make [target]"
	@echo ""
	@echo "High-Level Targets:"
	@echo "  deploy          - 🚀 Full Lifecycle: Generate secrets -> Audit -> Build -> Up"
	@echo "  deploy-prod     - 🚀 Production Build: Secrets -> Audit -> Distroless -> Up"
	@echo ""
	@echo "Individual Targets:"
	@echo "  gen-secrets     - 🔐 Generates .env with high-entropy keys"
	@echo "  audit           - 🔍 Validates .env against security_strict.json"
	@echo "  build           - 📦 Build all Docker containers (dev)"
	@echo "  build-prod      - 📦 Build production containers (distroless + stripped)"
	@echo "  up              - ⬆️  Start the stack"
	@echo "  down            - ⬇️  Stop and remove containers"
	@echo "  clean           - 🧹 Hard reset: Remove volumes and .env"
	@echo "  proto           - 🔄 Regenerate gRPC protobuf stubs"
	@echo "  proto-check     - 🔍 Fail if protobuf outputs are stale"
	@echo "  verify          - ✅ Run unified Go/Rust/Frontend validation"
	@echo "  reproducible    - ♻️  Check generated/repo state determinism"
	@echo "  dev             - 🧭 One-command local deterministic workflow"

# 🚀 The Master Lifecycle (Development)
deploy: gen-secrets audit build up

# 🚀 The Production Lifecycle (Distroless + Hardened)
deploy-prod: gen-secrets audit build-prod up

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

# 📦 Step 3: Docker Lifecycle (Development)
build:
	@echo "📦 Building Docker images (dev)..."
	@docker-compose build

# 📦 Step 3b: Docker Lifecycle (Production — Distroless + Stripped)
# 🛡️ Zero-Trust: Uses Dockerfile.prod for the Brain with:
#   - gcr.io/distroless/static-debian12 (no shell, no package manager)
#   - UID 1001 (matches PeerCred validation)
#   - CGO_ENABLED=0 + -ldflags="-s -w" (fully static, stripped)
build-prod:
	@echo "📦 Building PRODUCTION Docker images..."
	@docker-compose -f docker-compose.yml -f docker-compose.prod.yml build

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


# 🔄 Proto Regeneration
proto:
	@echo "🔄 Regenerating protobuf stubs..."
	@chmod +x scripts/proto-gen.sh && ./scripts/proto-gen.sh
	@echo "✅ Proto stubs regenerated."

# 🛡️ Proto Drift Guard (CI check)
proto-check:
	@echo "🔍 Validating protobuf stubs are up to date..."
	@chmod +x scripts/check-proto-drift.sh && ./scripts/check-proto-drift.sh

# ✅ Unified Local/CI Validation
verify:
	@echo "🧪 Running unified verification pipeline..."
	@go test ./...
	@cd agent && cargo fmt -- --check
	@cd agent && cargo clippy --all-targets --all-features
	@cd agent && cargo test
	@npm --prefix frontend ci
	@npm --prefix frontend run check
	@npm --prefix frontend run test -- --run
	@echo "✅ verify completed"


# ♻️ Reproducibility Guard
reproducible:
	@echo "♻️ Validating deterministic repository state..."
	@git diff --exit-code
	@echo "✅ Reproducibility check passed (no uncommitted drift)."

# 🧭 One-command developer workflow
dev: gen-secrets
	@echo "🧭 Running one-command developer workflow..."
	@if command -v protoc >/dev/null 2>&1 && command -v protoc-gen-go >/dev/null 2>&1 && command -v protoc-gen-go-grpc >/dev/null 2>&1; then \
		echo "🔄 Protobuf toolchain detected; running proto + drift checks..."; \
		$(MAKE) proto; \
		$(MAKE) proto-check; \
	else \
		echo "ℹ️  Protobuf toolchain not fully installed; skipping proto steps (safe unless editing .proto)."; \
	fi
	@$(MAKE) verify
	@$(MAKE) reproducible
	@echo "✅ dev workflow complete."
