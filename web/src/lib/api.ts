import { z } from 'zod'

const API_BASE = '/api'

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

// Callback for when the API receives a 401 (session expired)
let onAuthError: (() => void) | null = null

export function setOnAuthError(callback: (() => void) | null) {
  onAuthError = callback
}

const FETCH_TIMEOUT_MS = 30_000

/**
 * Validates a response, or fails with a message worth showing.
 *
 * `schema.parse` throws a ZodError whose `.message` is a JSON dump of every
 * issue — and the UI shows `error.message` verbatim in a toast or a Callout, so
 * a backend that drifted by one field produced "Failed to load users:
 * [{"code":"invalid_type","expected":"string"…". Unreadable, and it looks like
 * the app is broken rather than out of step with the server. The detail still
 * matters, so it goes to the console where a developer will find it.
 */
function parseResponse<T>(schema: z.ZodType<T>, data: unknown, endpoint: string): T {
  const result = schema.safeParse(data)
  if (result.success) {
    return result.data
  }
  console.error(`Unexpected response shape from ${endpoint}`, result.error.issues, data)
  throw new ApiError(
    0,
    'The server sent something this version of the app does not understand. ' +
      'It may have been updated — reload the page, and report it if it persists.',
  )
}

async function fetchApi<T>(
  endpoint: string,
  options: RequestInit = {},
  schema?: z.ZodType<T>,
  signal?: AbortSignal,
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  }

  // Compose external signal (from React Query) with a timeout to prevent hung requests
  const controller = new AbortController()
  let timedOut = false
  const timeoutId = setTimeout(() => {
    timedOut = true
    controller.abort()
  }, FETCH_TIMEOUT_MS)

  if (signal) {
    if (signal.aborted) {
      controller.abort()
    } else {
      signal.addEventListener('abort', () => controller.abort(), { once: true })
    }
  }

  try {
    const response = await fetch(`${API_BASE}${endpoint}`, {
      ...options,
      headers,
      // Session token now lives in an HttpOnly cookie set by the backend at
      // login; credentials: 'include' makes the browser ship that cookie on
      // every API call (including Vite's dev proxy hop).
      credentials: 'include',
      signal: controller.signal,
    })

    if (!response.ok) {
      if (response.status === 401) {
        onAuthError?.()
        throw new ApiError(response.status, 'Session expired')
      }
      const error = await response.json().catch(() => ({ error: 'Unknown error' }))
      throw new ApiError(response.status, error.error || 'Request failed')
    }

    const data = await response.json()

    if (schema) {
      return parseResponse(schema, data, endpoint)
    }

    return data as T
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      // Only OUR timeout is a real failure. A cancellation coming from the
      // caller's signal (React Query aborting an in-flight query on
      // navigation/unmount) must propagate as the original AbortError:
      // translating it into a regular error breaks the library's cancellation
      // contract — the phantom failure enters the retry path and can strand
      // the query in a permanently "fetching" state (infinite spinner until a
      // full page reload).
      if (!timedOut) {
        throw error
      }
      throw new ApiError(0, 'Request timed out')
    }
    throw error
  } finally {
    clearTimeout(timeoutId)
  }
}

// Public API fetch (no auth header)
async function fetchApiPublic<T>(
  endpoint: string,
  options: RequestInit = {},
  schema?: z.ZodType<T>,
  signal?: AbortSignal,
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  }

  const controller = new AbortController()
  let timedOut = false
  const timeoutId = setTimeout(() => {
    timedOut = true
    controller.abort()
  }, FETCH_TIMEOUT_MS)

  if (signal) {
    if (signal.aborted) {
      controller.abort()
    } else {
      signal.addEventListener('abort', () => controller.abort(), { once: true })
    }
  }

  try {
    const response = await fetch(`${API_BASE}${endpoint}`, {
      ...options,
      headers,
      signal: controller.signal,
    })

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Unknown error' }))
      throw new ApiError(response.status, error.error || 'Request failed')
    }

    const data = await response.json()

    if (schema) {
      return parseResponse(schema, data, endpoint)
    }

    return data as T
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      // See fetchApi: propagate caller-initiated cancellations untouched.
      if (!timedOut) {
        throw error
      }
      throw new ApiError(0, 'Request timed out')
    }
    throw error
  } finally {
    clearTimeout(timeoutId)
  }
}

// Schemas
export const SessionSchema = z.object({
  id: z.string().optional(),
  userDn: z.string(),
  userUid: z.string(),
  displayName: z.string(),
  mail: z.string().optional(),
  roleName: z.string(),
  permissions: z.array(z.string()),
  expiresAt: z.string(),
})

export const LoginResponseSchema = z.object({
  token: z.string(),
  session: SessionSchema,
})

// MeAvatarSchema is the current user's photo, fetched separately from the
// session (which no longer carries the base64 photo to keep session blobs lean).
export const MeAvatarSchema = z.object({
  jpegPhoto: z.string().optional(),
})

