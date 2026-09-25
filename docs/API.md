# LDAP Warden API Documentation

This document describes the REST API endpoints available in LDAP Warden.

## Base URL

```
http://localhost:8000/api
```

## Authentication

Most endpoints require authentication via session cookie. First, authenticate using the login endpoint to obtain a session.

### Login

```http
POST /api/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123"
}
```

**curl example:**
```bash
curl -X POST http://localhost:8000/api/auth/login \
  -H "Content-Type: application/json" \
  -c cookies.txt \
  -d '{"username": "admin", "password": "admin123"}'
```

**Response:**
```json
{
  "userDn": "uid=admin,ou=People,dc=example,dc=org",
  "userUid": "admin",
  "displayName": "System Administrator",
  "mail": "admin@example.org",
  "roleName": "admin",
  "permissions": ["users:read", "users:write", "users:delete", "groups:read", "groups:write", "groups:delete", "audit:read", "schema:read", "schema:write", "settings:read"]
}
```

### Logout

```http
POST /api/auth/logout
```

**curl example:**
```bash
curl -X POST http://localhost:8000/api/auth/logout \
  -b cookies.txt
```

### Get Current User

```http
GET /api/auth/me
```

**curl example:**
```bash
curl http://localhost:8000/api/auth/me \
  -b cookies.txt
```

### Change Own Password

```http
POST /api/auth/change-password
Content-Type: application/json

{
  "password": "newpassword123"
}
```

**curl example:**
```bash
curl -X POST http://localhost:8000/api/auth/change-password \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{"password": "newpassword123"}'
```

---

## Health

Both probes are unauthenticated and live outside `/api`, so an orchestrator can
reach them without credentials.

### Liveness

```http
GET /healthz
```

`200` with `{"status": "ok"}` for as long as the process is answering HTTP. It
checks nothing else, on purpose: a liveness probe that fails because the directory
is slow gets the container restarted, which fixes nothing and drops everyone's
session. Point a `livenessProbe` and the image's `HEALTHCHECK` here.

This replaced `GET /health`, which no longer exists.

### Readiness

```http
GET /readyz
```

`200` with `{"status": "ready"}` when the app can serve traffic, `503` with
`{"status": "degraded"}` when it cannot. Point a `readinessProbe` or a load
balancer here so requests stop arriving while they would only come back as `500`s.

Each dependency is named, with the error quoted, so an operator does not have to
guess which one is down:

```json
{
  "status": "degraded",
  "checks": {
    "database": "ok",
    "sessions": "dial tcp 127.0.0.1:6379: connect: connection refused",
    "directory": "ok"
  }
}
```

- `database` — PostgreSQL. `auditMutating` refuses every mutation when the audit log
  is unavailable, so an app that cannot reach it is read-only.
- `sessions` — Redis. Sessions live only there, so it cannot authenticate anyone.
- `directory` — LDAP: the configured base DN is read, which proves the bind
  credentials work *and* that the tree is reachable.

Each check is bounded by a three-second timeout.

---

## Users

### List Users

```http
GET /api/users
```

**Required permission:** `users:read`

**Query parameters:**

| Parameter | Description | Default |
|-----------|-------------|---------|
| `search` | Case-insensitive substring match on `uid`, `cn`, `mail`, `displayName` | (none) |
| `status` | `active` or `inactive` — filters on whether the account can currently log in (locked or past its expiration date counts as inactive) | both |
| `sort` | `uid`, `displayName`, `mail`, `employeeType`, `groupsCount` | `uid` |
| `order` | `asc` or `desc` | `asc` |
| `limit` | Page size, capped at 500. `0` or omitted returns every match | (no paging) |
| `offset` | Entries to skip. An offset past the end returns an empty page, not an error | `0` |

Sorting and paging are applied over the whole matching set, so page 1 of a
descending sort holds the last entries overall. The set itself is still bounded
by `LDAP_SEARCH_SIZE_LIMIT`, which is what `truncated` reports.

**Response fields:** this endpoint returns a deliberately narrow projection —
what a list row shows, plus what sorting and the status filter need. It does
**not** include the photo, password state, object classes or the Samba / shadow
attribute blocks; fetch a single user for those.

- `total` — matches for the current `search` and `status`, across all pages.
- `inactiveCount` — locked/expired matches for `search`, whatever `status` is.
- `hasPhoto` — whether the user has a `jpegPhoto`; fetch it from
  `GET /api/users/{dn}/photo`.
- `groupsCount` — number of groups listing the user in `memberUid`.
- `lockout` — what the entry's `pwdAccountLockedTime` means right now, resolved
  against the policy that governs the entry. `accountLocked` only reports that the
  attribute is set, which a lapsed ppolicy lockout also does; `status=active` /
  `status=inactive` filter on `lockout` instead. See *Account Expiry and Locking*.

**curl example:**
```bash
curl "http://localhost:8000/api/users?search=doe&sort=uid&limit=25&offset=0" \
  -b cookies.txt
```

**Response:**
```json
{
  "data": [
    {
      "dn": "uid=admin,ou=People,dc=example,dc=org",
      "uid": "admin",
      "cn": "System Administrator",
      "displayName": "System Administrator",
      "mail": "admin@example.org",
      "employeeType": "staff",
      "hasPhoto": false,
      "groupsCount": 1,
      "accountLocked": false,
      "lockout": { "state": "none", "durationSeconds": 0 }
    }
  ],
  "total": 1,
  "inactiveCount": 0,
  "limit": 25,
  "offset": 0,
  "truncated": false
}
```

### Get User Photo

```http
GET /api/users/{dn}/photo
```

**Required permission:** `users:read`

Returns the user's `jpegPhoto` as `image/jpeg`, or `404` when they have none.
Served as an image rather than base64 inside JSON so lists can point an `<img>`
at it and let the browser fetch and cache only the rows on screen. The response
is marked `Cache-Control: private` — it is only readable with a valid session.

### Get User

```http
GET /api/users/{dn}
```

**Required permission:** `users:read`

> **Note:** The DN must be URL-encoded.

**curl example:**
```bash
# URL-encode the DN
DN="uid=admin,ou=People,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl "http://localhost:8000/api/users/$ENCODED_DN" \
  -b cookies.txt
```

### Create User

```http
POST /api/users
Content-Type: application/json

{
  "uid": "jsmith",
  "givenName": "John",
  "sn": "Smith",
  "displayName": "John Smith",
  "mail": "jsmith@example.org",
  "uidNumber": 1005,
  "gidNumber": 1000,
  "password": "secretpassword123",
  "groups": ["developers"]
}
```

