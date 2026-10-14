# LDAP Warden

## Project Spec
See [docs/SPEC.md](docs/SPEC.md) for full architecture and requirements.

## Quick Start

```bash
# 1. Start infrastructure (PostgreSQL, OpenLDAP)
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
make test-web                 # Frontend unit tests (vitest)
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
  migrations/        # SQL migrations, embedded in the binary and applied
                     # by the integration tests from that same embed
web/                 # React frontend
  src/routes/        # TanStack Router pages
  src/components/    # UI components
  src/lib/           # API client, auth, utils
ldap/                # OpenLDAP 2.6 image for the bundled stack: schema,
                     # ppolicy overlay, seed data
```

## Code Conventions
- Go: Follow standard project layout; SQL is written by hand against pgx
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
- `POST /api/groups/:dn/change-cn` - Rename a group; rewrites the `%cn` sudo grants, refuses the admin group
- `DELETE /api/groups/:dn` - Reports the `%cn` grants and primary members it leaves behind, and refuses the admin group too
- `POST /api/users/:dn/change-posix-ids` - Change `uidNumber`/`gidNumber`; refuses a duplicate uid number, warns about file ownership
- `GET /api/groups/:dn/members` - Membership resolved to names
- `POST /api/admin/policy-selftest` - Prove whether ppolicy is enforced (writes a throwaway entry)
- `POST /api/admin/scheduled-tasks/lockout-scan/trigger` - Run the lockout scan now
- `GET /api/admin/scheduled-tasks/health` - Per configured task: last run, next run, and `overdue` when a scheduled run was missed
- `POST/DELETE /api/groups/:dn/members` - Group membership
- `GET/POST/PUT/DELETE /api/netgroups` - Netgroup CRUD; list takes `search`, `sort`, `order`, `limit`, `offset`
- `POST /api/netgroups/import` - Bulk create; triples arrive as `"(host,user,domain)"` text and are parsed server-side
- `GET /api/netgroups/:dn/expand` - Effective contents, nesting followed
- `GET /api/netgroups/:dn/sudo-roles` - Sudo rules naming `+cn`, on either side
- `GET /api/users/:dn/netgroups` - Netgroups reaching a user, direct or nested
- `GET /api/schema` - LDAP schema
- `GET /api/audit-logs` - Audit logs (admin only), filterable by actor / resource / action / date range
- `GET /api/audit-logs/export` - Audit logs as CSV, same filters
- `GET /healthz` - Liveness (process only); replaced the old `/health`
- `GET /readyz` - Readiness: pings Postgres (database and sessions) and LDAP; 503 when any is down

## Test Accounts (LDAP)
- admin / admin123 (system admin)
- jdoe / password123 (developer)
- viewer / viewer123 (read-only)

## Current Status
MVP implemented with:
- Dashboard, User/Group CRUD with form-based editing
- LDAP bind authentication
- Sessions in PostgreSQL (`internal/auth/session_store.go`); there is no Redis
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
  (`olcPPolicyDefault` or `pwdPolicySubentry`) — and the attributes stay writable
  regardless, so an unenforced directory reports accounts as locked while letting
  them authenticate. **Whether a policy applies is not detectable** from an
  ordinary bind identity: every readable signal is identical either way (see the
  comment on `Enforcement`). Hence three states, `unknown` by default, and
  `POST /api/admin/policy-selftest` as the only proof.
- **A lock is not self-describing.** `pwdAccountLockedTime` being present does not
  mean the account is locked: ppolicy lifts a timestamped lockout once
  `pwdLockoutDuration` elapses, but it has no timer — it re-reads the value at bind
  time and deletes it there, so the attribute outlives the lock until someone next
  tries to log in. `internal/ldap/lockout.go` resolves the value against the
  applicable policy into `none` / `manual` / `active` / `lifted` / `unknown`; ask
  `Lockout.Locked()` (Go) or `isLocked()` (`web/src/lib/account.ts`), never the raw
  attribute. Which policy applies needs `LDAP_DEFAULT_POLICY_DN` for entries
  without a `pwdPolicySubentry`, since `olcPPolicyDefault` lives in the unreadable
  `cn=config` — unset, such a lockout resolves to `unknown` and counts as locked.
