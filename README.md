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
  <img src="docs/screenshots/02-dashboard.png" alt="LDAP Warden — Dashboard" width="900">
</p>

<details>
<summary><strong>Login</strong></summary>

![Login](docs/screenshots/01-login.png)
</details>

<details>
<summary><strong>Users List</strong></summary>

![Users List](docs/screenshots/03-users-list.png)
</details>

<details>
<summary><strong>User Details</strong></summary>

![User Details](docs/screenshots/04-user-details.png)
</details>

<details>
<summary><strong>Groups List</strong></summary>

![Groups List](docs/screenshots/05-groups-list.png)
</details>

<details>
<summary><strong>Group Details</strong></summary>

![Group Details](docs/screenshots/06-group-details.png)
</details>

<details>
<summary><strong>Sudo Roles</strong></summary>

![Sudo Roles](docs/screenshots/07-sudo-roles.png)
</details>

<details>
<summary><strong>Audit Logs</strong></summary>

![Audit Logs](docs/screenshots/08-audit-logs.png)
</details>

<details>
<summary><strong>Administration</strong></summary>

![Administration](docs/screenshots/09-admin.png)
</details>

<details>
<summary><strong>Command Palette (⌘K)</strong></summary>

![Command Palette](docs/screenshots/10-command-palette.png)
</details>

## Features

- **Dashboard** — The directory at a glance on login: user population, locked and expiring accounts, recent changes, and whether the scheduled expiration jobs are still running
- **Command Palette** — `⌘K` / `Ctrl+K` searches users, groups, sudo roles and policies at once and doubles as a command list; `/` focuses the search box on any list
- **User Management** — Create, edit, and delete LDAP users with support for `inetOrgPerson`, `posixAccount`, and `shadowAccount` object classes
- **Group Management** — Manage POSIX groups and group memberships with an intuitive UI
- **Sudo Roles** — Full sudoers LDAP schema support with user and group-based sudo privilege management
- **Netgroups** — Manage `nisNetgroup` entries and grant sudo to all their members at once with `+name`; nesting is resolved so a rule shows who it actually reaches, and the app warns when a netgroup is named on the side sudo will ignore
- **Password Policies** — View and manage password policies (ppolicy overlay), assign policies to users
- **SSH Key Management** — Store and manage SSH public keys in LDAP (`ldapPublicKey` object class)
- **Samba Integration** — Optional support for `sambaSamAccount` and `sambaGroupMapping` object classes
- **LDAP Authentication** — Authenticate users directly against your LDAP directory; passwords are written via the RFC 3062 `PasswordModify` extended operation so the directory's hashing pipeline always runs
- **Role-Based Access Control** — Admin and read-only roles based on LDAP group membership, with active sessions invalidated immediately on user/admin-group mutation
- **Audit Logging** — Track every change in PostgreSQL with actor, IP and User-Agent, browsable with a per-attribute before/after diff, filterable by actor / resource / action / date range, exportable as CSV, plus opt-in per-change email notifications
- **Email Notifications** — Per-change audit emails, password-reset flow, and proactive account/password expiration reminders to admins and users
- **Security-First Defaults** — Per-IP rate limiting on auth endpoints, baseline CSP / X-Frame-Options / Referrer-Policy headers, `X-Forwarded-For` allowlist by trusted-proxy CIDR, and fail-fast on default secrets
- **Modular UI** — Enable/disable features based on your LDAP schema (sudo, policies, Samba)
- **Photo Support** — Upload and display user photos (JPEG)
- **Modern UI** — Clean interface built with React and Tailwind CSS: light and dark themes, usable down to a phone screen, keyboard-navigable, and every filtered/sorted view is a shareable URL

## Quick Start

> **Upgrading an existing install?** Read the *Upgrading* section of
> [CHANGELOG.md](CHANGELOG.md) first. The current release changes where an
> account's expiry is stored and moves the bundled directory to OpenLDAP 2.6;
> neither happens on its own.