**Required permission:** `users:write`

**Required fields:**
- `uid` - Username
- `givenName` - First name
- `sn` - Last name (surname)
- `uidNumber` - POSIX UID
- `gidNumber` - Primary POSIX GID

**Optional fields:**
- `cn` - Common name (defaults to "givenName sn")
- `displayName` - Display name (defaults to cn)
- `mail` - Email address
- `telephoneNumber` - Phone number
- `title` - Job title
- `departmentNumber` - Department
- `employeeNumber` - Employee ID
- `employeeType` - Employee type (e.g., "Full-Time", "Contractor")
- `initials` - User initials
- `manager` - Manager DN (e.g., "uid=jdoe,ou=People,dc=example,dc=org")
- `homeDirectory` - Home directory (defaults to /home/uid)
- `loginShell` - Login shell (defaults to /bin/bash)
- `gecos` - GECOS field (user information for finger command)
- `password` - Initial password
- `description` - Description
- `objectClasses` - Object classes (defaults to inetOrgPerson, posixAccount, shadowAccount)
- `groups` - Array of group CNs to add the user to after creation

**curl example:**
```bash
curl -X POST http://localhost:8000/api/users \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{
    "uid": "jsmith",
    "givenName": "John",
    "sn": "Smith",
    "displayName": "John Smith",
    "mail": "jsmith@example.org",
    "telephoneNumber": "+1-555-0199",
    "title": "Software Engineer",
    "departmentNumber": "Engineering",
    "uidNumber": 1005,
    "gidNumber": 1000,
    "password": "secretpassword123",
    "groups": ["developers", "engineering"]
  }'
```

**Response:**
```json
{
  "dn": "uid=jsmith,ou=People,dc=example,dc=org",
  "uid": "jsmith",
  "cn": "John Smith",
  "sn": "Smith",
  "givenName": "John",
  "displayName": "John Smith",
  "mail": "jsmith@example.org",
  "telephoneNumber": "+1-555-0199",
  "title": "Software Engineer",
  "departmentNumber": "Engineering",
  "uidNumber": 1005,
  "gidNumber": 1000,
  "homeDirectory": "/home/jsmith",
  "loginShell": "/bin/bash",
  "accountLocked": false,
  "lockout": {
    "state": "lifted",
    "lockedAt": "20260817173435Z",
    "liftsAt": "2026-08-17T17:49:35Z",
    "durationSeconds": 900,
    "policyDn": "cn=default,ou=policies,dc=example,dc=org"
  },
  "objectClasses": ["inetOrgPerson", "posixAccount", "shadowAccount"]
}
```

### Update User

```http
PUT /api/users/{dn}
Content-Type: application/json

{
  "displayName": "John D. Smith",
  "title": "Senior Software Engineer"
}
```

**Required permission:** `users:write`

**curl example:**
```bash
DN="uid=jsmith,ou=People,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl -X PUT "http://localhost:8000/api/users/$ENCODED_DN" \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{
    "displayName": "John D. Smith",
    "title": "Senior Software Engineer"
  }'
```

### Change a user's POSIX numbers

```http
POST /api/users/{dn}/change-posix-ids
```

**Required permission:** `users:write`

```json
{ "uidNumber": 1500, "gidNumber": 1500 }
```

Either field may be omitted to leave that number alone. Its own route rather than
part of the update, because unlike every other attribute this app edits, these two
are the identity the *filesystem* uses and nothing here can follow them there.

A `uidNumber` already held by another account is refused with `409`: two accounts
on one number are the same user as far as the kernel is concerned. A `gidNumber`
matching no group is accepted — the kernel only needs the number — but reported.

The answer carries what the change did **not** do, and that is the point of the
endpoint:

```json
{
  "user": { "uidNumber": 1500 },
  "warnings": [
    "files owned by uid 1200 on every host still belong to that number, not to 1500; they must be reassigned there (for example find / -uid 1200 -exec chown 1500 {} +)"
  ]
}
```

An entry carrying a `sambaSID` is realigned when its RID came from the old uid
number — either equal to it, which is what this app writes by default, or Samba's
`rid = 2*uid + 1000`. Any other RID means the SID follows a scheme this app does
not know, so it is left untouched and reported: rewriting one allocated by a
domain would hand the account a different principal. Either way the outcome is in
`warnings`, because a Windows client holding the old SID sees a different account.

### Delete User

```http
DELETE /api/users/{dn}
```

Removes the entry and the references other entries kept to it: the uid in every
group's `memberUid`, the uid in any `sudoRole`'s `sudoUser`, and `manager`
attributes pointing at the DN. Neither `memberUid` nor `sudoUser` stores a DN the
directory could follow, so leaving them behind would hand the uid's groups and
sudo grants to whoever gets that username next.

Roles reached through `ALL` or a `%group` are left alone — they do not name this
user. A role that named only this user is emptied but kept, and reported in
`warnings`.

```json
{
  "message": "user deleted",
  "warnings": ["sudo role \"deploy\" now has no sudoUser: it applied to \"jsmith\" only"]
}
```

The entry is deleted first and the references are then migrated best-effort, so a
cleanup failure is reported in `warnings` rather than failing the request — the
account is gone either way, and the named leftovers are the operator's to
reconcile. `404` if there is no such entry.

**Required permission:** `users:delete`

**curl example:**
```bash
DN="uid=jsmith,ou=People,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl -X DELETE "http://localhost:8000/api/users/$ENCODED_DN" \
  -b cookies.txt
```

### Lock User

```http
POST /api/users/{dn}/lock
```

**Required permission:** `users:write`

Locks the user account by adding a `!` prefix to the password hash.

**curl example:**
```bash
DN="uid=jsmith,ou=People,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl -X POST "http://localhost:8000/api/users/$ENCODED_DN/lock" \
  -b cookies.txt
```

### Unlock User

```http
POST /api/users/{dn}/unlock
```

**Required permission:** `users:write`

Unlocks the user account by removing the `!` prefix from the password hash.

**curl example:**
```bash
DN="uid=jsmith,ou=People,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl -X POST "http://localhost:8000/api/users/$ENCODED_DN/unlock" \
  -b cookies.txt
```

### Change User Password

```http
POST /api/users/{dn}/password
Content-Type: application/json

{
  "password": "newpassword123"
}
```

**Required permission:** `users:write`