- **The evidence of a lockout is perishable.** `pwdFailureTime` is
  NO-USER-MODIFICATION — slapd refuses it even from the rootdn, so tests have to
  produce real failed binds — and ppolicy prunes it to `pwdFailureCountInterval`,
  then deletes it with the lock at the account's next bind. The `lockouts_scan`
  task (`internal/scheduler/lockout_scan.go`) copies the timestamps into a
  `user.lockout` audit row while they exist; nothing else in the app records them.
  Its interval must stay shorter than the shortest `pwdLockoutDuration`, or
  lockouts start and expire between two runs.
- **A netgroup triple's empty slot means *any*, never *none*.** `nisNetgroupTriple`
  stores `(host,user,domain)`; `(,jdoe,)` is jdoe on every host, `(web01,,)` is every
  user on web01, and `-` is the NIS marker for "matches nothing". Turning a `-` into an
  empty slot widens a grant; reading an empty host list as "no hosts" reports the exact
  opposite of the truth. `NetgroupExpansion` therefore carries `OpenUser`/`OpenHost`
  alongside the name lists, and `AnyHost()` is the question to ask — never
  `len(Hosts) == 0`.
- **`nisNetgroupTriple` cannot be searched.** It is declared with a SYNTAX and *no*
  matching rules, so a filter over it is not evaluated — and OpenLDAP does not reject
  such a filter, it answers `Success` with zero entries. A search box built on one looks
  like it works and silently never finds a member. Only presence, `(nisNetgroupTriple=*)`,
  works server-side; everything else is filtered in Go on parsed triples. OpenLDAP does
  validate the syntax on write, so an unparsable value can only arrive from a server that
  does not.
- **A netgroup is named by string, from two places that do not track it.** Sudo says
  `+name` and other netgroups say `memberNisNetgroup`, and nothing in the directory ties
  either to the entry. Deleting a netgroup therefore leaves rules matching nobody, in
  silence — `DELETE /api/netgroups/:dn` collects those references before the delete and
  reports them. It is also why a netgroup's CN is immutable in the UI: renaming would
  need the migration `RenameUser` performs for users.
- **Which side a netgroup is used from decides which half of it applies.** `+name` in
  `sudoUser` reads only the user slots, in `sudoHost` only the host slots. A netgroup of
  users named in `sudoHost` matches nothing at all, with no error from sudo or the
  directory. `Netgroup.Kind()` derives users/hosts/mixed from the triples so the UI can
  warn; nothing is stored to declare it, because no other tool would honour it.
- **Writing a `+netgroup` grant is necessary and not sufficient.** Sudo resolves netgroups
  on the client — NSS, `netgroup_base` in `ldap.conf`, or SSSD — none of which is visible
  from the directory. Same shape as ppolicy enforcement: the app writes something it
  cannot verify, so it says so rather than implying the job is done. `getent netgroup
  <name>` on a host is what settles it.
- **A group is named by string from sudo, and by number from its members.** `%cn` in
  `sudoUser` is a bare string the directory does not track, so renaming a group has to
  rewrite those grants by hand — `RenameGroup` does, reporting what it could not reach.
  Members need nothing: `memberUid` holds usernames and primary members hold `gidNumber`,
  neither of which follows the name. Renaming the group named by `LDAPWARDEN_ADMIN_GROUP`
  is refused outright: admin is decided by comparing group names to that setting, so it
  would strip admin from everyone including whoever did it.
- **Clearing an attribute is a replace with no values, never a delete.** LDAP fails a
  delete of an attribute that is not there (`noSuchAttribute`, code 16), while a
  replace with an empty value set removes it if present and is a no-op if not. Since
  every detail form posts the fields it manages — including the empty ones — a delete
  turns the ordinary act of saving into a 500 for any entry that never had the
  attribute. Pass values through `clearing()` in `internal/ldap/client.go`.