/**
 * What an entry's `pwdAccountLockedTime` means right now, resolved by the
 * backend against the policy that applies to the entry.
 *
 * `accountLocked` only says the attribute is present, and a ppolicy lockout
 * whose `pwdLockoutDuration` has elapsed leaves it present until the entry's
 * next bind — so the attribute alone cannot tell a lock in force from one the
 * directory has already stopped honouring.
 */
export const LockoutSchema = z.object({
  // manual: locked by an administrator, never lifts on its own.
  // active: a ppolicy lockout still in force (liftsAt, when the duration is not 0).
  // lifted: the duration has elapsed; the account authenticates again.
  // unknown: the applicable policy could not be read, so neither can be claimed.
  state: z.enum(['none', 'manual', 'active', 'lifted', 'unknown']),
  lockedAt: z.string().optional(),
  // When the lockout duration runs out, RFC 3339. In the past under 'lifted'.
  liftsAt: z.string().optional(),
  // The applicable pwdLockoutDuration; 0 means the lock never lifts by itself.
  durationSeconds: z.number().optional(),
  // The policy that governs this entry: its own pwdPolicySubentry, or the
  // configured LDAP_DEFAULT_POLICY_DN. Empty when neither is known.
  policyDn: z.string().optional(),
})

export type Lockout = z.infer<typeof LockoutSchema>

export const UserSchema = z.object({
  dn: z.string(),
  uid: z.string(),
  cn: z.string(),
  givenName: z.string().optional(),
  sn: z.string(),
  displayName: z.string().optional(),
  mail: z.string().optional(),
  telephoneNumber: z.string().optional(),
  title: z.string().optional(),
  departmentNumber: z.string().optional(),
  o: z.string().optional(),
  employeeNumber: z.string().optional(),
  employeeType: z.string().optional(),
  initials: z.string().optional(),
  manager: z.string().optional(),
  uidNumber: z.number().optional(),
  gidNumber: z.number().optional(),
  homeDirectory: z.string().optional(),
  loginShell: z.string().optional(),
  gecos: z.string().optional(),
  description: z.string().optional(),
  jpegPhoto: z.string().optional(),
  sshPublicKey: z.array(z.string()).optional(),
  hasPassword: z.boolean().optional(),
  accountLocked: z.boolean().optional(),
  objectClasses: z.array(z.string()).optional(),
  // Samba attributes
  sambaSID: z.string().optional(),
  sambaPrimaryGroupSID: z.string().optional(),
  sambaAcctFlags: z.string().optional(),
  sambaHomePath: z.string().optional(),
  sambaHomeDrive: z.string().optional(),
  sambaLogonScript: z.string().optional(),
  sambaProfilePath: z.string().optional(),
  sambaDomainName: z.string().optional(),
  sambaPwdLastSet: z.string().optional(),
  sambaPwdCanChange: z.string().optional(),
  sambaPwdMustChange: z.string().optional(),
  sambaKickoffTime: z.string().optional(),
  // Shadow attributes
  shadowLastChange: z.number().optional(),
  shadowMin: z.number().optional(),
  shadowMax: z.number().optional(),
  shadowWarning: z.number().optional(),
  shadowInactive: z.number().optional(),
  shadowExpire: z.number().optional(),
  shadowFlag: z.number().optional(),
  // Password policy operational attributes
  pwdAccountLockedTime: z.string().optional(),
  pwdEndTime: z.string().optional(),
  pwdFailureTime: z.array(z.string()).optional(),
  pwdChangedTime: z.string().optional(),
  pwdGraceUseTime: z.array(z.string()).optional(),
  pwdReset: z.boolean().optional(),
  pwdPolicySubentry: z.string().optional(),
  lockout: LockoutSchema.optional(),
})

// UserListItemSchema is the narrow projection returned by GET /users. It
// carries no photo and no password state: a row only needs these fields, and
// keeping the payload small is what makes paging worthwhile.
export const UserListItemSchema = z.object({
  dn: z.string(),
  uid: z.string(),
  cn: z.string(),
  displayName: z.string().optional(),
  mail: z.string().optional(),
  employeeType: z.string().optional(),
  hasPhoto: z.boolean().optional(),
  groupsCount: z.number().optional(),
  accountLocked: z.boolean().optional(),
  pwdAccountLockedTime: z.string().optional(),
  // The account's expiry. shadowExpire is deliberately absent: slapd ignores it,
  // so it cannot say whether the account still works.
  pwdEndTime: z.string().optional(),
  lockout: LockoutSchema.optional(),
})

export const UsersResponseSchema = z.object({
  data: z.array(UserListItemSchema),
  // Matches for the current search and status filter, across all pages.
  total: z.number(),
  // Locked/expired matches for the search term, whatever the status filter —
  // labels the "Show inactive" toggle without a second request.
  inactiveCount: z.number().optional(),
  limit: z.number().optional(),
  offset: z.number().optional(),
  truncated: z.boolean().optional(),
})