**curl example:**
```bash
DN="uid=jsmith,ou=People,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl -X POST "http://localhost:8000/api/users/$ENCODED_DN/password" \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{"password": "newpassword123"}'
```

### Remove User Password

```http
DELETE /api/users/{dn}/password
```

**Required permission:** `users:write`

Removes the user's password by setting `userPassword` to `!`, preventing LDAP bind authentication. The user account remains active (not locked).

**curl example:**
```bash
DN="uid=jsmith,ou=People,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl -X DELETE "http://localhost:8000/api/users/$ENCODED_DN/password" \
  -b cookies.txt
```

### Get User Groups

```http
GET /api/users/{dn}/groups
```

**Required permission:** `users:read`

**curl example:**
```bash
DN="uid=admin,ou=People,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl "http://localhost:8000/api/users/$ENCODED_DN/groups" \
  -b cookies.txt
```

### Get User Sudo Roles

```http
GET /api/users/{dn}/sudo-roles
```

**Required permission:** `users:read`

**curl example:**
```bash
DN="uid=admin,ou=People,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl "http://localhost:8000/api/users/$ENCODED_DN/sudo-roles" \
  -b cookies.txt
```

### SSH Keys

#### Set All SSH Keys

```http
PUT /api/users/{dn}/ssh-keys
Content-Type: application/json

{
  "keys": [
    "ssh-rsa AAAAB3NzaC1yc2EAAAA... user@host",
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5... user@host2"
  ]
}
```

**Required permission:** `users:write`

#### Add SSH Key

```http
POST /api/users/{dn}/ssh-keys
Content-Type: application/json

{
  "key": "ssh-rsa AAAAB3NzaC1yc2EAAAA... user@host"
}
```

**Required permission:** `users:write`

#### Remove SSH Key

```http
DELETE /api/users/{dn}/ssh-keys
Content-Type: application/json

{
  "key": "ssh-rsa AAAAB3NzaC1yc2EAAAA... user@host"
}
```

**Required permission:** `users:write`

### Send Password Reset Email

```http
POST /api/users/{dn}/send-password-reset
```

**Required permission:** `users:write`

Sends a password reset email to the user (requires mail configuration).

---

## Groups

### List Groups

```http
GET /api/groups
```

**Required permission:** `groups:read`

**Query parameters:**

| Parameter | Description | Default |
|-----------|-------------|---------|
| `search` | Case-insensitive substring match on `cn`, `description` | (none) |
| `sort` | `cn`, `description`, `gidNumber`, `members` | `cn` |
| `order` | `asc` or `desc` | `asc` |
| `limit` | Page size, capped at 500. `0` or omitted returns every match | (no paging) |
| `offset` | Entries to skip | `0` |

**Response fields:** like the user listing, this is a narrow projection. It
reports `membersCount` rather than the `memberUid` array — fetch a single group
for its members.

**curl example:**
```bash
curl "http://localhost:8000/api/groups?sort=members&order=desc&limit=25" \
  -b cookies.txt
```

**Response:**
```json
{
  "data": [
    {
      "dn": "cn=admins,ou=Groups,dc=example,dc=org",
      "cn": "admins",
      "gidNumber": 1000,
      "description": "System administrators",
      "membersCount": 2
    }
  ],
  "total": 1,
  "limit": 25,
  "offset": 0,
  "truncated": false
}
```

### Get Group

```http
GET /api/groups/{dn}
```

**Required permission:** `groups:read`

**curl example:**
```bash
DN="cn=admins,ou=Groups,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl "http://localhost:8000/api/groups/$ENCODED_DN" \
  -b cookies.txt
```

### Create Group

```http
POST /api/groups
Content-Type: application/json

{
  "cn": "developers",
  "gidNumber": 1004,
  "description": "Development team",
  "memberUid": ["jsmith", "jdoe"]
}
```

**Required permission:** `groups:write`

**Required fields:**
- `cn` - Group name

**Optional fields:**
- `gidNumber` - POSIX GID. Omitted or `0`, the next free number is allocated
  when the entry is written; a number that is given is refused with `409` if
  another group holds it
- `description` - Group description
- `memberUid` - Array of member usernames

**curl example:**
```bash
curl -X POST http://localhost:8000/api/groups \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{
    "cn": "developers",
    "gidNumber": 1004,
    "description": "Development team",
    "memberUid": ["jsmith", "jdoe"]
  }'
```

### Update Group

```http
PUT /api/groups/{dn}
Content-Type: application/json

{
  "description": "Updated description"
}
```

**Required permission:** `groups:write`

**curl example:**
```bash
DN="cn=developers,ou=Groups,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl -X PUT "http://localhost:8000/api/groups/$ENCODED_DN" \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{"description": "Software Development team"}'
```

### Rename a group

```http
POST /api/groups/{dn}/change-cn
```

**Required permission:** `groups:write`

```json
{ "newCn": "platform" }
```

The `cn` is the RDN, so this renames the entry and its DN changes. Members do
not move: a POSIX group is joined by `memberUid` (a username) and by `gidNumber`
for primary members, and neither follows the name.

Sudo does. A rule grants to a group as `%cn`, a bare string nothing in the
directory tracks, so every such rule is rewritten here. `sudoRunAsGroup` holds a
group name without the prefix, which is indistinguishable from a group local to
the hosts — those are reported in `warnings` rather than rewritten.

Refused with `409` for the group named by `LDAPWARDEN_ADMIN_GROUP`: administrator
access is decided by comparing group names against that setting, so renaming it
would remove admin from everyone, with no way to undo it from the app.

Answers with the renamed group and the references it could not migrate:

```json
{ "group": { "dn": "cn=platform,ou=Groups,dc=example,dc=org", "cn": "platform" },
  "warnings": [] }
```

### Delete Group

```http
DELETE /api/groups/{dn}
```

**Required permission:** `groups:delete`

**curl example:**
```bash
DN="cn=developers,ou=Groups,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl -X DELETE "http://localhost:8000/api/groups/$ENCODED_DN" \
  -b cookies.txt
```

Answers with `warnings` naming what is left pointing at nothing: sudo roles that
still grant to `%cn` or run as the group, and accounts whose primary group is
still its `gidNumber`. Collected before the delete; reported, not rewritten.

Refuses the group named by `LDAPWARDEN_ADMIN_GROUP` with `409 Conflict`: admin
is decided by that group's name, so deleting it would strip admin from everyone.

### List Group Members

```http
GET /api/groups/{dn}/members
```

Resolves the group's `memberUid` values to user entries, so a client does not
have to fetch the user collection to display names.

