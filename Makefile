.PHONY: dev dev-backend dev-frontend infra migrate test test-integration clean lint lint-go lint-frontend

# Start all infrastructure
infra:
	docker compose up -d

# Stop all infrastructure
infra-down:
	docker compose down

# Run database migrations
migrate:
	@echo "Running migrations..."
	@for f in db/migrations/*.up.sql; do \
		echo "Applying $$f"; \
		docker exec -i ldapwarden-postgres psql -U ldapwarden -d ldapwarden < $$f; \
	done

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
	golangci-lint run ./...

# Lint frontend code
lint-frontend:
	cd web && pnpm lint

# Run tests
test:
	go test ./...

# Run integration tests against docker-compose services
test-integration:
	@docker compose up -d postgres redis openldap
	@echo "Waiting for postgres..."
	@until docker exec ldapwarden-postgres pg_isready -U ldapwarden > /dev/null 2>&1; do sleep 1; done
	@echo "Waiting for redis..."
	@until docker exec ldapwarden-redis redis-cli ping > /dev/null 2>&1; do sleep 1; done
	@echo "Waiting for OpenLDAP..."
	@ready=0; for i in $$(seq 1 60); do \
		if docker exec ldapwarden-openldap ldapsearch -x -D "cn=admin,dc=example,dc=org" -w admin -b "dc=example,dc=org" -s base -H ldap://localhost > /dev/null 2>&1; then \
			echo "OpenLDAP ready"; ready=1; break; \
		fi; \
		sleep 2; \
	done; \
	if [ $$ready -eq 0 ]; then echo "OpenLDAP did not become ready"; exit 1; fi
	LDAPWARDEN_TEST_INTEGRATION=1 go test -tags integration -count=1 -v ./...

# Clean build artifacts
clean:
	rm -rf web/dist
	docker compose down -v

# Show help
help:
	@echo "Available targets:"
	@echo "  infra          - Start PostgreSQL, Redis, OpenLDAP containers"
	@echo "  infra-down     - Stop all containers"
	@echo "  migrate        - Run database migrations"
	@echo "  dev-backend    - Run Go backend server"
	@echo "  dev-frontend   - Run React frontend dev server"
	@echo "  install-frontend - Install frontend dependencies"
	@echo "  build-frontend - Build frontend for production"
	@echo "  lint           - Run all linters"
	@echo "  lint-go        - Run golangci-lint"
	@echo "  lint-frontend  - Run ESLint on frontend"
	@echo "  test           - Run tests"
	@echo "  test-integration - Run integration tests against docker-compose services"
	@echo "  clean          - Clean build artifacts and volumes"