// Result of a bulk CSV import (POST /users/import, /groups/import).
export const ImportResultSchema = z.object({
  created: z.number(),
  failed: z.number(),
  results: z.array(
    z.object({
      index: z.number(),
      key: z.string(),
      status: z.enum(['created', 'error']),
      error: z.string().optional(),
    }),
  ),
})
export type ImportResult = z.infer<typeof ImportResultSchema>
export type ImportRow = Record<string, unknown>

// Result of renaming a user (POST /users/{dn}/change-uid): the renamed user
// plus any non-fatal reference-migration warnings.
export const ChangeUidResultSchema = z.object({
  user: UserSchema,
  // Tolerate null/omitted (a nil slice serialises as null) → treat as empty.
  warnings: z.array(z.string()).nullish().transform((v) => v ?? []),
})
export type ChangeUidResult = z.infer<typeof ChangeUidResultSchema>

// Result of a mutation that also had to clean up references (DELETE /users/{dn}).
// The operation itself succeeded; warnings name what it could not reach.
export const WarningsResultSchema = z.object({
  message: z.string().optional(),
  warnings: z.array(z.string()).nullish().transform((v) => v ?? []),
})

export const GroupSchema = z.object({
  dn: z.string(),
  cn: z.string(),
  gidNumber: z.number(),
  description: z.string().optional(),
  memberUid: z.array(z.string()).optional(),
  objectClasses: z.array(z.string()).optional(),
  // Samba attributes
  sambaSID: z.string().optional(),
  sambaGroupType: z.string().optional(),
  displayName: z.string().optional(),
})

// GroupsResponseSchema covers the endpoints that return whole groups
// (a user's or a group's memberships), where the member list is the point.
export const GroupsResponseSchema = z.object({
  data: z.array(GroupSchema),
  total: z.number(),
  truncated: z.boolean().optional(),
})

// GroupListItemSchema is the projection returned by GET /groups: row fields
// plus a member count, instead of every group's full member list.
export const GroupListItemSchema = z.object({
  dn: z.string(),
  cn: z.string(),
  gidNumber: z.number(),
  description: z.string().optional(),
  membersCount: z.number().optional(),
})

// A group's membership, resolved server-side. The uid is the value the group
// entry stores; the rest is only present when a user entry carries that uid, so
// a dangling membership still renders instead of vanishing.
export const GroupMemberSchema = z.object({
  uid: z.string(),
  dn: z.string().optional(),
  cn: z.string().optional(),
  displayName: z.string().optional(),
  hasPhoto: z.boolean().optional(),
})

export const GroupMembersResponseSchema = z.object({
  data: z.array(GroupMemberSchema),
})

export type GroupMember = z.infer<typeof GroupMemberSchema>

export const GroupListResponseSchema = z.object({
  data: z.array(GroupListItemSchema),
  total: z.number(),
  limit: z.number().optional(),
  offset: z.number().optional(),
  truncated: z.boolean().optional(),
})

export const AuditLogSchema = z.object({
  id: z.string(),
  actorDn: z.string(),
  actorUid: z.string(),
  action: z.string(),
  resourceType: z.string(),
  resourceDn: z.string().optional(),
  details: z.record(z.string(), z.unknown()).optional(),
  ipAddress: z.string().optional(),
  userAgent: z.string().optional(),
  createdAt: z.string(),
})

export const AuditLogsResponseSchema = z.object({
  data: z.array(AuditLogSchema),
  total: z.number(),
  limit: z.number(),
  offset: z.number(),
})

export const AuditActorsResponseSchema = z.object({
  data: z.array(z.string()),
})

// FieldChangeSchema is one entry of details.changes: the structured diff the
// handlers persist for update actions. A masked change (a password) reports the
// field but never a value.
export const FieldChangeSchema = z.object({
  field: z.string(),
  old: z.string().optional(),
  new: z.string().optional(),
  masked: z.boolean().optional(),
})
export type FieldChange = z.infer<typeof FieldChangeSchema>

// OverviewSchema is the dashboard payload: aggregates computed by the backend
// rather than by counting a full listing in the browser.
export const OverviewSchema = z.object({
  users: z.object({
    total: z.number(),
    active: z.number(),
    locked: z.number(),
    expired: z.number(),
    expiring: z
      .array(
        z.object({
          dn: z.string(),
          uid: z.string(),
          displayName: z.string().optional(),
          expiresAt: z.string(),
        }),
      )
      .nullish()
      .transform((v) => v ?? []),
  }),
  groups: z.number(),
  sudoRoles: z.number(),
  policies: z.number(),
  truncated: z.boolean().optional(),
})
export type Overview = z.infer<typeof OverviewSchema>

export const NextIDsSchema = z.object({
  nextUid: z.number(),
  nextGid: z.number(),
  minUid: z.number(),
  minGid: z.number(),
})

