.PHONY: dev dev-backend dev-frontend infra migrate test clean

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

# Run tests
test:
	go test ./...

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
	@echo "  test           - Run tests"
	@echo "  clean          - Clean build artifacts and volumes"
