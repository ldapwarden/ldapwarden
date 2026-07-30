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
ldap/                # OpenLDAP 2.6 image for the bundled stack: schema,
                     # ppolicy overlay, seed data
```

## Code Conventions
- Go: Follow standard project layout, use sqlc for queries
- TypeScript: Strict mode, Zod for all API responses
- Commits: Conventional commits (feat:, fix:, docs:)
- PRs: Must pass lint + tests

## API Endpoints
See [docs/API.md](docs/API.md) for the full reference, including the query
parameters the list endpoints accept.
- `POST /api/auth/login` - LDAP bind authentication
- `POST /api/auth/logout` - Session invalidation
- `GET /api/auth/me` - Current user info
- `GET /api/overview` - Dashboard aggregates (counts, expiring accounts)
- `GET/POST/PUT/DELETE /api/users` - User CRUD; list takes `search`, `status`, `sort`, `order`, `limit`, `offset`
- `GET /api/users/:dn/photo` - User photo as `image/jpeg`
- `GET/POST/PUT/DELETE /api/groups` - Group CRUD; list takes `search`, `sort`, `order`, `limit`, `offset`
- `GET /api/groups/:dn/members` - Membership resolved to names
- `POST/DELETE /api/groups/:dn/members` - Group membership
- `GET /api/schema` - LDAP schema
- `GET /api/audit-logs` - Audit logs (admin only), filterable by actor / resource / action / date range
- `GET /api/audit-logs/export` - Audit logs as CSV, same filters

## Test Accounts (LDAP)
- admin / admin123 (system admin)
- jdoe / password123 (developer)
- viewer / viewer123 (read-only)

## Current Status
MVP implemented with:
- Dashboard, User/Group CRUD with form-based editing
- LDAP bind authentication
- Redis session management
- Basic RBAC (admin/readonly roles)
- Audit logging with per-attribute diffs and CSV export
- Docker Compose deployment

## Directory Semantics
- **Locking** is `pwdAccountLockedTime` = `000001010000Z` (OpenLDAP's
  permanent-lock value). Never write a timestamp there: ppolicy lifts a
  timestamped lock once `pwdLockoutDuration` elapses.
- **Account expiry** is `pwdEndTime`, and nothing else. It needs OpenLDAP >= 2.5;
  `Client.Capabilities().ScheduledExpiry` probes for it and the UI hides the
  controls when absent.
- **`shadowExpire` is not an account expiry** for this app: slapd ignores it (only
  PAM/SSSD read it). It stays editable as a raw attribute and must not drive
  status, the dashboard or notifications.
- ppolicy enforces none of the above unless a policy applies to the entry
  (`olcPPolicyDefault` or `pwdPolicySubentry`).
- The bundled stack is OpenLDAP 2.6 built from `ldap/`, with the overlay and
  `olcPPolicyDefault` wired up — so integration tests exercise the real
  enforcement path, not the 422 fallback. From 2.5 the pwdPolicy schema lives
  inside the overlay module; never load a separate `ppolicy.ldif`.

## UI Conventions
- List views page, sort and filter **server-side**; the browser renders one page
  of a narrow projection. Never add a column that requires shipping a whole
  collection to the client.
- **Pickers search server-side too**, via `lib/use-picker-search.ts`: an
  unbounded list request silently stops at `LDAP_SEARCH_SIZE_LIMIT`, so a
  client-side filter over "everything" is really a filter over the first 500
  entries. Where a dropdown genuinely cannot search (password policies, the
  primary group), pass an explicit `limit` and say what is not listed.
- View state (search, sort, page, size, active tab, audit filters) lives in the
  URL via `validateSearch`, so every view is a shareable link. Helpers:
  `lib/list-search.ts`, `lib/use-url-search-input.ts`.
- Mutation outcomes are reported by a **toast**. Inline `Callout`s are for
  page-level load failures, field validation and persistent notices — not for
  confirming a save.
- Colours come from theme tokens (`text-success`, `bg-warning/10`, the `.tone`
  class for categorical tints). Never use raw palette utilities such as
  `bg-green-100`: they only work in light mode.
- Loading states use the skeletons in `components/ui/skeleton.tsx`, rendered
  inside the real table so nothing shifts when data arrives.
