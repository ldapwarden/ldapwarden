# Changelog

Versions are date-based (`YYYY.MM.DD.HHMMSS`), cut by tagging — see
`.github/workflows/release.yml`.

## Unreleased

Read **Upgrading** first: this release changes where an account's expiry is
stored, and the bundled Docker Compose stack moves to OpenLDAP 2.6. Neither is
automatic.

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
  elapsed, so those accounts authenticate today. Until the account's next
  successful bind — which makes ppolicy clear the attribute itself — the UI shows
  them as locked. Unlock them from the UI, or pass `-include-past-locks` if the
  dates were meant as expiries.
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

**3. Two API responses changed shape.** `GET /api/admin/config` renames
`capabilities.scheduledAccountExpiry` to `capabilities.scheduledExpirySupported`
and adds `capabilities.policyEnforcement`; `DELETE /api/users/{dn}` now returns
`{message, warnings}`. Both are consumed only by the bundled frontend.

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
- **`GET /api/groups/{dn}/members`**, resolving `memberUid` to names server-side
  (`69ecea2`).
- Password fields that can be revealed, with a generator and a strength meter
  (`c6ccef0`).
- Skeleton loaders, actionable empty states, and list/tab state kept in the URL so
  every view is a shareable link (`67d5819`, `2d63f78`, `902c640`).
- Bulk actions report every failure, not just the first (`182fa6d`).

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

- **Deleting a user left their uid in every group's `memberUid` and in any
  `sudoRole`'s `sudoUser`.** Recreating an account with the same username — routine
  when one is recycled — silently inherited the previous holder's groups and sudo
  grants (`1c44313`).
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