### Prerequisites

- Docker and Docker Compose
- Git

### One-Command Setup

```bash
# Clone the repository
git clone https://github.com/yourusername/ldapwarden.git
cd ldapwarden

# Start all services (PostgreSQL, OpenLDAP, and LDAP Warden)
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
- **OpenLDAP 2.6** — Pre-configured LDAP server with sample data (port 389),
  built from `ldap/` with the ppolicy overlay enabled and `olcPPolicyDefault`
  pointing at `cn=default,ou=policies,dc=example,dc=org`. The directory is baked
  into the image and copied into the volumes on first start, so what you change
  through the app survives a restart; `docker compose down -v` restores the seed.
- **PostgreSQL** — Database for sessions, audit logs and task history (port 5432)
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
| `LDAP_NETGROUP_OU` | Organizational Unit for netgroups | `ou=netgroups` |
| `LDAP_PPOLICY_OU` | Organizational Unit for password policies | `ou=policies` |
| `LDAP_DEFAULT_POLICY_DN` | The policy in the overlay's `olcPPolicyDefault`, which applies to entries with no `pwdPolicySubentry`. `cn=config` is not readable with an ordinary bind identity, so set this to tell a lockout still in force from one `pwdLockoutDuration` has lifted | *(none)* |
| `LDAP_MIN_UID` | Minimum UID for new users | `1000` |
| `LDAP_MIN_GID` | Minimum GID for new groups | `1000` |
| `LDAP_SEARCH_SIZE_LIMIT` | Max entries returned by UI list/search endpoints (0 = no limit) | `500` |
| `LDAP_TLS_MODE` | TLS mode: `none`, `ssl` (needs an `ldaps://` URL), or `starttls` (needs `ldap://`) | `none` |
| `LDAP_TLS_SKIP_VERIFY` | Skip TLS certificate verification. Prefer `LDAP_TLS_CA_FILE`: with verification off, anyone on the path can collect the bind password | `false` |
| `LDAP_TLS_CA_FILE` | PEM bundle of the CAs trusted to sign the directory's certificate. Replaces the system CAs, as `TLS_CACERT` does in `ldap.conf` | *(system CAs)* |
| `LDAP_TLS_CERT_FILE` / `LDAP_TLS_KEY_FILE` | Client certificate and key, for a directory with `olcTLSVerifyClient: demand`. Both or neither | *(none)* |

The TLS files are read again at every reconnection, so a rotated certificate is
picked up without a restart. A combination that would not do what it says — a CA
file on a plaintext connection, `ssl` on an `ldap://` URL — stops the server.

> **POSIX number uniqueness.** LDAP Warden refuses to create a user or group on a
> `uidNumber`/`gidNumber` another entry already holds, and allocates one at write
> time when the field is left alone. That closes the cases that actually happen —
> a number typed twice, a CSV listing one twice, a creation form read minutes
> earlier. It is still a check-then-write, not a constraint: OpenLDAP enforces no
> uniqueness on these attributes by default, so two genuinely simultaneous
> creations remain possible. Configure the `unique` overlay on your server if you
> need the guarantee rather than the safeguard:
>
> ```
> olcOverlay: unique
> olcUniqueURI: ldap:///ou=People,dc=example,dc=org?uidNumber?sub
> olcUniqueURI: ldap:///ou=Groups,dc=example,dc=org?gidNumber?sub
> ```


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
| `LDAPWARDEN_MODULES` | Comma-separated list of enabled modules (tabs) | `users,groups,sudo,policies,netgroups` |
| `LDAPWARDEN_USERS_OBJECTS` | LDAP objectClasses for users | `inetOrgPerson,posixAccount,ldapPublicKey,shadowAccount` |
| `LDAPWARDEN_GROUPS_OBJECTS` | LDAP objectClasses for groups | `posixGroup` |
| `LDAPWARDEN_AUDIT_NOTIFY_EMAILS` | Comma-separated recipients that receive an email for every UI modification (audit/traceability). Empty disables. | (empty) |
| `LDAPWARDEN_TRUSTED_PROXIES` | Comma-separated CIDR list of reverse proxies allowed to set `X-Forwarded-For` / `X-Real-IP`. Empty disables forwarded-header support. | (empty) |
| `LDAPWARDEN_CORS_ORIGINS` | Comma-separated list of **additional** origins allowed to make credentialed requests. The origin serving the app is always allowed, so this is only needed for a separately-hosted frontend (e.g. `pnpm dev`). `*` is refused. | `http://localhost:5173,http://localhost:3000` |
| `LDAPWARDEN_DEV_MODE` | Bypass production-secrets validation (also tolerates plain `http://` `PublicURL`). Only for the bundled compose stack and local tests. | `false` |