export const SudoRoleSchema = z.object({
  dn: z.string(),
  cn: z.string(),
  description: z.string().optional(),
  sudoUser: z.array(z.string()).optional(),
  sudoHost: z.array(z.string()).optional(),
  sudoCommand: z.array(z.string()).optional(),
  sudoRunAs: z.array(z.string()).optional(),
  sudoRunAsUser: z.array(z.string()).optional(),
  sudoRunAsGroup: z.array(z.string()).optional(),
  sudoOption: z.array(z.string()).optional(),
  sudoOrder: z.number().optional(),
  sudoNotBefore: z.string().optional(),
  sudoNotAfter: z.string().optional(),
})

export const SudoRolesResponseSchema = z.object({
  data: z.array(SudoRoleSchema),
  total: z.number(),
  limit: z.number().optional(),
  offset: z.number().optional(),
  truncated: z.boolean().optional(),
})

export const PasswordPolicySchema = z.object({
  dn: z.string(),
  cn: z.string(),
  description: z.string().optional(),
  pwdAttribute: z.string().optional(),
  pwdMinAge: z.number().optional(),
  pwdMaxAge: z.number().optional(),
  pwdInHistory: z.number().optional(),
  pwdCheckQuality: z.number().optional(),
  pwdMinLength: z.number().optional(),
  pwdExpireWarning: z.number().optional(),
  pwdGraceAuthNLimit: z.number().optional(),
  pwdLockout: z.boolean().optional(),
  pwdLockoutDuration: z.number().optional(),
  pwdMaxFailure: z.number().optional(),
  pwdFailureCountInterval: z.number().optional(),
  pwdMustChange: z.boolean().optional(),
  pwdAllowUserChange: z.boolean().optional(),
  pwdSafeModify: z.boolean().optional(),
  pwdCheckModule: z.string().optional(),
})

export const PasswordPoliciesResponseSchema = z.object({
  data: z.array(PasswordPolicySchema),
  total: z.number(),
  limit: z.number().optional(),
  offset: z.number().optional(),
  truncated: z.boolean().optional(),
})

// Whether the directory actually enforces a password policy. "unknown" is the
// normal answer and means the app cannot tell from its bind identity — every
// readable signal is identical either way — not that anything is wrong. Only a
// self-test, or a readable cn=config, settles it.
export const PolicyEnforcementSchema = z
  .enum(['enforced', 'not-enforced', 'unknown'])
  .catch('unknown')
export type PolicyEnforcement = z.infer<typeof PolicyEnforcementSchema>

// Outcome of POST /admin/policy-selftest.
export const PolicySelfTestSchema = z.object({
  result: PolicyEnforcementSchema,
  probeDn: z.string(),
  steps: z.array(z.string()).nullish().transform((v) => v ?? []),
  cleanedUp: z.boolean(),
  detail: z.string().optional(),
})
export type PolicySelfTest = z.infer<typeof PolicySelfTestSchema>

export const ConfigValueSchema = z.object({
  value: z.union([z.string(), z.number(), z.boolean(), z.array(z.string())]),
  source: z.enum(['env', 'default']),
  envVar: z.string().optional(),
})

export const ConfigResponseSchema = z.object({
  server: z.object({
    host: ConfigValueSchema,
    port: ConfigValueSchema,
  }),
  database: z.object({
    url: ConfigValueSchema,
  }),
  redis: z.object({
    url: ConfigValueSchema,
  }),
  ldap: z.object({
    url: ConfigValueSchema,
    baseDn: ConfigValueSchema,
    bindDn: ConfigValueSchema,
    userOu: ConfigValueSchema,
    groupOu: ConfigValueSchema,
    sudoersOu: ConfigValueSchema,
    ppolicyOu: ConfigValueSchema,
    defaultPolicyDn: ConfigValueSchema,
    minUid: ConfigValueSchema,
    minGid: ConfigValueSchema,
    tlsMode: ConfigValueSchema,
    tlsSkipVerify: ConfigValueSchema,
  }),
  session: z.object({
    ttl: ConfigValueSchema,
  }),
  app: z.object({
    adminGroup: ConfigValueSchema,
    organization: ConfigValueSchema,
    publicUrl: ConfigValueSchema,
    modules: ConfigValueSchema,
    usersObjects: ConfigValueSchema,
    groupsObjects: ConfigValueSchema,
  }),
  // Probed from the directory, not configured — no env var behind these.
  capabilities: z
    .object({
      // Whether pwdEndTime can be written at all, not whether writing it will be
      // enforced — that is policyEnforcement, a genuinely different question.
      scheduledExpirySupported: z.boolean(),
      policyEnforcement: PolicyEnforcementSchema,
    })
    .optional(),
  mail: z.object({
    host: ConfigValueSchema,
    port: ConfigValueSchema,
    user: ConfigValueSchema,
    password: ConfigValueSchema,
    from: ConfigValueSchema,
    ssl: ConfigValueSchema,
  }),
})