```json
{
  "data": [
    {
      "uid": "jdoe",
      "dn": "uid=jdoe,ou=People,dc=example,dc=org",
      "cn": "John Doe",
      "displayName": "John Doe",
      "hasPhoto": false
    },
    { "uid": "ghost", "hasPhoto": false }
  ]
}
```

`uid` is always present — it is the value the group entry stores. The other
fields are omitted when no user entry carries that uid, which is how a dangling
membership is reported rather than dropped. Sorted by uid.

**Required permission:** `groups:read`

**curl example:**
```bash
DN="cn=developers,ou=Groups,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl "http://localhost:8000/api/groups/$ENCODED_DN/members" -b cookies.txt
```

### Add Member to Group

```http
POST /api/groups/{dn}/members
Content-Type: application/json

{
  "memberUid": "jsmith"
}
```

**Required permission:** `groups:write`

**curl example:**
```bash
DN="cn=developers,ou=Groups,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl -X POST "http://localhost:8000/api/groups/$ENCODED_DN/members" \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{"memberUid": "jsmith"}'
```

### Remove Member from Group

```http
DELETE /api/groups/{dn}/members
Content-Type: application/json

{
  "memberUid": "jsmith"
}
```

**Required permission:** `groups:write`

**curl example:**
```bash
DN="cn=developers,ou=Groups,dc=example,dc=org"
ENCODED_DN=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$DN', safe=''))")

curl -X DELETE "http://localhost:8000/api/groups/$ENCODED_DN/members" \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{"memberUid": "jsmith"}'
```

### Get Group Sudo Roles

```http
GET /api/groups/{dn}/sudo-roles
```

**Required permission:** `groups:read`

---

## Sudo Roles

Sudo roles are governed by the **`users:*`** permissions, not by a permission of
their own: granting sudo is granting a user privileges, so the right to do it
follows the right to manage that user. The one exception is a role's *group*
grants (`%group` entries in `sudoUser`), which take `groups:write` because they
change what a group confers. This is deliberate — an operator who can edit sudo
rules but not the accounts they apply to could grant root to an account they
cannot see.

### List Sudo Roles

```http
GET /api/sudo-roles
```

**Required permission:** `users:read`

**Query parameters:**

| Parameter | Description | Default |
|-----------|-------------|---------|
| `search` | Case-insensitive substring match on `cn`, `description`, `sudoUser`, `sudoCommand` | (none) |
| `sort` | `cn` or `sudoOrder` | `cn` |
| `order` | `asc` or `desc` | `asc` |
| `limit` | Page size, capped at 500. `0` or omitted returns every match | (no paging) |
| `offset` | Entries to skip | `0` |

Entries keep their full attribute set — unlike users and groups there is nothing
heavy to project away — so the value here is a correct `total` and a page the
client did not have to compute.

**curl example:**
```bash
curl http://localhost:8000/api/sudo-roles \
  -b cookies.txt
```

**Response:**
```json
{
  "data": [
    {
      "dn": "cn=admin-all,ou=sudoers,dc=example,dc=org",
      "cn": "admin-all",
      "description": "Full admin access",
      "sudoUser": ["admin", "%admins"],
      "sudoHost": ["ALL"],
      "sudoCommand": ["ALL"],
      "sudoRunAs": ["ALL"],
      "sudoOption": ["!authenticate"]
    }
  ],
  "total": 1
}
```

### Get Sudo Role

```http
GET /api/sudo-roles/{dn}
```

**Required permission:** `users:read`

### Create Sudo Role

```http
POST /api/sudo-roles
Content-Type: application/json

{
  "cn": "docker-users",
  "description": "Allow docker commands",
  "sudoUser": ["%developers"],
  "sudoHost": ["ALL"],
  "sudoCommand": ["/usr/bin/docker"],
  "sudoOption": ["!authenticate"]
}
```

**Required permission:** `users:write`

**Required fields:**
- `cn` - Role name
- `sudoUser` - Users or groups (prefix groups with `%`)
- `sudoHost` - Hosts where rule applies
- `sudoCommand` - Allowed commands

**Optional fields:**
- `description` - Role description
- `sudoRunAs` - Run as user
- `sudoRunAsUser` - Run as specific user
- `sudoRunAsGroup` - Run as group
- `sudoOption` - Sudo options
- `sudoOrder` - Evaluation order
- `sudoNotBefore` - Rule not valid before (timestamp)
- `sudoNotAfter` - Rule not valid after (timestamp)

**curl example:**
```bash
curl -X POST http://localhost:8000/api/sudo-roles \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{
    "cn": "docker-users",
    "description": "Allow docker commands",
    "sudoUser": ["%developers"],
    "sudoHost": ["ALL"],
    "sudoCommand": ["/usr/bin/docker", "/usr/bin/docker-compose"],
    "sudoOption": ["!authenticate"]
  }'
```

### Update Sudo Role

```http
PUT /api/sudo-roles/{dn}
Content-Type: application/json

{
  "sudoCommand": ["/usr/bin/docker", "/usr/bin/docker-compose", "/usr/bin/podman"]
}
```

**Required permission:** `users:write`

### Delete Sudo Role

```http
DELETE /api/sudo-roles/{dn}
```

**Required permission:** `users:delete`

### Add User to Sudo Role

```http
POST /api/sudo-roles/{dn}/users
Content-Type: application/json

{
  "uid": "jsmith"
}
```

**Required permission:** `users:write`

### Remove User from Sudo Role

```http
DELETE /api/sudo-roles/{dn}/users
Content-Type: application/json

{
  "uid": "jsmith"
}
```

**Required permission:** `users:write`

### Add Group to Sudo Role

```http
POST /api/sudo-roles/{dn}/groups
Content-Type: application/json

{
  "cn": "developers"
}
```

**Required permission:** `groups:write`

The group will be added with the `%` prefix (e.g., `%developers`).

### Remove Group from Sudo Role

```http
DELETE /api/sudo-roles/{dn}/groups
Content-Type: application/json

{
  "cn": "developers"
}
```

**Required permission:** `groups:write`

---

## Netgroups

`nisNetgroup` entries, from `nis.schema`. Governed by the **`groups:*`**
permissions — a netgroup is a collection, and managing one is the same kind of
act as managing a group. Naming a netgroup *from a sudo role* stays under
`users:*`, like the rest of sudo.

Every route answers **501 Not Implemented** when the directory does not declare
`nisNetgroup`. An empty list would say "there are none", which is a different
answer from "this server cannot hold any".