#### Account expiry and locking

The two are different states, stored in different attributes, and both are
enforced by the directory rather than by LDAP Warden:

| State | Attribute | Effect |
|-------|-----------|--------|
| Locked | `pwdAccountLockedTime` = `000001010000Z` | binds refused until an administrator unlocks |
| Expires on a date | `pwdEndTime` | binds refused from that instant on, and never lifted automatically |

**Scheduled expiry needs OpenLDAP 2.5 or later**, which is where the ppolicy
overlay gained `pwdEndTime`. On an older directory the UI hides the expiration
controls and the API refuses to set one (HTTP 422) rather than writing something
the server would ignore; locking is unaffected. The check is a probe of the
subschema, refreshed with the schema-refresh action. The bundled compose stack
runs 2.6 with a default policy in effect, so both states work out of the box.

Two caveats worth knowing:

- **ppolicy enforces nothing unless a policy is in effect** for the entry, via
  `olcPPolicyDefault` on the overlay or a `pwdPolicySubentry` on the user. With no
  policy, `pwdEndTime` and `pwdAccountLockedTime` are inert attributes.
- **`shadowExpire` is not an account expiry as far as the directory is
  concerned** — slapd ignores it; only PAM/SSSD on your hosts read it. It stays
  editable on the Shadow tab as a raw attribute, but it does not drive the account
  status, the dashboard or the expiration notifications.

Earlier versions stored the expiry in `pwdAccountLockedTime`, which OpenLDAP reads
as *when a lockout began*. A scheduled future date therefore disabled the account
immediately on 2.4 (and was silently ignored on 2.6), and a past one stopped
applying once `pwdLockoutDuration` elapsed. To move existing data across:

```bash
# report what would change (default)
ldapwarden migrate-account-expiry

# apply
ldapwarden migrate-account-expiry -apply
```

Past lock timestamps and `shadowExpire` values are reported but left alone unless
you pass `-include-past-locks` / `-include-shadow-expire`: those accounts work
today, and migrating them would start refusing them.

#### Module Configuration

LDAP Warden allows you to customize which features are visible based on your LDAP setup:

**`LDAPWARDEN_MODULES`** controls which high-level navigation tabs are shown:
- `users` — User management
- `groups` — Group management
- `sudo` — Sudo roles management (requires sudoers schema)
- `policies` — Password policies (requires ppolicy overlay)
- `netgroups` — Netgroup management (requires `nis.schema`); the tab hides
  itself on a directory that does not declare `nisNetgroup`

