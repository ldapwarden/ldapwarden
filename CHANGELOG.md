# Changelog

Versions are date-based (`YYYY.MM.DD.HHMMSS`, UTC), cut by tagging — see
[Cutting a Release](CONTRIBUTING.md#cutting-a-release).

The topmost section here becomes the body of the GitHub release, so anything an
operator has to *do* before upgrading belongs in an **Upgrading** block at the top
of it.

## Unreleased

Read **Upgrading** first: this release changes where an account's expiry is
stored, moves the bundled Docker Compose stack to OpenLDAP 2.6, turns the new
netgroups module on by default, and drops Redis. The first two are not automatic.

### Upgrading

**1. Migrate account expirations.** Earlier versions stored a scheduled expiry in
`pwdAccountLockedTime`, which OpenLDAP reads as *when a lockout began* — so a
future date disabled the account immediately on 2.4 and was ignored on 2.6. The
expiry now lives in `pwdEndTime`, which the directory enforces as a validity
window. Existing data does not move on its own:

```bash
ldapwarden migrate-account-expiry          # report what would change (default)
ldapwarden migrate-account-expiry -apply   # move future lock timestamps across
```

The default run migrates only the accounts that are broken right now — a
`pwdAccountLockedTime` in the future. Two cases are reported and deliberately left
alone, because migrating them would start refusing accounts that currently work:

- **Lock timestamps in the past.** ppolicy lifted them once `pwdLockoutDuration`
  elapsed, so those accounts authenticate today, and the attribute stays on the
  entry until their next successful bind makes ppolicy clear it. With
  `LDAP_DEFAULT_POLICY_DN` set (see below) the UI reports these as *lockout
  expired* rather than as locked. Unlock them from the UI to clear the attribute
  now, or pass `-include-past-locks` if the dates were meant as expiries.
- **`shadowExpire`.** slapd never read it; only PAM/SSSD on your hosts do. Pass
  `-include-shadow-expire` to move enforcement to the directory, which also means
  taking it away from the hosts.

**2. The bundled directory is now OpenLDAP 2.6, on new volumes.** The compose
volumes were renamed (`openldap_data`/`openldap_config` →
`directory_data`/`directory_config`) because the old `cn=config`, written by the
retired `osixia` image, points at module paths that do not exist in the new one.
`docker compose up -d` therefore starts from empty volumes and rebuilds the
directory from `ldap/seed.ldif` — which looks like a reset. The old volumes are
untouched; move real data across yourself (`slapcat` out, `ldapadd` in) or pin the
old names in a `compose.override.yaml`. To discard them:

```bash
docker volume rm ldapwarden_openldap_data ldapwarden_openldap_config
```

**3. `/health` has been replaced by `/healthz`.** The old path is gone, so move any
`livenessProbe`, `HEALTHCHECK` or uptime monitor pointing at it — a probe left on
`/health` gets a `404`, and a failing liveness probe restarts the container. Point
load balancers and `readinessProbe`s at the new `/readyz` instead, which checks
PostgreSQL and the directory rather than only that the process answers.

**4. Two API responses changed shape.** `GET /api/admin/config` renames
`capabilities.scheduledAccountExpiry` to `capabilities.scheduledExpirySupported`
and adds `capabilities.policyEnforcement`; `DELETE /api/users/{dn}` now returns
`{message, warnings}`. Both are consumed only by the bundled frontend.

**5. Set `LDAP_DEFAULT_POLICY_DN`** to whatever `olcPPolicyDefault` names on the
ppolicy overlay — for the bundled stack,
`cn=default,ou=policies,dc=example,dc=org`. Without it, ldapwarden cannot know how
long a lockout lasts for entries that carry no `pwdPolicySubentry`, and reports
every lock on them as `unknown`, i.e. still locked. `cn=config` is not readable
with an ordinary bind identity, which is why this has to be told rather than
discovered.

**6. The lockout scan starts by itself.** `lockouts_scan` defaults to
`*/5 * * * *`, so after upgrading, the accounts your directory locks will start
producing `user.lockout` audit entries and digest emails — to
`LDAPWARDEN_AUDIT_NOTIFY_EMAILS`, or to the admin group when that is unset. Set
`LDAPWARDEN_SCHEDULED_TASKS_LOCKOUT_SCAN=""` to keep it off, and check the cadence
against your shortest `pwdLockoutDuration`: slower than that, and lockouts start and
expire between two runs.

**7. `LDAPWARDEN_MODULES` gains `netgroups`.** The default is now
`users,groups,sudo,policies,netgroups`, so a directory that declares `nisNetgroup`
grows a Netgroups tab on upgrade. Nothing is created and nothing changes in the
directory; where the object class is absent the tab hides itself rather than
leading to a dead page. Set the variable explicitly to keep the old set. If you
already set it, it is honoured as before — add `netgroups` to it to get the tab
(`9835390`).

**8. Redis is no longer used.** It held the sessions and nothing else; they now
live in PostgreSQL, in a `sessions` table migration 007 creates on startup. Stop
the Redis service and drop `REDIS_URL` — the app logs a notice while it is still
set, and ignores it. Everyone signs in again once after the upgrade, since the
sessions in Redis are not carried over. Expired sessions are purged by the
existing hourly cleanup (`LDAPWARDEN_SCHEDULED_TASKS_TOKENS_CLEANUP`). The bundled
compose file no longer has a `redis` service; its old volume is left in place
(`docker volume rm ldapwarden_redis_data`).

**9. Check the LDAP TLS settings; a contradiction now stops the server.**
`LDAP_TLS_MODE=ssl` needs an `ldaps://` URL: on `ldap://` it used to connect in
clear, because go-ldap only encrypts the `ldaps` scheme. `starttls` needs an
`ldap://` URL. A CA file or client certificate set on an unencrypted connection,
or `LDAP_TLS_SKIP_VERIFY` together with `LDAP_TLS_CA_FILE`, is refused too.

**Prerequisite, unchanged but easy to miss:** ppolicy enforces neither locking nor
expiry unless a policy applies to the entry (`olcPPolicyDefault` on the overlay,
or `pwdPolicySubentry` on the entry) — and the attributes stay writable either
way, so an unenforced directory reports accounts as locked while letting them
authenticate. The admin page now says what it knows, and
`POST /api/admin/policy-selftest` proves it.

### Added

- **Dashboard** on the landing page: counts, accounts expiring soon, recent
  activity (`814a198`).
- **Command palette** (⌘K / Ctrl-K) and `/` to focus the search box (`3119a48`).
- **Audit log rework**: per-attribute diffs, filters by actor / resource / action /
  date range, and CSV export (`60165ef`).
- **Account expiry on `pwdEndTime`**, with schema detection: the UI hides the
  controls and the API answers `422` on a directory without it rather than writing
  an attribute the server ignores. The scheduler and the expiry notifications read
  the same attribute (`af9f212`).
- **`ldapwarden migrate-account-expiry`**, dry-run by default (`af9f212`).
- **Policy enforcement reporting and self-test**: three states
  (`enforced`/`not-enforced`/`unknown`, never guessed) plus
  `POST /api/admin/policy-selftest`, which creates a throwaway entry, locks it,
  tries to authenticate as it and removes it — the only definitive answer
  (`f79f44c`).
- **Lock states, instead of "the attribute is there"**: every user payload carries
  a `lockout` object resolving `pwdAccountLockedTime` against the policy that
  governs the entry — `none` / `manual` / `active` (with the instant it lifts) /
  `lifted` / `unknown`. ppolicy has no timer: it clears a lapsed lockout when the
  entry is next bound to, so the attribute outlived the lock and the UI reported an
  account as locked for as long as nobody tried to log in. The user page, the list
  status column, the `status=active` filter and the dashboard's `locked` figure all
  answer from the resolved state now, and a lapsed lockout says so instead of
  reading as a lock. Needs the new `LDAP_DEFAULT_POLICY_DN` for entries without
  their own policy — see Upgrading.
- **Lockout notifications** (`lockouts_scan`, every 5 minutes by default): the
  accounts the *directory* locked after too many failed sign-ins now get reported,
  where previously only administrator actions did. Each one produces a
  `user.lockout` audit entry **carrying the failed-bind timestamps and their
  spread** — ppolicy prunes `pwdFailureTime` to `pwdFailureCountInterval` and
  deletes it at the account's next sign-in, so that row is the lasting record —
  plus one digest email per run, listing each account with the window it is locked
  in. Recipients are `LDAPWARDEN_AUDIT_NOTIFY_EMAILS`, or the admin group when that
  is unset. Recorded and emailed are tracked separately, so an unreachable mail
  server delays an alert rather than losing it or repeating the audit entry. The
  schedule is a correctness setting, not a preference: a lockout is only visible for
  `pwdLockoutDuration`, and the run says so when the cadence is slower than the
  shortest policy's.
- **Netgroups**, a module of its own: `nisNetgroup` entries with full CRUD, nesting
  through `memberNisNetgroup` resolved on demand, CSV import, and the memberships
  that reach a user shown on their page — direct and inherited kept apart, because
  an inherited one still confers what a sudo rule grants but is not the entry to
  edit to take it away. Off unless the directory declares the object class; see
  Upgrading for the module default.

  The feature is shaped around three properties of the NIS schema that make silent
  wrongness the default, and every one of them is surfaced rather than smoothed
  over. An empty slot in a `(host,user,domain)` triple means **any**, never *none*
  — `-` is the marker for "matches nothing" — so the UI spells each slot out in
  prose and the expansion carries `openUser`/`openHost` alongside the name lists,
  since an empty list and an open slot mean opposite things. `nisNetgroupTriple`
  declares no matching rules, so the directory cannot evaluate a filter over it and
  OpenLDAP answers *success with zero entries* rather than refusing: everything but
  a presence test is filtered in Go over parsed triples. And a netgroup is named by
  string from two places nothing tracks — `+name` in sudo and `memberNisNetgroup`
  in other netgroups — so deleting one reports the references it leaves pointing at
  nothing, and a netgroup's `cn` cannot be changed here at all: renaming one would
  have to rewrite every `+name` and `memberNisNetgroup` that reaches it, and
  nothing in the directory can find them.

  Renaming or deleting a user rewrites the triples that name them, best-effort,
  reporting what it could not reach — the uid in a triple is a plain string like
  `memberUid` and `sudoUser`, so the directory will not follow it either
  (`76a8296`, `e961b1d`, `61eb7a6`, `1b941c3`, `8685626`, `ae026ee`, `22bbc8d`,
  `af31ca9`, `d062cee`).
- **Sudo grants to a netgroup**, written as `+name`, with the two things that
  string cannot say on its own. Which side it is named from decides which half of
  the netgroup applies — `sudoUser` reads only its user slots, `sudoHost` only its
  hosts — so a netgroup on the wrong side matches nobody, silently, and the rule
  now says so. And the rule shows who it actually reaches, resolved through
  nesting, instead of leaving the reader to open the netgroup. A netgroup naming
  both users and hosts is reported as *partly* applying, not as broken: half of it
  still works. Sudo resolves netgroups on the client — NSS, `netgroup_base`, SSSD —
  none of which is visible from here, so the app says the grant is necessary and
  not sufficient rather than implying the job is done
  (`045e0aa`, `a95a03f`, `857a251`, `dbd0090`, `0b9a8ba`).
- **A user's `uidNumber` and `gidNumber` can be changed**, on their own control
  rather than in the form they sit in. Every other attribute this app edits lives
  only in the directory; these two are the identity the filesystem uses, and
  nothing here can follow them there — files on every host keep the old numbers,
  the account loses access to its own data, and whoever is issued the old number
  next inherits it. So the change reports what it did not do, including the
  `chown` left to run. A `sambaSID` whose RID came from the old number is
  realigned; one allocated by a domain is left alone and reported, since its RID
  belongs to a different principal. A
  `uidNumber` another account already holds is refused: to the kernel that is one
  user, not two.
- **Renaming a group**, on its own control like the user rename, with the sudo
  grants that name it as `%cn` rewritten. Members are untouched: a POSIX group is
  joined by `memberUid` and by `gidNumber`, neither of which follows the name.
  Renaming the group named by `LDAPWARDEN_ADMIN_GROUP` is refused — admin is
  decided by comparing group names to that setting, so it would strip admin from
  everyone including whoever ran it (`20444b0`).
- **The running build, in a footer**: version and short commit, stamped into the
  binary at release and read from the server rather than from the bundle, so a
  browser holding a stale page still reports what it is talking to. A build from a
  modified tree says so, since it is not the commit it names (`ae5e1e0`).
- **`GET /api/server-info`**, readable by any authenticated session: the enabled
  modules, the configured object classes and the directory capabilities — the
  things the UI needs to decide what to render. See Fixed for why (`1408097`).
- **Group membership lists flag accounts that cannot use what the group grants.**
  A membership review is where dead access is meant to be spotted, and a locked
  account looked exactly like one in daily use (`c312790`).
- **The applicable password policy is stated on the user page**, and changing it is
  confirmed rather than saved silently (`6afc0d9`).
- An expiring session is explained and warned about before it lapses (`2fc2cc1`),
  a skip link reaches the main content (`acd8f58`), and `prefers-reduced-motion` is
  honoured (`8123966`).
- The CSV import shows its four steps, offers a template with the column names, and
  lists every error on a row instead of the first (`afc6018`).
- A frontend test harness (vitest, in CI as `make test-web`), covering the pure
  logic in `src/lib` (`f591094`).
- **`GET /api/groups/{dn}/members`**, resolving `memberUid` to names server-side
  (`69ecea2`).
- **`GET /healthz` and `GET /readyz`**, the Kubernetes names, split the way the
  convention intends: liveness reports only that the process answers, readiness
  pings PostgreSQL, the session store and the directory and returns `503` naming whichever is
  down. `/healthz` replaces `/health`, which answered `200` whatever had failed
  underneath it — see Upgrading.
- Password fields that can be revealed, with a generator and a strength meter
  (`c6ccef0`).
- Skeleton loaders, actionable empty states, and list/tab state kept in the URL so
  every view is a shareable link (`67d5819`, `2d63f78`, `902c640`).
- Bulk actions report every failure, not just the first (`182fa6d`).

- **TLS to the directory**: `LDAP_TLS_CA_FILE` for a private CA (replacing the
  system pool), `LDAP_TLS_CERT_FILE`/`LDAP_TLS_KEY_FILE` for a client certificate.
  Files are re-read at each reconnection, so a rotated certificate is picked up
  without a restart. A failed handshake now says which TLS settings were in play.
- **Secrets from files**: `SESSION_SECRET`, `LDAP_BIND_PASS`, `DATABASE_URL` and
  `MAIL_PASSWORD` can each be read from the file named by the same variable with a
  `_FILE` suffix (Docker and Kubernetes secrets). Setting both, or a file that is
  missing or empty, stops the server.

### Changed

- **Bundled directory: OpenLDAP 2.6**, built from `ldap/` with the ppolicy overlay
  and `olcPPolicyDefault` wired up. `osixia/openldap` has been unmaintained since
  2021 and stuck on 2.4.57, and the `bitnami` namespace was withdrawn from Docker
  Hub in August 2025 (`507a02b`).
- **Lists page, sort and filter server-side** — users, groups, sudo roles and
  password policies. The browser receives one page of a narrow projection
  (`3a09547`, `55f046f`).
- **Pickers search the directory** instead of downloading the collection and
  filtering it locally, which silently stopped working past
  `LDAP_SEARCH_SIZE_LIMIT` (`69ecea2`).
- **The dashboard counts instead of fetching.** Each figure is now one search with
  no attributes requested, so the numbers are exact — and on a 706-entry directory
  the payload is smaller too (55 kB against 82 kB), though that saving narrows as
  the directory grows. Consider indexing `pwdAccountLockedTime` and `pwdEndTime`,
  which these filters name and slapd does not index by default.
  `LDAP_SEARCH_SIZE_LIMIT` no longer bounds them — it exists to keep list payloads
  small — so the permanent "these counts are lower bounds" warning is gone from
  every directory of more than 500 entries. It now appears only when the *server*
  limits the bind identity, which is something an operator can act on.
- **Netgroup changes are recorded with what they did to the grant.** Create,
  update, delete and import write a per-attribute diff like users and groups
  always have; before, the journal said only that something had been modified. Each
  triple is recorded with its meaning beside its stored spelling, as
  `(,jdoe,) — jdoe on any host`, because an audit diff is exactly where
  `(,jdoe,)` and `(-,jdoe,)` sit one character apart and mean opposite things. A
  delete records the whole contents, since nothing else will hold them afterwards
  (`62cc2bc`).
- **The UI no longer decides what to render from `/api/admin/config`.** That
  endpoint keeps its `settings:read` permission and still backs the settings page;
  everything about *what this deployment can do* moved to `/api/server-info` — see
  Fixed (`1408097`).
- Selection and bulk delete are one model across all four lists (`37e3b22`), and
  bulk lock/unlock asks for confirmation, shows progress and keeps its bar in view
  (`efe91cb`).
- Dependencies updated, and four that nothing imported were dropped
  (`ae15a3c`, `ec340bc`), including a move off the deprecated `httprate.LimitByIP`
  (`b50ec5d`); the runtime base moved to Alpine 3.24 (`0d15d40`); CI
  gained a static-analysis job and runs the integration suite under `-race`
  (`3550d03`).
- The unused sqlc layer is gone: every query in the tree is hand-written against
  pgx, and the generated code had stopped compiling (`690ea77`, `56a0633`,
  `6912ce0`).
- Locking writes `000001010000Z`, the value ppolicy never lifts, instead of the
  current time — which ppolicy lifted after `pwdLockoutDuration`, turning "Locked"
  into "unlocked" half an hour later with nothing recorded (`af9f212`).
- `shadowExpire` no longer drives account status, the dashboard or the
  notifications; slapd ignores it. It stays editable as a raw attribute
  (`af9f212`).
- CI runs the frontend checks (`tsc`, `eslint`, `vite build`) and the database
  migrations, so a broken frontend fails the PR rather than the release
  (`eb31244`).
- Frontend dependencies updated in-range: the Radix primitives, `lucide-react`,
  `react-hook-form`, Vite 8.2, ESLint 10.8, `@vitejs/plugin-react`, `postcss` and
  `globals`; `pnpm audit` reports no known vulnerabilities. Go modules were already
  current. TypeScript stays on 6: it builds under 7, but `typescript-eslint`
  refuses to load against it, so linting stops working
  ([typescript-eslint#10940](https://github.com/typescript-eslint/typescript-eslint/issues/10940))
  (`cfe2905`).

### Fixed

- **StartTLS with certificate verification never connected**: go-ldap leaves the
  TLS `ServerName` empty and Go refuses to verify without one, so `starttls` only
  ever worked with `LDAP_TLS_SKIP_VERIFY`. The name now comes from `LDAP_URL`.

- **A user's sign-in had no LDAP timeout**: the connection a password is checked
  on carried its own copy of the dial code, without the dial timeout or the
  per-request deadline, so a directory that stopped answering hung the login
  request for good. Both connections now go through one dial.

- **A failed lookup on a netgroup page rendered as a spinner that never
  stopped.** The expansion views had two states, data and skeleton, and read only
  `data` from the query — so once it had failed and stopped retrying, the panel
  went on promising an answer that was never coming. The sudo-usage panel was
  worse than that: a failed search rendered as its empty state, telling an
  operator that no sudo rule names the netgroup when in fact nobody had looked.
  All three now say what went wrong and offer to try again, and the empty state
  means only what it says.
- **A search that read a whole collection had no time limit**, so a slow
  directory left the request hanging until the connection deadline with nothing
  to report. Searches now carry a server-side time limit just inside that
  deadline: the server giving up and saying so is an answer. The size limit is
  deliberately still absent on these reads — a truncated collection would produce
  confidently wrong answers, since a netgroup missing from the set reads as a
  dangling reference.
- **Adding a host to a netgroup answered `500`** whenever that netgroup nested no
  other netgroup — which is most of them. The page posts every field it manages,
  so `members` arrived as an empty list, and the update asked the directory to
  *delete* an attribute the entry never had. LDAP calls that `noSuchAttribute`,
  result code 16. The same defect hit groups: clearing a description a group did
  not have, or emptying an already-empty membership. Clearing an attribute is now
  a replace with no values, which removes it if present and does nothing if not,
  so no caller has to know whether it is there. Users, sudo roles and password
  policies already guarded against it by checking the current entry — the two that
  did not were the ones written later.
- **Two accounts could be created on the same `uidNumber`**, and two groups on the
  same `gidNumber`. Nothing checked whether a number was in use — `validatePOSIXID`
  tests only the range — and OpenLDAP enforces no uniqueness on these attributes
  unless the `unique` overlay is configured, which the bundled stack does not do.
  Every creation path was affected: a number typed into the form, a CSV listing one
  twice, and two people creating accounts at once, since the form replayed a number
  read when the page loaded. To the kernel two accounts on one number are one user —
  same files, same access, and nothing afterwards can tell them apart. Occupancy is
  now checked immediately before the write, an omitted number is allocated
  server-side at that moment, and a taken one is refused with `409` naming the
  holder. It remains a check-then-write: see the README for the overlay that makes
  it a constraint.
- **Deleting a user left their uid in every group's `memberUid` and in any
  `sudoRole`'s `sudoUser`.** Recreating an account with the same username — routine
  when one is recycled — silently inherited the previous holder's groups and sudo
  grants (`1c44313`).
- **Read-only users were served a UI missing features they were allowed to use.**
  Everything the frontend used to decide what to render came from
  `/api/admin/config`, which takes `settings:read` — a permission the `readonly`
  role does not have. The 403 left every answer undefined and every gate defaulted
  to off: no Netgroups tab, no membership section, no netgroup panels on sudo
  rules, and the wrong tabs on the user and group pages, since the configured
  object classes fell back to a hardcoded guess instead of the deployment's own.
  Nothing reported any of it. `/api/server-info` answers the same questions for any
  authenticated session (`1408097`).
- **A missing base entry answered `500 internal server error`.** Pointing the app
  at a directory with no `ou=netgroups` — or no `ou=sudoers`, or no `ou=policies`,
  which is the normal state of a directory that never needed them — failed every
  request for that collection while naming the wrong culprit: the fix is an
  `ldapadd` or one line of configuration, not a bug report. LDAP result code 32
  now answers `501` naming the setting to change, and the same for a create, whose
  parent is what is missing. A missing *entry* stays an ordinary `404`. Startup
  logs each base the directory does not hold, which is when an operator can still
  act (`c66daf6`).
- **The `lockout_events` migration was not embedded in the binary**, so a release
  ran without the table the lockout scan writes to (`9d5860e`).
- **The dashboard's numbers could vanish and never come back.** The capability
  probe held its lock across unbounded directory searches, so one slow or silent
  LDAP server wedged every later caller for the life of the process — taking out
  the dashboard and the admin page while everything backed by PostgreSQL kept
  working, which made it look like anything but a lock. Every operation is bounded
  now (`1016185`).
- **`GET /users/{dn}/sudo-roles` and `/groups/{dn}/sudo-roles` answered `400` for
  every request**, having resolved the DN against the sudoers base instead of the
  user's or group's own (`87f5390`).
- A sudo rule reported a netgroup as non-existent when it merely sat outside the
  page that was searched — condemning a working rule on the one screen whose job is
  to say when a rule matches nobody. Absence is now claimed only from a complete
  result set (`91b9155`).
- The destructive colour failed contrast as text in both themes (`d86fad3`); an
  over-tall dialog would not scroll (`581d123`); form field rows overflowed narrow
  screens (`ee86baa`); switches had no accessible name (`9b8ec1d`); list avatars
  were announced to screen readers (`a6f8053`).
- Detail forms stopped adopting the server's copy after the first save, so a
  background refetch left the field stale and claimed unsaved changes that were
  someone else's edit (`35be26d`).
- Raw Zod errors reached the user (`f56522e`); a rejected password did not say
  which rule it broke (`af2aaa9`); lockout-scan runs were reported as password
  expiration on the admin page (`fdbeabb`); a row did not say which kind of lock it
  was reporting (`0f12ed6`); the sudo roles search box ignored the `/` hotkey
  (`004f73f`).
- **Same-origin mutations were refused**, so the deployment described in the README
  could read but not write (`dbec0bf`).
- **An administrator could lock themselves out** via lock, delete, remove-password,
  rename, expiration or `shadowExpire` on their own account. All six are refused
  with `409`; clearing an expiry stays available (`55f1281`, `d7cc8fc`).
- An expiry date displayed a day late anywhere east of UTC: the field said the
  20th, the page said the 21st (`8611a1c`).
- The theme bootstrap was blocked by the Content-Security-Policy, so the app
  always started in light mode (`ef6619a`).
- The dashboard grid overflowed a 390 px screen (`ef6619a`).
- Deep links to permission-gated routes bounced to `/users` because the router
  mounted before the session probe resolved (`902c640`).
- `?size=10` rendered 25 rows (`902c640`).
- Status colours only worked in light mode; sorting, account status and tabs were
  unusable without sight or a mouse (`1f5e953`, `f282393`).
- Unsaved-changes prompts are the app's own dialog with wording that matches the
  move, and now cover the creation forms and the group information tab — the places
  where losing the work costs the most (`295a9e8`, `69ecea2`).
- Deleting an absent user answered `500` instead of `404` (`1c44313`).
- `docs/API.md` documented a `uid` field on the group members endpoints where the
  handler reads `memberUid` (`1c44313`).