### The triple, and its one trap

A netgroup stores members as `nisNetgroupTriple`, spelled `(host,user,domain)`.
The API exposes it parsed:

```json
{ "host": "web01", "user": "jdoe", "domain": "" }
```

**An empty slot means *any*, not *none*.** `(,jdoe,)` is jdoe on every host;
`(web01,,)` is every user on web01. The NIS marker `-` is the one that means
"matches nothing". A client that renders an empty slot as a blank, or reads an
empty host list as "no hosts", reports the opposite of the truth — see
`openHost` / `openUser` on the expansion below.

`nisNetgroupTriple` carries no matching rules, so it cannot be filtered
server-side; `?search=` is applied by the server in Go, over the name,
description, triples and nested names.

### List Netgroups

```http
GET /api/netgroups
```

**Required permission:** `groups:read`

Accepts `search`, `sort` (`cn`), `order`, `limit`, `offset`.

### Get Netgroup

```http
GET /api/netgroups/{dn}
```

**Required permission:** `groups:read`

`malformed` holds stored values the server could not parse. They are never
dropped: an update carries them through untouched, and a user rename reports
them as references it could not migrate.

### Expand Netgroup

```http
GET /api/netgroups/{dn}/expand
```

**Required permission:** `groups:read`

Resolves `memberNisNetgroup` and answers with what the netgroup contributes.

```json
{
  "root": "ops-oncall",
  "users": [{ "name": "jdoe", "via": "ops" }],
  "hosts": [],
  "openHost": ["ops", "ops-oncall"],
  "cycles": [],
  "dangling": []
}
```

- `via` names the nested netgroup a member came from; absent when named directly.
- `openHost` / `openUser` name the netgroups leaving that slot open. Non-empty
  means **every** host / user, which the empty `hosts` list above cannot say on
  its own.
- `cycles` and `dangling` are reported, not errors: `innetgr()` bounds a loop
  rather than failing, and a nested name pointing at nothing simply contributes
  nothing.

A separate endpoint rather than a field on the entry, because expanding reads
the whole collection.

### Sudo roles naming a netgroup

```http
GET /api/netgroups/{dn}/sudo-roles
```

**Required permission:** `groups:read`

The reverse lookup. A sudo rule names a netgroup as `+name`, in `sudoUser` or in
`sudoHost`, and nothing in the directory ties that string back to the entry — so
this searches the sudo tree for both spellings. Answers in the same shape as
`/users/{dn}/sudo-roles` and `/groups/{dn}/sudo-roles`.

Which side a rule names it on is left in the payload rather than summarised:
`sudoUser` reads only the netgroup's user slots and `sudoHost` only its host
slots, so the side is what decides whether the reference does anything at all.

### Create Netgroup

```http
POST /api/netgroups
```

**Required permission:** `groups:write`

```json
{
  "cn": "ops",
  "description": "Operators, on any host",
  "triples": [{ "host": "", "user": "jdoe", "domain": "" }],
  "members": ["ops-junior"]
}
```

A triple with all three fields empty is refused: it matches every user on every
host. Nothing else is — a triple with the host and user the wrong way round is
indistinguishable from a deliberate one.

### Import Netgroups

```http
POST /api/netgroups/import
```

**Required permission:** `groups:write`

Bulk create, up to 1000 rows. Triples travel as their stored text rather than as
parsed slots, because that is what a CSV cell can carry and what every other tool
produces:

```json
{
  "rows": [
    {
      "cn": "ops-oncall",
      "description": "On-call operators",
      "triples": ["(,jdoe,)", "(-,asmith,)"],
      "members": ["ops"]
    }
  ]
}
```

Each value is read with the same parser that reads the attribute back off the
directory, so an import is held to the same rules as an edit — including the one
that matters most here: an empty slot means *any* and `-` means *nothing*, and
both survive the round trip unchanged.

Rows are independent. A row that fails is reported and skipped, the rest are
created, and the request still answers `200`:

```json
{
  "created": 2,
  "failed": 1,
  "results": [
    { "index": 0, "key": "ops-oncall", "status": "created" },
    { "index": 1, "key": "ops", "status": "created" },
    { "index": 2, "key": "bad", "status": "error",
      "error": "netgroup triple \"(web01,jdoe)\" has 2 comma-separated fields, want 3 (host,user,domain)" }
  ]
}
```

A `members` entry naming a netgroup that does not exist yet is accepted: a file
may create it in a later row, and the directory does not resolve
`memberNisNetgroup` either. `POST /api/users/import` and
`POST /api/groups/import` answer in the same shape.

### Update Netgroup

```http
PUT /api/netgroups/{dn}
```

**Required permission:** `groups:write`

The `cn` cannot change: sudo rules and other netgroups reference it by name, and
nothing in the directory tracks those references.

### Delete Netgroup

```http
DELETE /api/netgroups/{dn}
```

**Required permission:** `groups:delete`

Answers with `warnings` naming what is left pointing at nothing — sudo roles
that still say `+name`, and netgroups that still nest it. Collected before the
delete, while the name still resolves. Reported rather than refused: removing a
netgroup is legitimate.

### Netgroups of a user

```http
GET /api/users/{dn}/netgroups
```

**Required permission:** `users:read`

```json
{ "data": [{ "cn": "ops", "direct": true }, { "cn": "ops-oncall", "direct": false }] }
```

`direct: false` means the user is reached only through a nesting parent — a sudo
rule naming it still grants to them, but it is not the netgroup to edit to take
that away.

### Using a netgroup in a sudo rule

Sudo names a netgroup `+name`, in `sudoUser` or in `sudoHost`. The two are the
same string and mean different things: `+name` in `sudoUser` reads only the
netgroup's user slots, in `sudoHost` only its host slots. **A netgroup of users
placed in `sudoHost` matches nothing at all**, with no error from sudo or from
the directory.

`GET /api/users/{dn}/sudo-roles` returns each role with a `via` array holding the
`sudoUser` values that matched — the uid itself, `ALL`, `%group` or `+netgroup` —
so a caller can tell a direct grant from an inherited one.

Writing the grant is necessary and not sufficient: sudo resolves netgroups on the
client, through NSS, `netgroup_base` in `ldap.conf`, or SSSD. None of that is
visible from the directory, so LDAP Warden cannot confirm a `+netgroup` rule has
any effect. `getent netgroup <name>` on a host is what settles it.

## Password Policies