export const PasswordResetInfoSchema = z.object({
  uid: z.string(),
  displayName: z.string(),
  organization: z.string(),
})

export const SchemaSchema = z.object({
  objectClasses: z.record(z.string(), z.object({
    oid: z.string(),
    name: z.string(),
    description: z.string().optional(),
    superior: z.array(z.string()).optional(),
    kind: z.string(),
    must: z.array(z.string()).optional(),
    may: z.array(z.string()).optional(),
  })),
  attributeTypes: z.record(z.string(), z.object({
    oid: z.string(),
    name: z.string(),
    description: z.string().optional(),
    syntax: z.string().optional(),
    singleValue: z.boolean(),
    noUserModification: z.boolean(),
    usage: z.string().optional(),
  })),
})

// Types
export type Session = z.infer<typeof SessionSchema>
export type LoginResponse = z.infer<typeof LoginResponseSchema>
export type User = z.infer<typeof UserSchema>
export type UserListItem = z.infer<typeof UserListItemSchema>
export type UsersResponse = z.infer<typeof UsersResponseSchema>
export type Group = z.infer<typeof GroupSchema>
export type GroupListItem = z.infer<typeof GroupListItemSchema>
export type GroupsResponse = z.infer<typeof GroupsResponseSchema>
export type GroupListResponse = z.infer<typeof GroupListResponseSchema>
export type AuditLog = z.infer<typeof AuditLogSchema>
export type AuditLogsResponse = z.infer<typeof AuditLogsResponseSchema>
export type NextIDs = z.infer<typeof NextIDsSchema>
export type SudoRole = z.infer<typeof SudoRoleSchema>
export type SudoRolesResponse = z.infer<typeof SudoRolesResponseSchema>
export type PasswordPolicy = z.infer<typeof PasswordPolicySchema>
export type PasswordPoliciesResponse = z.infer<typeof PasswordPoliciesResponseSchema>
export type ConfigValue = z.infer<typeof ConfigValueSchema>
export type ConfigResponse = z.infer<typeof ConfigResponseSchema>
export type PasswordResetInfo = z.infer<typeof PasswordResetInfoSchema>

// Scheduled Tasks schemas
export const TaskRunSchema = z.object({
  id: z.string(),
  taskName: z.string(),
  startedAt: z.string(),
  completedAt: z.string().nullable().optional(),
  status: z.string(),
  usersChecked: z.number(),
  notificationsSent: z.number(),
  errors: z.array(z.string()).nullable().optional(),
  triggeredBy: z.string(),
})

export const TaskRunsResponseSchema = z.object({
  data: z.array(TaskRunSchema),
  total: z.number(),
})

export const TriggerTaskResponseSchema = z.object({
  runId: z.string(),
  taskName: z.string(),
  usersChecked: z.number(),
  notificationsSent: z.number(),
  errors: z.array(z.string()).nullable().optional(),
})

export const ScheduledTasksConfigSchema = z.object({
  usersExpiration: ConfigValueSchema,
  passwordsExpiration: ConfigValueSchema,
  // The lockout scan runs far more often than the expiration tasks: it looks for
  // something the directory erases at the account's next sign-in.
  lockoutScan: ConfigValueSchema,
})

export type TaskRun = z.infer<typeof TaskRunSchema>
export type TaskRunsResponse = z.infer<typeof TaskRunsResponseSchema>
export type TriggerTaskResponse = z.infer<typeof TriggerTaskResponseSchema>
export type ScheduledTasksConfig = z.infer<typeof ScheduledTasksConfigSchema>
export type Schema = z.infer<typeof SchemaSchema>

// Paging, sorting and filtering accepted by the user and group list endpoints.
// Omitting `limit` returns every match up to LDAP_SEARCH_SIZE_LIMIT, silently:
// always pass one. Pickers go through usePickerSearch, which does.
export interface ListParams {
  search?: string
  sort?: string
  order?: 'asc' | 'desc'
  limit?: number
  offset?: number
  status?: 'active' | 'inactive'
}

function listQuery(params?: ListParams): string {
  if (!params) return ''
  const q = new URLSearchParams()
  const search = params.search?.trim()
  if (search) q.set('search', search)
  if (params.sort) q.set('sort', params.sort)
  if (params.order) q.set('order', params.order)
  if (params.limit) q.set('limit', String(params.limit))
  if (params.offset) q.set('offset', String(params.offset))
  if (params.status) q.set('status', params.status)
  const query = q.toString()
  return query ? `?${query}` : ''
}

// URL of a user's photo. Used as an <img> source in lists so the browser
// fetches and caches the visible rows' photos itself.
export function userPhotoUrl(dn: string): string {
  return `${API_BASE}/users/${encodeURIComponent(dn)}/photo`
}

