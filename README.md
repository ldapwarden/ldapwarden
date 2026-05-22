<p align="center">
  <img src="docs/assets/ldapwarden-logo.svg" alt="LDAP Warden Logo" width="200" height="200">
</p>

<h1 align="center">LDAP Warden</h1>

<p align="center">
  <strong>A modern, web-based LDAP directory management tool</strong>
</p>

<p align="center">
  <a href="#features">Features</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#configuration">Configuration</a> •
  <a href="docs/API.md">API</a> •
  <a href="#development">Development</a> •
  <a href="#license">License</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react" alt="React Version">
  <img src="https://img.shields.io/badge/License-MIT-green?style=flat-square" alt="License">
</p>

---

LDAP Warden is a lightweight, self-hosted web application for managing OpenLDAP directories. It provides an intuitive interface for user and group management, sudo role configuration, and audit logging — without the complexity of traditional LDAP administration tools.

## Screenshots

<p align="center">
  <img src="docs/screenshots/02-users-list.png" alt="LDAP Warden — Users list" width="900">
</p>

<details>
<summary><strong>Login</strong></summary>

![Login](docs/screenshots/01-login.png)
</details>

<details>
<summary><strong>User Details</strong></summary>

![User Details](docs/screenshots/03-user-details.png)
</details>

<details>
<summary><strong>Groups List</strong></summary>

![Groups List](docs/screenshots/04-groups-list.png)
</details>

<details>
<summary><strong>Group Details</strong></summary>

![Group Details](docs/screenshots/05-group-details.png)
</details>

<details>
<summary><strong>Sudo Roles</strong></summary>

![Sudo Roles](docs/screenshots/06-sudo-roles.png)
</details>

<details>
<summary><strong>Audit Logs</strong></summary>

![Audit Logs](docs/screenshots/07-audit-logs.png)
</details>

<details>
<summary><strong>Administration</strong></summary>

![Administration](docs/screenshots/08-admin.png)
</details>

## Features

- **User Management** — Create, edit, and delete LDAP users with support for `inetOrgPerson`, `posixAccount`, and `shadowAccount` object classes
- **Group Management** — Manage POSIX groups and group memberships with an intuitive UI
- **Sudo Roles** — Full sudoers LDAP schema support with user and group-based sudo privilege management
- **Password Policies** — View and manage password policies (ppolicy overlay), assign policies to users
- **SSH Key Management** — Store and manage SSH public keys in LDAP (`ldapPublicKey` object class)
- **Samba Integration** — Optional support for `sambaSamAccount` and `sambaGroupMapping` object classes
- **LDAP Authentication** — Authenticate users directly against your LDAP directory; passwords are written via the RFC 3062 `PasswordModify` extended operation so the directory's hashing pipeline always runs
- **Role-Based Access Control** — Admin and read-only roles based on LDAP group membership, with active sessions invalidated immediately on user/admin-group mutation
- **Audit Logging** — Track every change in PostgreSQL with actor, IP and User-Agent, plus opt-in per-change email notifications
- **Email Notifications** — Per-change audit emails, password-reset flow, and proactive account/password expiration reminders to admins and users
- **Security-First Defaults** — Per-IP rate limiting on auth endpoints, baseline CSP / X-Frame-Options / Referrer-Policy headers, `X-Forwarded-For` allowlist by trusted-proxy CIDR, and fail-fast on default secrets
- **Modular UI** — Enable/disable features based on your LDAP schema (sudo, policies, Samba)
- **Photo Support** — Upload and display user photos (JPEG)
- **Modern UI** — Clean, responsive interface built with React and Tailwind CSS

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Git

### One-Command Setup

```bash
# Clone the repository
git clone https://github.com/yourusername/ldapwarden.git
cd ldapwarden

# Start all services (PostgreSQL, Redis, OpenLDAP, and LDAP Warden)
docker compose up -d

# Access the application
open http://localhost:8000
```

**Default credentials:** `admin` / `admin123`