Requires the ppolicy overlay on the directory (see the schema prerequisites in
the README). Create / update / delete mirror the other resources; the fields are
the `pwdPolicy` attributes (`pwdMaxAge`, `pwdMinLength`, `pwdInHistory`,
`pwdLockout`, …).

### List Password Policies

```http
GET /api/password-policies
```

**Required permission:** `settings:read`

**Query parameters:**

| Parameter | Description | Default |
|-----------|-------------|---------|
| `search` | Case-insensitive substring match on `cn`, `description` | (none) |
| `sort` | `cn`, `pwdMaxAge`, `pwdMinLength` | `cn` |
| `order` | `asc` or `desc` | `asc` |
| `limit` | Page size, capped at 500. `0` or omitted returns every match | (no paging) |
| `offset` | Entries to skip | `0` |

**curl example:**
```bash
curl "http://localhost:8000/api/password-policies?sort=pwdMinLength&order=desc" \
  -b cookies.txt
```

**Response:**
```json
{
  "data": [
    {
      "dn": "cn=default,ou=policies,dc=example,dc=org",
      "cn": "default",
      "description": "Default password policy",
      "pwdMaxAge": 7776000,
      "pwdMinLength": 12,
      "pwdInHistory": 5,
      "pwdLockout": true
    }
  ],
  "total": 1,
  "limit": 0,
  "offset": 0,
  "truncated": false
}
```

### Delete a Password Policy

```http
DELETE /api/password-policies/{dn}
```

**Required permission:** `settings:write`

Answers with `warnings` naming what is left pointing at nothing: accounts whose
`pwdPolicySubentry` still names the policy — a DN the directory does not follow,
so they stop getting it in silence — and `LDAP_DEFAULT_POLICY_DN` if it names it.
Collected before the delete; reported, not rewritten.

### Assigning a Policy to a User

A user's policy is the `pwdPolicySubentry` attribute, set through
`PUT /api/users/{dn}` like any other attribute.

---

## Schema

### Get LDAP Schema

```http
GET /api/schema
```

**Required permission:** `schema:read`

Returns the cached LDAP schema including object classes and attribute types.

**curl example:**
```bash
curl http://localhost:8000/api/schema \
  -b cookies.txt
```

### Refresh Schema Cache

```http
POST /api/schema/refresh
```

**Required permission:** `schema:write`

Forces a refresh of the cached LDAP schema.

---

## Overview

### Get Overview

```http
GET /api/overview
```

**Required permission:** `users:read`

The directory at a glance, as shown on the dashboard. Each figure is a count the
directory performs — one search per state, with no attributes requested — rather
than a length measured here after fetching every entry. On a 706-entry directory
that moved 55 kB where the fetch-and-tally version moved 82 kB while still being
wrong — though the saving narrows as the directory grows, since only the old
version was bounded. The filters name `pwdAccountLockedTime` and `pwdEndTime`,
which are unindexed by default; index them if the dashboard gets slow.

- `users.total` / `locked` / `expired` / `active` — an account is locked when a
  lock is *still refusing binds* (see *Account Expiry and Locking*: a ppolicy
  lockout past its `pwdLockoutDuration` is not counted, and needs
  `LDAP_DEFAULT_POLICY_DN` to be recognised for entries with no
  `pwdPolicySubentry`), and expired when its `pwdEndTime` has passed.
  The two are independent, so **`locked` + `expired` can exceed
  `total` − `active`**: an account that is both appears in each and is excluded
  from `active` once. `active` is the complement of the union, so
  `total` − `active` is the number of accounts that cannot log in.
- `users.expiring` — accounts expiring within 30 days, soonest first, capped at
  5. The window matches the expiration scheduler's first reminder, so the
  dashboard warns about the accounts admins are already emailed about. Empty on a
  directory without `pwdEndTime`.
- `groups` / `sudoRoles` / `policies` — collection sizes. A module whose schema
  is absent from the directory reports 0 rather than failing the request.
- `truncated` — the **server** stopped a count early, so the figures are lower
  bounds. This is not `LDAP_SEARCH_SIZE_LIMIT`, which bounds list payloads and is
  deliberately not applied to counts; it means a size limit applies to the bind
  identity (`olcSizeLimit` on OpenLDAP). A rootdn bind is normally unlimited, so
  this is usually `false`.

**Response:**
```json
{
  "users": {
    "total": 42,
    "active": 40,
    "locked": 1,
    "expired": 1,
    "expiring": [
      {
        "dn": "uid=jdoe,ou=People,dc=example,dc=org",
        "uid": "jdoe",
        "displayName": "John Doe",
        "expiresAt": "2026-02-01T23:59:59Z"
      }
    ]
  },
  "groups": 7,
  "sudoRoles": 3,
  "policies": 2,
  "truncated": false
}
```

---

## Audit Logs

### List Audit Logs

```http
GET /api/audit-logs?limit=50&offset=0
```

**Required permission:** `audit:read`

**Query parameters:**
- `limit` - Number of results (default: 50, max: 100)
- `offset` - Pagination offset
- `actorDn` - Filter by actor DN (exact)
- `actorUid` - Filter by actor uid (exact)
- `resourceType` - Filter by resource type (user, group, sudorole, pwdpolicy, schema)
- `action` - Filter by action
- `search` - Case-insensitive substring match over the actor uid **and** the
  resource DN: "every entry mentioning jdoe", whether as actor or as target
- `from`, `to` - Inclusive day bounds (`YYYY-MM-DD`). `to` covers the whole day

