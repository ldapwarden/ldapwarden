# LDAP Warden

## Overview & Goals

We want to build a tool similar to "LDAP Account Manager" (https://www.ldap-account-manager.org/lamcms/ ). This tool's goal is to manage an LDAP Directory, especially an LDAP managing accounts (Linux, Windows, etc.).

### Goals
- Replace LAM with a modern, maintainable alternative
- Focus on OpenLDAP first, then expand backend support
- API-first design for automation

### Non-Goals (v1)
- Full Active Directory management (use RSAT)
- Email server configuration
- Acting as an LDAP proxy

## MVP Scope
1. Single OpenLDAP backend connection
2. User/Group CRUD with dynamic form generation
3. LDAP bind authentication
4. Basic RBAC (admin/read-only)
5. Audit logging
6. Docker Compose deployment
7. Netgroup and sudo rule management

## Architecture

### Data Flow
- PostgreSQL: user sessions, audit logs, scheduled task history, notification
  and lockout tracking
- No Redis: it once held sessions and nothing else, and was dropped. Rate
  limiting is in-process (httprate), so it is per instance.
- LDAP: Source of truth for directory objects (never duplicated in PG)

### Component Diagram
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   React UI  │────▶│   Go API    │────▶│  OpenLDAP   │
└─────────────┘     └──────┬──────┘     └─────────────┘
                          │
                          ▼
                    ┌──────────┐
                    │ Postgres │
                    └──────────┘

### Key Design Decisions
- **No LDAP data duplication**: All directory data stays in LDAP
- **Schema-driven UI**: Forms generated from LDAP schema, not hardcoded
- **Stateless API**: Horizontal scaling ready

### Backend Architecture

**Stack**
- Go 1.23+
- Chi router
- pgx for PostgreSQL, queries written by hand (no ORM, no generator)
- go-ldap for LDAP operations
- oapi-codegen for OpenAPI spec generation

**Project Structure**
```
cmd/
  server/           # Entry point
internal/
  api/              # HTTP handlers
  ldap/             # LDAP client wrapper
  auth/             # Authentication logic
  rbac/             # Permission checks
  schema/           # LDAP schema parsing
  migrations/       # SQL migrations (embedded in the binary)
web/                # React app
```

### Frontend Architecture

**Stack**
- React 19 with TypeScript
- TanStack Router (file-based routing)
- TanStack Query (server state management)
- Tailwind CSS 4 + shadcn/ui components
- Zod for runtime validation

**Key Patterns**
- Forms hold their own state and adopt server data through `useSyncedForm`
  (`web/src/lib/form-sync.ts`): it re-reads a refetched entry while the form is
  untouched and leaves edits in progress alone. Deliberately not a form
  library — the fields mirror LDAP attributes one for one, so there is no
  schema to generate a form from, and the interesting problem is reconciling
  with a directory that other tools also write to.
- List views page, sort and filter server-side; the browser renders one page of
  a narrow projection. See the UI conventions in `CLAUDE.md`.
- Mutations invalidate the affected queries on success rather than updating the
  cache optimistically. A directory write can be refused by ppolicy or by an
  ACL, and showing a change that the server then rejects is worse than a
  moment's wait.
- Zod validates every API response at the boundary.

**Not built yet**
- Code-splitting by route: the bundle is a single chunk of roughly 840 kB.
- i18n: the interface is English-only and pulls in no i18n library.

### Testing Strategy

**Backend**
- Unit tests for business logic (schema parsing, RBAC)
- Integration tests against OpenLDAP container

**Frontend**
- Vitest for unit tests — set up and run in CI (`make test-web`). Covers the pure
  logic in `src/lib`, which is where the rules a component must not get wrong are
  deliberately kept; components themselves are not covered yet, though jsdom is
  installed so they can be
- Playwright for E2E (critical flows: login, user CRUD) — not set up
- Storybook for component documentation — not set up

**CI Pipeline**
- Lint + type-check on every PR
- Tests run against ephemeral OpenLDAP + Postgres
- Docker image build verification


## Data Model

### Core Tables
- `ldap_connections` - Backend server configs (URL, bind DN, TLS settings)
- `users` - Local app users (for non-LDAP auth scenarios, API keys)
- `sessions` - Active sessions (the whole session as JSON, keyed by token hash)
- `roles` / `permissions` - RBAC definitions
- `audit_logs` - All operations with actor, action, target, timestamp
- `schema_cache` - Parsed LDAP schema (JSON), refreshed on demand
- `templates` - Attribute templates for object creation

### Key Constraints
- No directory data stored here
- Audit logs are append-only (no UPDATE/DELETE)
- Foreign keys to ldap_connections for multi-backend support


## Schema Management
- Auto-detect LDAP schema on connection
- Cache the schema in process
- Map objectClasses to UI form templates
- Support for:
  - inetOrgPerson, posixAccount, posixGroup (core)
  - sambaSamAccount, shadowAccount (extended)
  - Custom objectClasses via admin UI
- Attribute metadata: required, multi-value, syntax validation


## Authentication & Security

**MVP**
- LDAP bind authentication
- Session management in PostgreSQL
- Basic RBAC (admin / read-only / custom roles)
- Audit logging for all operations
- TLS 1.2+ for LDAP connections

**Post-MVP**
- SSO integration (SAML2, OIDC)
- Kerberos authentication
- Optional 2FA (TOTP) for admin accounts
- Certificate management UI


## Application Security
- CSRF protection on all state-changing endpoints
- Rate limiting on authentication endpoints
- Content Security Policy headers
- Secrets management (env vars, Vault integration)
- Input sanitization to prevent LDAP injection


## Directory Object Management

**MVP**
- CRUD operations for users, groups, and custom object classes
- Schema autodetection and dynamic form generation
- Attribute templates for consistent object creation

**Post-MVP**
- Bulk operations (import/export, batch edits, mass deletion)
- Support for multiple LDAP backends (389ds, Active Directory, FreeIPA)
- Hosts and roles management


## Netgroups & Sudo

**MVP**
- CRUD for `nisNetgroup` entries, with nesting (`memberNisNetgroup`) resolved on
  demand rather than stored
- Sudo rules granting to a netgroup through the `+name` prefix, on either side
- CSV import for netgroups
- A user's netgroup memberships, direct and inherited, shown on their page

**Post-MVP**
- A host inventory, so a triple's host slot can be picked rather than typed
- Renaming a netgroup, which means migrating every `+name` and
  `memberNisNetgroup` reference by hand, as `RenameUser` already does for uids

**Design constraints.** These are properties of the NIS schema and of how sudo
consumes it, not choices this project made. They are listed here because each one
is a way for the app to be confidently wrong, and the implementation is shaped
around saying so instead:

- An empty slot in a `(host,user,domain)` triple means *any*, never *none*; `-`
  is the NIS marker for "matches nothing". Reading one as the other reports the
  exact opposite of a grant.
- `nisNetgroupTriple` declares a syntax and no matching rules, so the directory
  cannot evaluate a filter over it — and OpenLDAP does not refuse such a filter,
  it answers `Success` with zero entries. Only a presence test works server-side;
  everything else is filtered in Go over parsed triples.
- A netgroup is referenced by string from two places that do not track it,
  `sudoUser`/`sudoHost` and `memberNisNetgroup`. Deleting or renaming one leaves
  references matching nobody, and nothing in the directory reports it.
- Which side a `+name` is used from decides which half of the netgroup applies.
  A netgroup of users named in `sudoHost` matches nothing, silently.
- Sudo resolves netgroups on the client — NSS, `netgroup_base`, or SSSD — none of
  which is visible from the directory. Writing a grant is therefore necessary and
  not sufficient, and the app states that rather than implying the job is done.

The full rules live in `CLAUDE.md` under Directory Semantics, and the endpoints
in [API.md](API.md).


## Password & Account Policies

- Password reset flows with admin approval or self-service
- Password strength validation and policy enforcement
- Account lock/unlock, expiration, and lifecycle management
- Integration with Kerberos or external password vaults

_Note: Password expiration checks are handled via Workflow & Automation scheduled tasks._


## Workflow & Automation

**MVP**
- Bulk import from CSV/LDIF
- Bulk export to CSV/LDIF/JSON

**Post-MVP**
- Configurable provisioning workflows (onboarding/offboarding templates)
- Scheduled tasks via cron-like syntax:
  - Password expiration warnings
  - Inactive account detection
  - Schema refresh
- Webhook triggers on object changes


## API & Integration

**MVP**
- REST API (OpenAPI 3.1 spec) as primary interface
- Webhooks for object change events

**Post-MVP**
- GraphQL endpoint for complex queries
- SCIM 2.0 support for identity provisioning
- Integration templates for ticketing systems (Jira, GLPI, ServiceNow)


## User Interface & UX

- Clean, responsive web UI built with React 19
- Dark/light & system theme with CSS variables and accessibility compliance (WCAG 2.1 AA)
- Customizable dashboards and quick‑action panels
- Inline validation and contextual help for LDAP attributes
- Multi-language support  


## Error Handling
- Structured error responses with LDAP error code mapping
- Client-side validation mirrors server-side rules
- Graceful degradation when LDAP is unreachable
- Circuit breaker pattern for LDAP connections


## Monitoring & Reporting

- Real-time status of LDAP servers
- Reports on inactive accounts, expiring passwords, group membership, etc.
- Export to CSV/JSON/PDF


## Deployment & Ops

- Docker/Kubernetes-ready deployment
- Configuration-as-code: YAML for human-readable configuration
- Backup/restore utilities
- High-availability support

### Development Setup
```bash
# Prerequisites: Docker, Go 1.23+, Node 22+, pnpm

# Start infrastructure
docker compose up -d postgres openldap

# Run backend
cd cmd/server && go run .

# Run frontend
cd web && pnpm dev
```

**Dev OpenLDAP** comes pre-seeded with:
- Base DN: `dc=example,dc=org`
- Admin: `cn=admin,dc=example,dc=org` / `admin`
- Sample users and groups


## Configuration

**Environment Variables**
- `DATABASE_URL` - PostgreSQL connection string
- `LDAP_URL` - Default LDAP server (can be overridden in UI)
- `SESSION_SECRET` - Secret for session signing
- `LOG_LEVEL` - debug/info/warn/error

The full set, with defaults, is in `.env.example` and in the Configuration
tables of the README; the list above is only the connection essentials.

**Config File** (`config.yaml`)
- Feature flags
- Default RBAC roles
- UI customization (logo, colors)
- Scheduled task definitions


## Future Modules

- Mailbox provisioning (IMAP/SMTP systems)
- Alias and routing management
- Disk quota management (Linux, Pykota, etc.)
- DHCP, DNS, Radius, Samba, or other service-specific modules


## License

LDAP Warden is released under the Apache 2.0 license.