// Filters accepted by the audit log listing and its CSV export. The two take
// the same set so an export always covers the view it was taken from.
export interface AuditLogParams {
  actorUid?: string
  resourceType?: string
  action?: string
  // Substring match over the actor uid and the resource DN.
  search?: string
  // Inclusive day bounds, YYYY-MM-DD.
  from?: string
  to?: string
}

function auditQuery(params?: AuditLogParams & { limit?: number; offset?: number }): string {
  if (!params) return ''
  const q = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '' && value !== 0) {
      q.set(key, String(value))
    }
  }
  const query = q.toString()
  return query ? `?${query}` : ''
}

// API functions
export const api = {
  auth: {
    login: (username: string, password: string) =>
      fetchApi('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      }, LoginResponseSchema),

    logout: () =>
      fetchApi('/auth/logout', { method: 'POST' }),

    me: (signal?: AbortSignal) =>
      fetchApi('/auth/me', {}, SessionSchema, signal),

    avatar: (signal?: AbortSignal) =>
      fetchApi('/auth/me/avatar', {}, MeAvatarSchema, signal),

    changePassword: (password: string) =>
      fetchApi('/auth/change-password', {
        method: 'POST',
        body: JSON.stringify({ password }),
      }),
  },

  users: {
    list: (params?: ListParams, signal?: AbortSignal) =>
      fetchApi(`/users${listQuery(params)}`, {}, UsersResponseSchema, signal),

    get: (dn: string, signal?: AbortSignal) =>
      fetchApi(`/users/${encodeURIComponent(dn)}`, {}, UserSchema, signal),

    create: (data: {
      uid: string
      givenName: string
      sn: string
      cn?: string
      displayName?: string
      mail?: string
      telephoneNumber?: string
      title?: string
      departmentNumber?: string
      o?: string
      employeeType?: string
      uidNumber: number
      gidNumber: number
      homeDirectory?: string
      loginShell?: string
      password?: string
      description?: string
      groups?: string[]  // Group CNs to add the user to
      createPrimaryGroup?: boolean  // If true, create a posixGroup with CN=UID and the given gidNumber
      expirationDate?: string  // ISO date format (YYYY-MM-DD) for account expiration
    }) =>
      fetchApi('/users', {
        method: 'POST',
        body: JSON.stringify(data),
      }, UserSchema),

    update: (dn: string, data: Partial<User> & { password?: string }) =>
      fetchApi(`/users/${encodeURIComponent(dn)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }, UserSchema),

    delete: (dn: string) =>
      fetchApi(`/users/${encodeURIComponent(dn)}`, { method: 'DELETE' }, WarningsResultSchema),

    import: (rows: ImportRow[]) =>
      fetchApi('/users/import', { method: 'POST', body: JSON.stringify({ rows }) }, ImportResultSchema),

    getGroups: (dn: string, signal?: AbortSignal) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/groups`, {}, GroupsResponseSchema, signal),

    changeUid: (dn: string, newUid: string) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/change-uid`, {
        method: 'POST',
        body: JSON.stringify({ newUid }),
      }, ChangeUidResultSchema),

    lock: (dn: string) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/lock`, { method: 'POST' }),

    unlock: (dn: string) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/unlock`, { method: 'POST' }),

    setExpiration: (dn: string, expirationDate: string | null) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/expiration`, {
        method: 'PUT',
        body: JSON.stringify({ expirationDate: expirationDate || '' }),
      }),

    changePassword: (dn: string, password: string) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/password`, {
        method: 'POST',
        body: JSON.stringify({ password }),
      }),

    removePassword: (dn: string) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/password`, {
        method: 'DELETE',
      }),

    setSSHKeys: (dn: string, keys: string[]) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/ssh-keys`, {
        method: 'PUT',
        body: JSON.stringify({ keys }),
      }, UserSchema),

    addSSHKey: (dn: string, key: string) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/ssh-keys`, {
        method: 'POST',
        body: JSON.stringify({ key }),
      }, UserSchema),

    removeSSHKey: (dn: string, key: string) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/ssh-keys`, {
        method: 'DELETE',
        body: JSON.stringify({ key }),
      }, UserSchema),

    getSudoRoles: (dn: string, signal?: AbortSignal) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/sudo-roles`, {}, SudoRolesResponseSchema, signal),

    sendPasswordReset: (dn: string) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/send-password-reset`, { method: 'POST' }),

    updateSamba: (dn: string, data: {
      sambaSID?: string
      sambaPrimaryGroupSID?: string
      sambaAcctFlags?: string
      sambaHomePath?: string
      sambaHomeDrive?: string
      sambaLogonScript?: string
      sambaProfilePath?: string
      sambaDomainName?: string
    }) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/samba`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }, UserSchema),

    updateShadow: (dn: string, data: {
      shadowLastChange?: number
      shadowMin?: number
      shadowMax?: number
      shadowWarning?: number
      shadowInactive?: number
      shadowExpire?: number
      shadowFlag?: number
    }) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/shadow`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }, UserSchema),
  },

  groups: {
    list: (params?: ListParams, signal?: AbortSignal) =>
      fetchApi(`/groups${listQuery(params)}`, {}, GroupListResponseSchema, signal),

    get: (dn: string, signal?: AbortSignal) =>
      fetchApi(`/groups/${encodeURIComponent(dn)}`, {}, GroupSchema, signal),

    create: (data: {
      cn: string
      gidNumber: number
      description?: string
      memberUid?: string[]
    }) =>
      fetchApi('/groups', {
        method: 'POST',
        body: JSON.stringify(data),
      }, GroupSchema),

    update: (dn: string, data: { description?: string; memberUid?: string[] }) =>
      fetchApi(`/groups/${encodeURIComponent(dn)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }, GroupSchema),

    delete: (dn: string) =>
      fetchApi(`/groups/${encodeURIComponent(dn)}`, { method: 'DELETE' }),

    import: (rows: ImportRow[]) =>
      fetchApi('/groups/import', { method: 'POST', body: JSON.stringify({ rows }) }, ImportResultSchema),

    addMember: (dn: string, memberUid: string) =>
      fetchApi(`/groups/${encodeURIComponent(dn)}/members`, {
        method: 'POST',
        body: JSON.stringify({ memberUid }),
      }),

    removeMember: (dn: string, memberUid: string) =>
      fetchApi(`/groups/${encodeURIComponent(dn)}/members`, {
        method: 'DELETE',
        body: JSON.stringify({ memberUid }),
      }),

    members: (dn: string, signal?: AbortSignal) =>
      fetchApi(`/groups/${encodeURIComponent(dn)}/members`, {}, GroupMembersResponseSchema, signal),

    getSudoRoles: (dn: string, signal?: AbortSignal) =>
      fetchApi(`/groups/${encodeURIComponent(dn)}/sudo-roles`, {}, SudoRolesResponseSchema, signal),

    updateSamba: (dn: string, data: {
      sambaSID?: string
      sambaGroupType?: string
      displayName?: string
    }) =>
      fetchApi(`/groups/${encodeURIComponent(dn)}/samba`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }, GroupSchema),
  },

  sudoRoles: {
    list: (params?: ListParams, signal?: AbortSignal) =>
      fetchApi(`/sudo-roles${listQuery(params)}`, {}, SudoRolesResponseSchema, signal),

    get: (dn: string, signal?: AbortSignal) =>
      fetchApi(`/sudo-roles/${encodeURIComponent(dn)}`, {}, SudoRoleSchema, signal),

    create: (data: {
      cn: string
      description?: string
      sudoUser?: string[]
      sudoHost?: string[]
      sudoCommand?: string[]
      sudoRunAs?: string[]
      sudoRunAsUser?: string[]
      sudoRunAsGroup?: string[]
      sudoOption?: string[]
      sudoOrder?: number
      sudoNotBefore?: string
      sudoNotAfter?: string
    }) =>
      fetchApi('/sudo-roles', {
        method: 'POST',
        body: JSON.stringify(data),
      }, SudoRoleSchema),

    update: (dn: string, data: {
      description?: string
      sudoUser?: string[]
      sudoHost?: string[]
      sudoCommand?: string[]
      sudoRunAs?: string[]
      sudoRunAsUser?: string[]
      sudoRunAsGroup?: string[]
      sudoOption?: string[]
      sudoOrder?: number
      sudoNotBefore?: string
      sudoNotAfter?: string
    }) =>
      fetchApi(`/sudo-roles/${encodeURIComponent(dn)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }, SudoRoleSchema),

    delete: (dn: string) =>
      fetchApi(`/sudo-roles/${encodeURIComponent(dn)}`, { method: 'DELETE' }),

    addUser: (dn: string, uid: string) =>
      fetchApi(`/sudo-roles/${encodeURIComponent(dn)}/users`, {
        method: 'POST',
        body: JSON.stringify({ uid }),
      }, SudoRoleSchema),

    removeUser: (dn: string, uid: string) =>
      fetchApi(`/sudo-roles/${encodeURIComponent(dn)}/users`, {
        method: 'DELETE',
        body: JSON.stringify({ uid }),
      }, SudoRoleSchema),

    addGroup: (dn: string, cn: string) =>
      fetchApi(`/sudo-roles/${encodeURIComponent(dn)}/groups`, {
        method: 'POST',
        body: JSON.stringify({ cn }),
      }, SudoRoleSchema),

    removeGroup: (dn: string, cn: string) =>
      fetchApi(`/sudo-roles/${encodeURIComponent(dn)}/groups`, {
        method: 'DELETE',
        body: JSON.stringify({ cn }),
      }, SudoRoleSchema),
  },

  passwordPolicies: {
    list: (params?: ListParams, signal?: AbortSignal) =>
      fetchApi(`/password-policies${listQuery(params)}`, {}, PasswordPoliciesResponseSchema, signal),

    get: (dn: string, signal?: AbortSignal) =>
      fetchApi(`/password-policies/${encodeURIComponent(dn)}`, {}, PasswordPolicySchema, signal),

    create: (data: {
      cn: string
      description?: string
      pwdAttribute?: string
      pwdMinAge?: number
      pwdMaxAge?: number
      pwdInHistory?: number
      pwdCheckQuality?: number
      pwdMinLength?: number
      pwdExpireWarning?: number
      pwdGraceAuthNLimit?: number
      pwdLockout?: boolean
      pwdLockoutDuration?: number
      pwdMaxFailure?: number
      pwdFailureCountInterval?: number
      pwdMustChange?: boolean
      pwdAllowUserChange?: boolean
      pwdSafeModify?: boolean
      pwdCheckModule?: string
    }) =>
      fetchApi('/password-policies', {
        method: 'POST',
        body: JSON.stringify(data),
      }, PasswordPolicySchema),

    update: (dn: string, data: {
      description?: string
      pwdAttribute?: string
      pwdMinAge?: number
      pwdMaxAge?: number
      pwdInHistory?: number
      pwdCheckQuality?: number
      pwdMinLength?: number
      pwdExpireWarning?: number
      pwdGraceAuthNLimit?: number
      pwdLockout?: boolean
      pwdLockoutDuration?: number
      pwdMaxFailure?: number
      pwdFailureCountInterval?: number
      pwdMustChange?: boolean
      pwdAllowUserChange?: boolean
      pwdSafeModify?: boolean
      pwdCheckModule?: string
    }) =>
      fetchApi(`/password-policies/${encodeURIComponent(dn)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }, PasswordPolicySchema),

    delete: (dn: string) =>
      fetchApi(`/password-policies/${encodeURIComponent(dn)}`, { method: 'DELETE' }),
  },

  schema: {
    get: (signal?: AbortSignal) =>
      fetchApi('/schema', {}, SchemaSchema, signal),

    refresh: () =>
      fetchApi('/schema/refresh', { method: 'POST' }, SchemaSchema),
  },

  auditLogs: {
    list: (params?: AuditLogParams & { limit?: number; offset?: number }, signal?: AbortSignal) =>
      fetchApi(`/audit-logs${auditQuery(params)}`, {}, AuditLogsResponseSchema, signal),

    // Actors present in the log, for the actor filter.
    actors: (signal?: AbortSignal) =>
      fetchApi('/audit-logs/actors', {}, AuditActorsResponseSchema, signal),

    // URL of the CSV export for the current filters. Handed to the browser as a
    // download rather than fetched, so a large export never sits in JS memory.
    exportUrl: (params?: AuditLogParams) => `${API_BASE}/audit-logs/export${auditQuery(params)}`,
  },

  nextIds: {
    get: (signal?: AbortSignal) =>
      fetchApi('/next-ids', {}, NextIDsSchema, signal),
  },

  overview: {
    get: (signal?: AbortSignal) =>
      fetchApi('/overview', {}, OverviewSchema, signal),
  },

  admin: {
    getConfig: (signal?: AbortSignal) =>
      fetchApi('/admin/config', {}, ConfigResponseSchema, signal),

    // Writes a throwaway entry to the directory to prove whether a password
    // policy is enforced. Explicit action, never automatic.
    policySelfTest: () =>
      fetchApi('/admin/policy-selftest', { method: 'POST' }, PolicySelfTestSchema),

    scheduledTasks: {
      getConfig: (signal?: AbortSignal) =>
        fetchApi('/admin/scheduled-tasks/config', {}, ScheduledTasksConfigSchema, signal),

      getRuns: (taskName?: string, signal?: AbortSignal) =>
        fetchApi(`/admin/scheduled-tasks/runs${taskName ? `?taskName=${taskName}` : ''}`, {}, TaskRunsResponseSchema, signal),

      triggerUsersExpiration: () =>
        fetchApi('/admin/scheduled-tasks/users-expiration/trigger', { method: 'POST' }, TriggerTaskResponseSchema),

      triggerPasswordsExpiration: () =>
        fetchApi('/admin/scheduled-tasks/passwords-expiration/trigger', { method: 'POST' }, TriggerTaskResponseSchema),

      triggerLockoutScan: () =>
        fetchApi('/admin/scheduled-tasks/lockout-scan/trigger', { method: 'POST' }, TriggerTaskResponseSchema),
    },
  },

  passwordReset: {
    getInfo: (token: string, signal?: AbortSignal) =>
      fetchApiPublic(`/password-reset/${token}`, {}, PasswordResetInfoSchema, signal),

    confirm: (token: string, password: string) =>
      fetchApiPublic(`/password-reset/${token}`, {
        method: 'POST',
        body: JSON.stringify({ password }),
      }),
  },
}