> **Directory schema prerequisites.** The `sudo` and `policies` modules need
> the corresponding schema loaded **on your LDAP server** — the bundled compose
> stack loads them automatically, but an existing production directory may not
> have them. Without it, creating the first sudo role or password policy fails
> because the `sudoRole` / `pwdPolicy` object class is undefined (LDAP result
> code 21 "invalid per syntax", surfaced in the UI as "a required object class
> or attribute is missing"). Load the schema files this repo ships under
> `ldap/`:
>
> ```bash
> # Sudo roles
> ldapadd -Y EXTERNAL -H ldapi:/// -f ldap/schema/sudo.ldif
> # Password policies — enable the ppolicy overlay (skip if already configured).
> # From OpenLDAP 2.5 the pwdPolicy schema is built into the overlay module, so
> # loading the module is all it takes; adjust olcPPolicyDefault in the file to
> # a policy entry that exists in your tree.
> ldapmodify -Y EXTERNAL -H ldapi:/// -f ldap/config/ppolicy-overlay.ldif
> ```
>
> The `netgroups` module needs no such step: `nis.schema` is part of OpenLDAP's
> default schema set and is normally already loaded. The app probes for
> `nisNetgroup` and answers 501 on a directory that does not have it, rather than
> reporting an empty list as though there were simply no netgroups.
>
> These LDIFs target OpenLDAP's `cn=config`. For a different directory server
> (389-DS, FreeIPA, AD) use that server's equivalent sudo/ppolicy schema.

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

#### Cross-origin requests

Cookie-authenticated mutations must carry an `Origin` (or `Referer`) that
LDAP Warden recognises, which closes the CSRF cases `SameSite=Lax` does not.

**The origin serving the app is always accepted** — when the binary serves both
the UI and the API, as the Docker image does, nothing needs configuring.
`LDAPWARDEN_CORS_ORIGINS` is for the case where the frontend lives on a
*different* origin, such as the Vite dev server during development.

Behind a TLS-terminating reverse proxy, the browser's origin is `https://…` while
the connection reaching LDAP Warden is plain HTTP. Set
`LDAPWARDEN_TRUSTED_PROXIES` so the proxy's `X-Forwarded-Proto` is honoured;
otherwise the scheme cannot be reconstructed and same-origin requests are refused
(list the public origin in `LDAPWARDEN_CORS_ORIGINS` as a fallback). A proxy that
rewrites the `Host` header also needs the public origin listed there, since
`X-Forwarded-Host` is deliberately ignored.

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

`SESSION_SECRET`, `LDAP_BIND_PASS`, `DATABASE_URL` and `MAIL_PASSWORD` can also
be read from a file: set `LDAP_BIND_PASS_FILE=/run/secrets/ldap_bind_pass` instead
of the variable itself, the convention Docker and Kubernetes secrets are mounted
for. Trailing newlines are stripped. Setting both, or pointing at a file that is
missing or empty, stops the server rather than falling back to the default — in
dev mode too.

### Mail

| Variable | Description | Default |
|----------|-------------|---------|
| `MAIL_HOST` | SMTP server hostname | `localhost` |
| `MAIL_PORT` | SMTP server port | `1025` |
| `MAIL_USER` | SMTP username | (empty) |
| `MAIL_PASSWORD` | SMTP password | (empty) |
| `MAIL_FROM` | Sender email address | `noreply@example.org` |
| `MAIL_FROM_NAME` | Optional sender display name (e.g. `LDAP Warden`) | (empty) |
| `MAIL_SSL` | SSL mode: `none`, `starttls`, or `ssl` | `none` |

### Scheduled Tasks

| Variable | Description | Default |
|----------|-------------|---------|
| `LDAPWARDEN_SCHEDULED_TASKS_USERS_EXPIRATION` | Cron schedule for account expiration checks | `42 3 * * *` |
| `LDAPWARDEN_SCHEDULED_TASKS_PASSWORDS_EXPIRATION` | Cron schedule for password expiration checks | `42 3 * * *` |
| `LDAPWARDEN_SCHEDULED_TASKS_LOCKOUT_SCAN` | Cron schedule for the lockout scan. Must be more frequent than the shortest `pwdLockoutDuration`, or lockouts start and expire between two runs | `*/5 * * * *` |

Set a variable to an empty string to disable that task.

#### How Background Tasks Work

LDAP Warden uses a cron-based scheduler (via [robfig/cron](https://github.com/robfig/cron)) to run periodic background tasks. Three tasks are available:

**Account Expiration (`users_expiration`)**
- Checks all users for upcoming account expiration (via `pwdEndTime`, the only attribute slapd enforces as a validity limit)
- Sends email notifications to **administrators** (members of `LDAPWARDEN_ADMIN_GROUP`)
- Useful for reminding admins to extend or disable accounts before they lock out

**Password Expiration (`passwords_expiration`)**
- Checks all users for upcoming password expiration (via `shadowMax` or ppolicy `pwdMaxAge`)
- Sends email notifications directly to **each user** (requires `mail` attribute)
- Helps users proactively change their passwords before expiration

**Lockout Scan (`lockouts_scan`)**
- Finds accounts the *directory* locked after too many failed sign-ins — not the ones
  an administrator locked here, which are audited and emailed where that happens
- Records each lockout in the audit log as `user.lockout`, **with the failed-bind
  timestamps**: ppolicy prunes `pwdFailureTime` to `pwdFailureCountInterval` and
  wipes it at the account's next sign-in, so the audit row is where that evidence
  survives
- Emails one digest per run — an account that locks a dozen accounts in a minute
  should not produce a dozen alerts — to `LDAPWARDEN_AUDIT_NOTIFY_EMAILS`, or to the
  admin group when that is not set
- Runs often by design. A lockout is visible for `pwdLockoutDuration` and no longer,
  so a slower schedule misses them; the run says so when the two disagree

The two expiration tasks send notifications at these intervals before expiration:
- 3 weeks before
- 1 week before
- 1 day before
- On expiration

Notifications are tracked in PostgreSQL to prevent duplicate emails for the same
interval — and, for lockouts, for the same lockout. "Recorded" and "emailed" are
tracked separately there, so an unreachable mail server delays a lockout alert
instead of either losing it or re-recording it on every run.

Tasks can also be triggered manually from the Administration UI, or via
`POST /api/admin/scheduled-tasks/{users-expiration,passwords-expiration,lockout-scan}/trigger`.

## Development

### Prerequisites

- Go 1.23+
- Node.js 20+
- pnpm
- Docker (for infrastructure) or local PostgreSQL and OpenLDAP

### Setup with Docker (Recommended)

```bash
# Start infrastructure (PostgreSQL, OpenLDAP)
docker compose up -d postgres openldap

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

2. **OpenLDAP** — Install and configure:
   ```bash
   # macOS
   brew install openldap

   # Linux (Debian/Ubuntu)
   sudo apt install slapd ldap-utils
   sudo dpkg-reconfigure slapd
   ```

   Load the sudo schema, enable the ppolicy overlay and add the seed data. The
   overlay is what makes locking and expiry work at all — see [Account expiry and
   locking](#account-expiry-and-locking):
   ```bash
   ldapadd -Y EXTERNAL -H ldapi:/// -f ldap/schema/sudo.ldif
   ldapmodify -Y EXTERNAL -H ldapi:/// -f ldap/config/ppolicy-overlay.ldif
   ldapadd -x -D "cn=admin,dc=example,dc=org" -W -f ldap/seed.ldif
   ```

3. **Environment variables** — Set connection URLs:
   ```bash
   export DATABASE_URL="postgres://localhost:5432/ldapwarden?sslmode=disable"
   export LDAP_URL="ldap://localhost:389"
   export LDAP_BIND_DN="cn=admin,dc=example,dc=org"
   export LDAP_BIND_PASS="your-admin-password"
   ```

4. **Start the app** (the backend applies database migrations on startup):
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
│   ├── config/          # Configuration
│   └── migrations/      # SQL migrations (embedded in the binary)
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
- [pgx](https://github.com/jackc/pgx) — PostgreSQL driver
- [PostgreSQL](https://www.postgresql.org/) — Sessions, audit logs & metadata

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