**curl example:**
```bash
curl "http://localhost:8000/api/audit-logs?limit=10&from=2026-01-01&search=jdoe" \
  -b cookies.txt
```

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "actorDn": "uid=admin,ou=People,dc=example,dc=org",
      "actorUid": "admin",
      "action": "user.create",
      "resourceType": "user",
      "resourceDn": "uid=jsmith,ou=People,dc=example,dc=org",
      "details": {"uid": "jsmith"},
      "createdAt": "2025-01-02T10:30:00Z"
    }
  ],
  "total": 1
}
```

**Action types:**
- `login`, `login.failed`, `logout`
- `user.create`, `user.update`, `user.delete`, `user.rename`, `user.lock`, `user.unlock`
- `user.lockout` — the *directory* locked the account after too many failed binds,
  found by the lockout scan (actor `system`); `user.lock` is an administrator doing
  it through the API
- `group.create`, `group.update`, `group.delete`
- `group.member.add`, `group.member.remove`
- `sudorole.create`, `sudorole.update`, `sudorole.delete`
- `sudorole.user.add`, `sudorole.user.remove`
- `sudorole.group.add`, `sudorole.group.remove`
- `pwdpolicy.create`, `pwdpolicy.update`, `pwdpolicy.delete`
- `schema.refresh`
- `notification.account_expiration`, `notification.password_expiration`

For update actions, `details.changes` holds the structured diff: one entry per
attribute with `field`, `old` and `new`. A `masked: true` entry (passwords)
records that the field changed without either value.

For `user.lockout`, `details` carries the evidence the directory does not keep:
`failedBinds` (the `pwdFailureTime` values as RFC 3339 instants), `attemptsCount`,
`attemptSpread`, `lockedAt`, `liftsAt`, `lockState` and `policyDn`. ppolicy prunes
`pwdFailureTime` to `pwdFailureCountInterval` and deletes it, with the lock, at the
account's next sign-in — so these rows are the lasting record of the attempts, and
`attemptsCount` is a lower bound whenever older attempts had already aged out.

### List Audit Actors

```http
GET /api/audit-logs/actors
```

**Required permission:** `audit:read`

Returns the actors present in the log, most recently active first (capped at
200), so a UI can offer them as a filter instead of asking for an exact uid.

```json
{ "data": ["admin", "jdoe"] }
```

### Export Audit Logs

```http
GET /api/audit-logs/export
```

**Required permission:** `audit:read`

Returns `text/csv` (as an attachment) for the same filters as the listing —
`actorDn`, `actorUid`, `resourceType`, `action`, `search`, `from`, `to`. Entries
come oldest first, the order an audit trail is read in, and are capped at 10 000
rows per request.

Columns: `timestamp` (RFC 3339, UTC), `actor_uid`, `actor_dn`, `action`,
`resource_type`, `resource_dn`, `changes` (the diff as `field: old → new` lines),
`details` (whatever else the action recorded, as JSON), `ip_address`,
`user_agent`.

**curl example:**
```bash
curl "http://localhost:8000/api/audit-logs/export?from=2026-01-01&to=2026-03-31" \
  -b cookies.txt -o audit-q1.csv
```

---

## Next IDs

### Get Next Available IDs

```http
GET /api/next-ids
```

**Required permission:** `users:write`

Returns the next available UID and GID numbers.

**curl example:**
```bash
curl http://localhost:8000/api/next-ids \
  -b cookies.txt
```

**Response:**
```json
{
  "nextUid": 1006,
  "nextGid": 1005
}
```

---

## Admin

### Get Configuration

```http
GET /api/admin/config
```

**Required permission:** `settings:read`

Returns the current server configuration (passwords masked).

**curl example:**
```bash
curl http://localhost:8000/api/admin/config \
  -b cookies.txt
```

---

## Password Reset (Public)

These endpoints do not require authentication.

### Get Password Reset Info

```http
GET /api/password-reset/{token}
```

Returns information about the password reset token (user display name, organization).

### Confirm Password Reset

```http
POST /api/password-reset/{token}
Content-Type: application/json

