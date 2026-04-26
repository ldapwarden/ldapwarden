# LDAP Warden

## Project Spec
See [docs/SPEC.md](docs/SPEC.md) for full architecture and requirements.

## Quick Start

```bash
# 1. Start infrastructure (PostgreSQL, Redis, OpenLDAP)
docker compose up -d

# 2. Install frontend dependencies
cd web && pnpm install && cd ..

# 3. Start backend (in one terminal) — applies DB migrations on startup
cd cmd/server && go run .

# 4. Start frontend (in another terminal)
cd web && pnpm dev
```

Access the app at http://localhost:5173

**Demo credentials:** admin / admin123

## Development Commands
```bash
docker compose up -d          # Start infrastructure
docker compose down           # Stop infrastructure
cd cmd/server && go run .     # Backend (port 8000) — applies migrations on startup
cd web && pnpm dev            # Frontend (port 5173)
make test-integration         # Run integration tests against compose services
```

## Project Structure
```
cmd/server/           # Go backend entry point
internal/
  api/               # HTTP handlers & routes
  ldap/              # LDAP client wrapper
  auth/              # Authentication & sessions
  rbac/              # Permission checks
  audit/             # Audit logging
  config/            # Configuration
db/
  migrations/        # SQL migrations
  queries/           # sqlc queries
web/                 # React frontend
  src/routes/        # TanStack Router pages
  src/components/    # UI components
  src/lib/           # API client, auth, utils
ldap/                # OpenLDAP seed data
```

## Code Conventions
- Go: Follow standard project layout, use sqlc for queries
- TypeScript: Strict mode, Zod for all API responses
- Commits: Conventional commits (feat:, fix:, docs:)
- PRs: Must pass lint + tests

## API Endpoints
- `POST /api/auth/login` - LDAP bind authentication
- `POST /api/auth/logout` - Session invalidation
- `GET /api/auth/me` - Current user info
- `GET/POST/PUT/DELETE /api/users` - User CRUD
- `GET/POST/PUT/DELETE /api/groups` - Group CRUD
- `POST/DELETE /api/groups/:dn/members` - Group membership
- `GET /api/schema` - LDAP schema
- `GET /api/audit-logs` - Audit logs (admin only)

## Test Accounts (LDAP)
- admin / admin123 (system admin)
- jdoe / password123 (developer)
- viewer / viewer123 (read-only)

## Current Status
MVP implemented with:
- User/Group CRUD with form-based editing
- LDAP bind authentication
- Redis session management
- Basic RBAC (admin/readonly roles)
- Audit logging
- Docker Compose deployment