- **Nothing in the directory keeps a `uidNumber` unique.** OpenLDAP enforces it only
  with the `unique` overlay, which the bundled stack does not configure — so the app
  checks occupancy itself, in `reserveUIDNumber`/`reserveGIDNumber`, as late as it can
  before the write. A number of 0 in a create request means "allocate one", which is
  what the user and group creation forms send when the field is untouched: filling it in
  client-side replayed a value read when the page loaded. For an account's gidNumber, 0 is
  only accepted with `createPrimaryGroup`, whose number is reserved before the account is
  written — otherwise it would be stored as is, and 0 is root's group. This is a check-then-write, not a
  constraint; two simultaneous creations can still collide, and the overlay is the only
  real guarantee.
- **A POSIX number is filesystem state, not directory state.** `uidNumber` is what
  every inode on every host stores, and nothing in the directory records where those
  files are — so changing it succeeds instantly and breaks access somewhere this app
  cannot see. `ChangePosixIDs` therefore returns what the operator must still run
  (`chown`), realigns a `sambaSID` whose RID came from the old number — equal to it,
  or Samba's `2*uid+1000`, and nothing else, since another scheme's RID belongs to a
  different principal — and refuses a
  `uidNumber` another account already holds: to the kernel that is one user, not two.
- **`memberUid` and `sudoUser` store a plain uid, not a DN**, so the directory
  cannot follow them: any operation on a user that changes or removes its uid has
  to rewrite them by hand. `RenameUser` and `DeleteUser` both do, best-effort,
  returning `warnings` for what they could not reach. Leaving one behind hands the
  groups and sudo grants to whoever gets that username next.
- **go-ldap decides on TLS from the URL scheme**, not from the mode: `ssl` on an
  `ldap://` URL connected in clear, and StartTLS sends no `ServerName`, so Go cannot
  verify. `config.ValidateTLS` refuses the first; `ldap.TLSConfig` sets the second.
  Every connection — the shared one and a sign-in's — goes through `Client.dial`.
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
  A list page registers that state with `useRememberListSearch`, and the back
  arrow of its detail and create pages is `<BackToList>`, which returns to it.
- **Which sections exist is decided once**, in `useSections()` (`lib/sections.ts`):
  module, directory capability and the permission the list endpoint requires. The
  nav bar, the mobile menu and the command palette all read it; never gate a link
  to a section anywhere else. The admin page is `useIsAdmin()`, the route's own test.
- A detail form ends in `<FormActions>`: Save is disabled until the form is dirty
  (the value `useSyncedForm` already returns for the unsaved-changes guard), and
  Discard puts the stored values back. An action that cannot be walked back —
  deleting a user or a policy, renaming — asks for the name typed, via
  `ConfirmDialog`'s `confirmText`; its error shows in the dialog, not a toast too.
- Mutation outcomes are reported by a **toast**. Inline `Callout`s are for
  page-level load failures, field validation and persistent notices — not for
  confirming a save.
- **What an operation leaves for the operator to do** — the `warnings` a rename,
  delete or POSIX change answers with — never goes in a toast: it expires, usually
  while the page navigates away. Pass it to `showOutcome()` (`lib/outcome.ts`),
  which queues a dialog mounted at the root that survives the navigation, can be
  copied and waits to be dismissed; it is a no-op on an empty list. Bulk actions
  list theirs per item in `BulkResultDialog`'s `warned` section.
- Colours come from theme tokens (`text-success`, `bg-warning/10`, the `.tone`
  class for categorical tints). Never use raw palette utilities such as
  `bg-green-100`: they only work in light mode.
  A surface that is light whatever the theme (the sign-in card) takes the
  `light` class, which re-declares the light tokens for its subtree; never
  force `bg-white text-gray-900` by hand, which leaves everything inside on
  dark-theme tokens.
- Loading states use the skeletons in `components/ui/skeleton.tsx`, rendered
  inside the real table so nothing shifts when data arrives.
