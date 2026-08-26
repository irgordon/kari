SHELL := /bin/bash
.DEFAULT_GOAL := help

include toolchain.env

export GOTOOLCHAIN := go$(GO_VERSION)

.PHONY: help verify verify-preflight verify-go verify-rust verify-frontend verify-proto \
	verify-database verify-topology verify-static verify-docs proto compose-up compose-down \
	topology-smoke migrate

help:
	@echo "Kari baseline commands"
	@echo "  make verify          Run the complete local/CI verification contract"
	@echo "  make proto           Regenerate committed protobuf artifacts"
	@echo "  make migrate         Apply the canonical database migration chain"
	@echo "  make compose-up      Start the supported container topology"
	@echo "  make compose-down    Stop the supported container topology"
	@echo "  make topology-smoke  Build, start, and health-check the topology"

verify:
	@./scripts/verify.sh

verify-preflight:
	@./scripts/verify-preflight.sh

verify-go:
	@./scripts/verify-go.sh

verify-rust:
	@./scripts/verify-rust.sh

verify-frontend:
	@./scripts/verify-frontend.sh

verify-proto:
	@./scripts/check-proto-drift.sh

verify-database:
	@./scripts/verify-database.sh

verify-topology:
	@./scripts/verify-topology.sh

verify-static:
	@./scripts/verify-static.sh

verify-docs:
	@./scripts/verify-docs.sh

proto:
	@./scripts/proto-gen.sh

migrate:
	@go run ./api/cmd/migrate

compose-up:
	@docker build --build-arg "GO_BUILDER_IMAGE=$(GO_BUILDER_IMAGE)" --tag kari-api:local --file api/Dockerfile .
	@./scripts/compose.sh build frontend
	@./scripts/compose.sh up --no-build --detach

compose-down:
	@./scripts/compose.sh down --remove-orphans

topology-smoke:
	@./scripts/topology-smoke.sh
