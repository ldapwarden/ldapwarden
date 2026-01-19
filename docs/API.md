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

## Users

### List Users

```http
GET /api/users
```

**Required permission:** `users:read`

**curl example:**
```bash
curl http://localhost:8000/api/users \
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
      "sn": "Administrator",
      "givenName": "System",
      "displayName": "System Administrator",
      "mail": "admin@example.org",
      "uidNumber": 1000,
      "gidNumber": 1000,
      "homeDirectory": "/home/admin",
      "loginShell": "/bin/bash",
      "accountLocked": false,
      "objectClasses": ["inetOrgPerson", "posixAccount", "shadowAccount"]
    }
  ],
  "total": 1
}
```

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

### Delete User

```http
DELETE /api/users/{dn}
```

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

**curl example:**
```bash
curl http://localhost:8000/api/groups \
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
      "memberUid": ["admin", "jdoe"]
    }
  ],
  "total": 1
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
- `gidNumber` - POSIX GID

**Optional fields:**
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

### Add Member to Group

```http
POST /api/groups/{dn}/members
Content-Type: application/json

{
  "uid": "jsmith"
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
  -d '{"uid": "jsmith"}'
```

### Remove Member from Group

```http
DELETE /api/groups/{dn}/members
Content-Type: application/json

{
  "uid": "jsmith"
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
  -d '{"uid": "jsmith"}'
```

### Get Group Sudo Roles

```http
GET /api/groups/{dn}/sudo-roles
```

**Required permission:** `groups:read`

---

## Sudo Roles

### List Sudo Roles

```http
GET /api/sudo-roles
```

**Required permission:** `users:read`

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

## Audit Logs

### List Audit Logs

```http
GET /api/audit-logs?limit=50&offset=0
```

**Required permission:** `audit:read`

**Query parameters:**
- `limit` - Number of results (default: 50, max: 100)
- `offset` - Pagination offset
- `actorDn` - Filter by actor DN
- `resourceType` - Filter by resource type (user, group, sudorole)
- `action` - Filter by action

**curl example:**
```bash
curl "http://localhost:8000/api/audit-logs?limit=10" \
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
- `login`, `logout`
- `user.create`, `user.update`, `user.delete`, `user.lock`, `user.unlock`
- `group.create`, `group.update`, `group.delete`
- `group.member.add`, `group.member.remove`
- `sudorole.create`, `sudorole.update`, `sudorole.delete`
- `sudorole.user.add`, `sudorole.user.remove`
- `sudorole.group.add`, `sudorole.group.remove`
- `schema.refresh`

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
- `403 Forbidden` - Insufficient permissions
- `404 Not Found` - Resource not found
- `500 Internal Server Error` - Server error

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