{
  "password": "newpassword123"
}
```

Resets the user's password using the token.

---

## Error Responses

All errors return a JSON object with an `error` field:

```json
{
  "error": "user not found"
}
```

**Common HTTP status codes:**
- `400 Bad Request` - Invalid request body or parameters
- `401 Unauthorized` - Not authenticated
- `403 Forbidden` - Insufficient permissions, or a cross-origin mutation (see
  the CORS notes in the README)
- `404 Not Found` - Resource not found
- `409 Conflict` - The request is permitted in principle but conflicts with the
  caller's own state — see *Protecting the signed-in account* below
- `422 Unprocessable Entity` - The directory cannot do what was asked, e.g.
  setting an account expiry on OpenLDAP < 2.5, which has no `pwdEndTime`
- `500 Internal Server Error` - Server error

### Account Expiry and Locking

`POST /api/users/{dn}/lock` sets `pwdAccountLockedTime` to OpenLDAP's
permanent-lock value, so the directory refuses the account until
`POST /api/users/{dn}/unlock` removes it. Locking never touches the expiry.

`PUT /api/users/{dn}/expiration` sets `pwdEndTime` — the attribute slapd enforces
as a validity limit — from a `YYYY-MM-DD` date (a bare day becomes its last
second, so the named day is still usable). An empty `expirationDate` clears it.

```json
{ "expirationDate": "2026-12-31" }
```

The attribute exists only on OpenLDAP 2.5 and later; on an older directory this
endpoint returns `422` rather than writing something the server ignores. Clearing
is always accepted, so an entry carrying a value from a newer server stays fixable.

`GET /api/admin/config` reports two different things, and the difference matters:

```json
{
  "capabilities": {
    "scheduledExpirySupported": true,
    "policyEnforcement": "unknown"
  }
}
```

`scheduledExpirySupported` says `pwdEndTime` can be written. `policyEnforcement`
says whether writing it will have any effect — ppolicy enforces nothing unless a
policy applies to the entry, and the attributes stay writable regardless. With the
overlay loaded but no `olcPPolicyDefault` and no `pwdPolicySubentry`, locking an
account and expiring it both succeed, both do nothing, and the account keeps
authenticating.

| `policyEnforcement` | Meaning |
|---|---|
| `enforced` | A policy was seen to apply. |
| `not-enforced` | Proven: either `cn=config` was readable and carries no default policy, or a self-test locked an account and it authenticated anyway. |
| `unknown` | The normal answer. The bind identity cannot read `cn=config`, and nothing else distinguishes the two states. Not a problem in itself. |

`unknown` is honest rather than lazy. With an ordinary bind identity, `cn=config`
answers "no such object", the monitor backend is usually absent, the server does
not synthesise an effective `pwdPolicySubentry`, the ppolicy control is advertised
by the module rather than by a policy, and `pwdChangedTime` is stamped either way.
`pwdFailureTime` does differ — but it tracks `pwdMaxFailure`, so a policy with
`pwdLockout: FALSE` looks exactly like no policy while still enforcing
`pwdEndTime`. Concluding from it would warn about a working directory.

### Verify Policy Enforcement

```http
POST /api/admin/policy-selftest
```

**Required permission:** `settings:write`

The only definitive answer, and the reason it is an explicit action: it **writes to
the directory**. It creates `uid=ldapwarden-policy-selftest` in the user OU, gives
it a random password, checks it can authenticate, locks it, checks whether the
directory now refuses it, and removes the entry. `409` if that DN is already taken
— an entry the self-test did not create is never touched.

```json
{
  "result": "enforced",
  "probeDn": "uid=ldapwarden-policy-selftest,ou=People,dc=example,dc=org",
  "cleanedUp": true,
  "steps": [
    "created uid=ldapwarden-policy-selftest,ou=People,dc=example,dc=org",
    "set a password via PasswordModify",
    "bind succeeded before locking (control)",
    "wrote pwdAccountLockedTime = 000001010000Z",
    "bind refused after locking — the directory enforces the policy",
    "removed uid=ldapwarden-policy-selftest,ou=People,dc=example,dc=org"
  ]
}
```

`steps` is the evidence, so the verdict can be checked rather than trusted.
`cleanedUp: false` is the one outcome needing action: the probe entry is still
there and must be removed by hand.

The verdict covers **newly created entries in the user OU**. A policy attached
per-entry through `pwdPolicySubentry` applies to those entries and says nothing
about this one. The result is remembered in memory and surfaces as
`policyEnforcement` until the server restarts — re-run it after changing the
policy configuration.

A user's `accountLocked` (any `pwdAccountLockedTime`) and `pwdEndTime` are
independent: an account can be locked, expired, both, or neither. `shadowExpire`
is not consulted — slapd ignores it, so it cannot describe whether the account
still works.

#### Reading a lock: the `lockout` object

`accountLocked` answers "is the attribute there", which is not the same question
as "is this account locked". ppolicy has no timer: it re-reads
`pwdAccountLockedTime` when the entry is next bound to, and deletes it there once
`pwdLockoutDuration` has elapsed. A lockout posted after a run of failed binds
therefore sits in the entry long after it stopped refusing anything — a night, a
weekend, however long until someone tries to log in.

So every user payload carries `lockout`, resolved server-side:

| `state` | Meaning |
|---------|---------|
| `none` | No `pwdAccountLockedTime`. |
| `manual` | The permanent-lock value `000001010000Z`, written by `POST /lock`. ppolicy never lifts it. |
| `active` | A timestamped lockout still in force. `liftsAt` says when it ends; a `durationSeconds` of 0 means it never does. |
| `lifted` | A timestamped lockout past its duration. The account authenticates; the attribute is stale and the directory drops it at the next bind — or `POST /unlock` clears it now. |
| `unknown` | A timestamped lockout whose policy could not be read, so neither answer can be given. Treated as locked. |

`policyDn` is the policy the duration came from: the entry's own
`pwdPolicySubentry`, or `LDAP_DEFAULT_POLICY_DN`. That variable exists because
`olcPPolicyDefault` lives in `cn=config`, which an ordinary bind identity cannot
read — leave it unset and every lockout on an entry without its own policy
resolves to `unknown`.

### Scheduled Tasks

```http
GET  /api/admin/scheduled-tasks/config
GET  /api/admin/scheduled-tasks/runs?taskName=lockouts_scan
GET  /api/admin/scheduled-tasks/health
POST /api/admin/scheduled-tasks/users-expiration/trigger
POST /api/admin/scheduled-tasks/passwords-expiration/trigger
POST /api/admin/scheduled-tasks/lockout-scan/trigger
```

**Required permission:** `settings:read` for the first three, `settings:write` to
trigger. A trigger answers `409` when that task is already running.

`health` has one entry per task with a schedule configured (a disabled task is
left out): its `schedule`, its most recent run (`lastRun`, manual or scheduled,
`null` if it never ran), `nextRun`, and `overdue`. The run history cannot show a
run that never happened, so `overdue` compares the schedule against it: it is set
when the fire time following the last *scheduled* run — or following the
server's start, if there is none — is more than 10 minutes past, and
`expectedAt` names that missed fire time. A manual run does not clear it: it
proves the task works, not that the schedule fires. A fire time that fell while
the server was down counts as missed.

`lockout-scan` reports the accounts the directory locked by itself, which nothing
else in the app can see happening: ppolicy writes `pwdAccountLockedTime` during a
failed bind and deletes it, along with `pwdFailureTime`, at the first bind after
`pwdLockoutDuration` elapses. So a lockout is visible for that duration and no
longer — which makes the schedule
(`LDAPWARDEN_SCHEDULED_TASKS_LOCKOUT_SCAN`, default `*/5 * * * *`) a correctness
setting: run it less often than the shortest `pwdLockoutDuration` and lockouts pass
between two runs. The run reports that mismatch in `errors` rather than going quiet.

Each new lockout produces one `user.lockout` audit entry carrying the failed-bind
timestamps, and one digest email per run to `LDAPWARDEN_AUDIT_NOTIFY_EMAILS` (or the
admin group when unset). Locks an administrator applied through
`POST /api/users/{dn}/lock` are excluded: they are already audited and notified
where they happen.

`usersChecked` in the response counts the locked accounts examined, not the whole
directory — 0 is the healthy answer. `errors` is also where a run says that the mail
server was unreachable; the audit entries are written first and unconditionally, and
an unsent digest is retried by the next run rather than dropped.

### Protecting the signed-in account

An administrator holds `users:write` and `users:delete` over every entry,
including their own, so the following are refused with `409` when the target DN is
the caller's own account. Each one would otherwise cut the caller off from the
application — and if they are the only administrator, the way back is
`ldapmodify` against the directory.

| Endpoint | Refused on self |
|----------|-----------------|
| `POST /api/users/{dn}/lock` | always |
| `DELETE /api/users/{dn}` | always |
| `DELETE /api/users/{dn}/password` | always |
| `POST /api/users/{dn}/change-uid` | always — renaming changes the entry's identity, invalidating the caller's session |
| `PUT /api/users/{dn}/expiration` | when setting a date; clearing one is allowed |
| `PUT /api/users/{dn}/shadow` | when `shadowExpire` names an actual expiry; other shadow fields, and clearing, are allowed |

Unlocking your own account, and changing your own password
(`POST /api/auth/change-password`), are always available.

---

## Helper: URL Encoding DNs

Since LDAP DNs contain special characters, they must be URL-encoded when used in paths.

**Bash function:**
```bash
urlencode() {
  python3 -c "import urllib.parse; print(urllib.parse.quote('$1', safe=''))"
}

# Usage
DN="uid=admin,ou=People,dc=example,dc=org"
curl "http://localhost:8000/api/users/$(urlencode "$DN")" -b cookies.txt
```

**JavaScript:**
```javascript
const dn = "uid=admin,ou=People,dc=example,dc=org";
const encodedDn = encodeURIComponent(dn);
fetch(`/api/users/${encodedDn}`);
```