> **Production note.** The bundled stack ships with `LDAPWARDEN_DEV_MODE=1`,
> which tolerates the well-known `SESSION_SECRET` and `LDAP_BIND_PASS=admin`.
> For a real deployment, drop that variable and provide fresh values — see
> the [Production-secrets validation](#production-secrets-validation) section
> below.

### What's Included

The Docker Compose setup includes:
- **LDAP Warden** — The main application (port 8000)
- **OpenLDAP** — Pre-configured LDAP server with sample data (port 389)
- **PostgreSQL** — Database for sessions and audit logs (port 5432)
- **Redis** — Session store (port 6379)
- **phpLDAPadmin** — Optional LDAP browser for debugging (port 8080, not started by default). Bring it up with `docker compose --profile debug up -d` or `docker compose up -d phpldapadmin`.

## Configuration

LDAP Warden is configured via environment variables:

### Server

| Variable | Description | Default |
|----------|-------------|---------|
| `SERVER_HOST` | Bind address | `0.0.0.0` |
| `SERVER_PORT` | HTTP port | `8000` |

### Database

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://ldapwarden:ldapwarden@localhost:5432/ldapwarden?sslmode=disable` |
| `REDIS_URL` | Redis connection string | `redis://localhost:6379` |

### LDAP

| Variable | Description | Default |
|----------|-------------|---------|
| `LDAP_URL` | LDAP server URL | `ldap://localhost:389` |
| `LDAP_BASE_DN` | Base Distinguished Name | `dc=example,dc=org` |
| `LDAP_BIND_DN` | Bind DN for admin operations | `cn=admin,dc=example,dc=org` |
| `LDAP_BIND_PASS` | Bind password | `admin` |
| `LDAP_USER_OU` | Organizational Unit for users | `ou=People` |
| `LDAP_GROUP_OU` | Organizational Unit for groups | `ou=Groups` |
| `LDAP_SUDOERS_OU` | Organizational Unit for sudo roles | `ou=sudoers` |
| `LDAP_MIN_UID` | Minimum UID for new users | `1000` |
| `LDAP_MIN_GID` | Minimum GID for new groups | `1000` |
| `LDAP_TLS_MODE` | TLS mode: `none`, `ssl`, or `starttls` | `none` |
| `LDAP_TLS_SKIP_VERIFY` | Skip TLS certificate verification (for self-signed certs) | `false` |

### Session & Security

| Variable | Description | Default |
|----------|-------------|---------|
| `SESSION_SECRET` | Secret key for session encryption | (change in production!) |
| `SESSION_TTL` | Session time-to-live | `24h` |

### Application

| Variable | Description | Default |
|----------|-------------|---------|
| `LDAPWARDEN_ADMIN_GROUP` | LDAP group that grants admin privileges | `admins` |
| `LDAPWARDEN_ORGANIZATION` | Organization name (for emails) | `Example Organization` |
| `LDAPWARDEN_PUBLIC_URL` | Public URL (for password reset links) | `http://localhost:8000` |
| `LDAPWARDEN_MODULES` | Comma-separated list of enabled modules (tabs) | `users,groups,sudo,policies` |
| `LDAPWARDEN_USERS_OBJECTS` | LDAP objectClasses for users | `inetOrgPerson,posixAccount,ldapPublicKey,shadowAccount` |
| `LDAPWARDEN_GROUPS_OBJECTS` | LDAP objectClasses for groups | `posixGroup` |
| `LDAPWARDEN_AUDIT_NOTIFY_EMAILS` | Comma-separated recipients that receive an email for every UI modification (audit/traceability). Empty disables. | (empty) |
| `LDAPWARDEN_TRUSTED_PROXIES` | Comma-separated CIDR list of reverse proxies allowed to set `X-Forwarded-For` / `X-Real-IP`. Empty disables forwarded-header support. | (empty) |
| `LDAPWARDEN_CORS_ORIGINS` | Comma-separated list of origins allowed by the CORS middleware. `*` is refused (the API serves credentialed requests). | `http://localhost:5173,http://localhost:3000` |
| `LDAPWARDEN_DEV_MODE` | Bypass production-secrets validation (also tolerates plain `http://` `PublicURL`). Only for the bundled compose stack and local tests. | `false` |

#### Module Configuration

LDAP Warden allows you to customize which features are visible based on your LDAP setup:

**`LDAPWARDEN_MODULES`** controls which high-level navigation tabs are shown:
- `users` — User management
- `groups` — Group management
- `sudo` — Sudo roles management (requires sudoers schema)
- `policies` — Password policies (requires ppolicy overlay)

**`LDAPWARDEN_USERS_OBJECTS`** controls which user attribute tabs appear:
- `inetOrgPerson` — Identity tab (name, email, phone, etc.)
- `posixAccount` — POSIX tab (UID, GID, shell, home directory)
- `ldapPublicKey` — SSH Keys tab
- `sambaSamAccount` — Samba tab (Windows/Samba integration)

**`LDAPWARDEN_GROUPS_OBJECTS`** controls which group attribute tabs appear:
- `posixGroup` — Standard POSIX group attributes
- `sambaGroupMapping` — Samba tab (Windows/Samba integration)

#### Audit notifications

**`LDAPWARDEN_AUDIT_NOTIFY_EMAILS`** is a comma-separated list of email
addresses that will receive a notification for every modification performed
through the UI (user/group/sudo-role/password-policy create/update/delete,
group membership changes, lock/unlock, etc.). The mail mirrors the entry
written to the audit log, including actor, action, resource and details.

- Login/logout, schema refresh and scheduler-driven notifications are
  excluded — only direct UI modifications are forwarded.
- Requires a working SMTP configuration (see the Mail section below).
- Set the variable to an empty value (or leave it unset) to disable the
  feature entirely.

Example: `LDAPWARDEN_AUDIT_NOTIFY_EMAILS=secops@acme.com,it-audit@acme.com`

#### Trusted proxies

**`LDAPWARDEN_TRUSTED_PROXIES`** is a comma-separated list of CIDRs that
identifies reverse proxies allowed to override the connection-level peer
address via `X-Forwarded-For`, `X-Real-IP` or `True-Client-IP`. Any header
coming from a peer outside the list is ignored — the connection-level IP
is recorded instead.

- **Default is empty (most secure)**: forwarded headers are never
  honoured. Recommended when LDAP Warden is exposed directly.
- **Behind a single reverse proxy** (nginx, Traefik, Caddy, AWS ALB,
  etc.): set this to the proxy's network. Without it, audit logs and the
  password-reset notification email will record the proxy's IP, not the
  client's.
- The server **fails to start** if a CIDR is malformed.

Example: `LDAPWARDEN_TRUSTED_PROXIES=10.0.0.0/8,127.0.0.1/32`

#### Production-secrets validation

LDAP Warden refuses to start with the in-repo default values for
`SESSION_SECRET` and `LDAP_BIND_PASS`. Specifically:

- `SESSION_SECRET` must be set, must be at least 32 bytes, and must not
  equal the documented default (`change-me-in-production-32bytes!`).
- `LDAP_BIND_PASS` must not equal the documented default (`admin`).

All checks are skipped when **`LDAPWARDEN_DEV_MODE=1`**. The bundled
`docker-compose.yaml` sets this on the `ldapwarden` service so the
local stack keeps working with its well-known credentials. **Do not
set `LDAPWARDEN_DEV_MODE=1` in a real deployment.**

Errors are aggregated so a misconfigured deployment surfaces every
issue in a single startup failure.

### Mail

| Variable | Description | Default |
|----------|-------------|---------|
| `MAIL_HOST` | SMTP server hostname | `localhost` |
| `MAIL_PORT` | SMTP server port | `1025` |
| `MAIL_USER` | SMTP username | (empty) |
| `MAIL_PASSWORD` | SMTP password | (empty) |
| `MAIL_FROM` | Sender email address | `noreply@example.org` |
| `MAIL_SSL` | SSL mode: `none`, `starttls`, or `ssl` | `none` |

### Scheduled Tasks

| Variable | Description | Default |
|----------|-------------|---------|
| `LDAPWARDEN_SCHEDULED_TASKS_USERS_EXPIRATION` | Cron schedule for account expiration checks | `42 3 * * *` |
| `LDAPWARDEN_SCHEDULED_TASKS_PASSWORDS_EXPIRATION` | Cron schedule for password expiration checks | `42 3 * * *` |

Set a variable to an empty string to disable that task.

#### How Background Tasks Work

LDAP Warden uses a cron-based scheduler (via [robfig/cron](https://github.com/robfig/cron)) to run periodic background tasks. Two tasks are available:

**Account Expiration (`users_expiration`)**
- Checks all users for upcoming account expiration (via `shadowExpire` or `pwdAccountLockedTime`)
- Sends email notifications to **administrators** (members of `LDAPWARDEN_ADMIN_GROUP`)
- Useful for reminding admins to extend or disable accounts before they lock out

**Password Expiration (`passwords_expiration`)**
- Checks all users for upcoming password expiration (via `shadowMax` or ppolicy `pwdMaxAge`)
- Sends email notifications directly to **each user** (requires `mail` attribute)
- Helps users proactively change their passwords before expiration

Both tasks send notifications at these intervals before expiration:
- 3 weeks before
- 1 week before
- 1 day before
- On expiration

Notifications are tracked in PostgreSQL to prevent duplicate emails for the same interval.

Tasks can also be triggered manually via the Administration UI or API (`POST /api/scheduler/:task/trigger`).

## Development

### Prerequisites

- Go 1.23+
- Node.js 20+
- pnpm
- Docker (for infrastructure) or local PostgreSQL, Redis, and OpenLDAP

### Setup with Docker (Recommended)

```bash
# Start infrastructure (PostgreSQL, Redis, OpenLDAP)
docker compose up -d postgres redis openldap

# Start the backend (terminal 1) — applies migrations on startup
cd cmd/server && go run .

# Start the frontend (terminal 2)
cd web && pnpm install && pnpm dev
```

Access the development server at http://localhost:5173

### Setup without Docker

If you prefer to run services locally without Docker:

1. **PostgreSQL** — Install and create a database:
   ```bash
   createdb ldapwarden
   ```

2. **Redis** — Install and start Redis:
   ```bash
   # macOS
   brew install redis && brew services start redis

   # Linux
   sudo apt install redis-server && sudo systemctl start redis
   ```

3. **OpenLDAP** — Install and configure:
   ```bash
   # macOS
   brew install openldap

   # Linux (Debian/Ubuntu)
   sudo apt install slapd ldap-utils
   sudo dpkg-reconfigure slapd
   ```

   Load the sudo schema and seed data:
   ```bash
   ldapadd -Y EXTERNAL -H ldapi:/// -f ldap/schema/sudo.ldif
   ldapadd -x -D "cn=admin,dc=example,dc=org" -W -f ldap/seed.ldif
   ```

4. **Environment variables** — Set connection URLs:
   ```bash
   export DATABASE_URL="postgres://localhost:5432/ldapwarden?sslmode=disable"
   export REDIS_URL="redis://localhost:6379"
   export LDAP_URL="ldap://localhost:389"
   export LDAP_BIND_DN="cn=admin,dc=example,dc=org"
   export LDAP_BIND_PASS="your-admin-password"
   ```

5. **Start the app** (the backend applies database migrations on startup):
   ```bash
   cd cmd/server && go run .      # Terminal 1
   cd web && pnpm install && pnpm dev  # Terminal 2
   ```

### Project Structure

```
ldapwarden/
├── cmd/server/          # Go backend entry point
├── internal/
│   ├── api/             # HTTP handlers & routes
│   ├── ldap/            # LDAP client wrapper
│   ├── auth/            # Authentication & sessions
│   ├── rbac/            # Permission checks
│   ├── audit/           # Audit logging
│   └── config/          # Configuration
├── db/
│   ├── migrations/      # SQL migrations
│   └── queries/         # sqlc queries
├── web/                 # React frontend
│   ├── src/routes/      # TanStack Router pages
│   ├── src/components/  # UI components
│   └── src/lib/         # API client, auth, utils
└── ldap/                # OpenLDAP seed data & schema
```

### Test Accounts

| Username | Password | Role |
|----------|----------|------|
| `admin` | `admin123` | Administrator |
| `jdoe` | `password123` | Developer (read-only) |
| `viewer` | `viewer123` | Read-only |

## Tech Stack

**Backend:**
- [Go](https://go.dev/) — Fast, compiled backend
- [Chi](https://github.com/go-chi/chi) — Lightweight HTTP router
- [go-ldap](https://github.com/go-ldap/ldap) — LDAP client library
- [sqlc](https://sqlc.dev/) — Type-safe SQL
- [PostgreSQL](https://www.postgresql.org/) — Audit logs & metadata
- [Redis](https://redis.io/) — Session storage

**Frontend:**
- [React 19](https://react.dev/) — UI framework
- [TanStack Router](https://tanstack.com/router) — Type-safe routing
- [TanStack Query](https://tanstack.com/query) — Data fetching
- [Tailwind CSS](https://tailwindcss.com/) — Styling
- [Radix UI](https://www.radix-ui.com/) — Accessible components
- [Zod](https://zod.dev/) — Schema validation

## Roadmap

- [x] LDAP over TLS (LDAPS/StartTLS) support
- [x] Password policy management (ppolicy overlay)
- [x] Samba integration (sambaSamAccount, sambaGroupMapping)
- [x] Modular UI configuration
- [ ] Multi-tenant / multiple LDAP backends
- [ ] Self-service password reset
- [ ] TOTP/2FA support
- [ ] SAML/OIDC integration
- [ ] Bulk user import/export (CSV, LDIF)
- [ ] Custom attribute support
- [ ] i18n / internationalization

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## AI-Assisted Development

This project was built with the help of AI coding assistant.

## License

This project is licensed under the Apache 2 License — see the [LICENSE](LICENSE) file for details.

---

<p align="center">
  Made with :heart: for sysadmins & DevOps
</p>
