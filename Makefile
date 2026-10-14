.PHONY: dev dev-backend dev-frontend infra test test-web test-e2e version-stamp-check test-integration clean lint lint-go lint-frontend

# Start all infrastructure
infra:
	docker compose up -d

# Stop all infrastructure
infra-down:
	docker compose down

# Run backend
dev-backend:
	cd cmd/server && go run .

# Run frontend
dev-frontend:
	cd web && pnpm dev

# Install frontend dependencies
install-frontend:
	cd web && pnpm install

# Build frontend
build-frontend:
	cd web && pnpm build

# Lint all code
lint: lint-go lint-frontend

# Lint Go code
lint-go:
	golangci-lint run --build-tags=integration ./...

# Lint frontend code
lint-frontend:
	cd web && pnpm lint

# Run tests
test:
	go test ./...

# Proves the -ldflags path the release build depends on: a typo in the symbol
# name fails silently, leaving every release reporting itself as "dev".
version-stamp-check:
	go test -count=1 -ldflags "-X github.com/ldapwarden/ldapwarden/internal/buildinfo.Version=stamp-check" \
		-run TestVersionVariableIsLinkable ./internal/buildinfo/

# Frontend unit tests (vitest), the counterpart of `test` for the web side
test-web:
	cd web && pnpm test

# Run integration tests against docker-compose services
# --wait replaces the hand-rolled polling loops: both services declare a
# healthcheck, and the directory's is the same ldapsearch this used to run.
test-integration:
	@docker compose up -d --wait postgres openldap
	LDAPWARDEN_TEST_INTEGRATION=1 go test -tags integration -race -count=1 -v ./...

# What each role sees, in a browser, against the real binary and build: the UI
# must agree with what the API lets that session do (web/e2e/rbac.spec.ts).
# Starts its own servers on 8810/8811 against the compose services; reads only.
test-e2e:
	@docker compose up -d --wait postgres openldap
	cd web && pnpm build
	go build -o tmp/ldapwarden-e2e ./cmd/server
	cd web && pnpm test:e2e

# Clean build artifacts
clean:
	rm -rf web/dist
	docker compose down -v

# Show help
help:
	@echo "Available targets:"
	@echo "  infra          - Start PostgreSQL and OpenLDAP containers"
	@echo "  infra-down     - Stop all containers"
	@echo "  dev-backend    - Run Go backend server"
	@echo "  dev-frontend   - Run React frontend dev server"
	@echo "  install-frontend - Install frontend dependencies"
	@echo "  build-frontend - Build frontend for production"
	@echo "  lint           - Run all linters"
	@echo "  lint-go        - Run golangci-lint"
	@echo "  lint-frontend  - Run ESLint on frontend"
	@echo "  test           - Run tests"
	@echo "  test-integration - Run integration tests against docker-compose services"
	@echo "  test-e2e       - Run the browser RBAC matrix against docker-compose services"
	@echo "  clean          - Clean build artifacts and volumes"
