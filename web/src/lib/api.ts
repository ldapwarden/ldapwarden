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
  const timeoutId = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS)

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
      return schema.parse(data)
    }

    return data as T
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
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
  const timeoutId = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS)

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
      return schema.parse(data)
    }

    return data as T
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
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
  pwdFailureTime: z.array(z.string()).optional(),
  pwdChangedTime: z.string().optional(),
  pwdGraceUseTime: z.array(z.string()).optional(),
  pwdReset: z.boolean().optional(),
  pwdPolicySubentry: z.string().optional(),
})

export const UsersResponseSchema = z.object({
  data: z.array(UserSchema),
  total: z.number(),
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

export const GroupsResponseSchema = z.object({
  data: z.array(GroupSchema),
  total: z.number(),
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
})

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
export type UsersResponse = z.infer<typeof UsersResponseSchema>
export type Group = z.infer<typeof GroupSchema>
export type GroupsResponse = z.infer<typeof GroupsResponseSchema>
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
})

export type TaskRun = z.infer<typeof TaskRunSchema>
export type TaskRunsResponse = z.infer<typeof TaskRunsResponseSchema>
export type TriggerTaskResponse = z.infer<typeof TriggerTaskResponseSchema>
export type ScheduledTasksConfig = z.infer<typeof ScheduledTasksConfigSchema>
export type Schema = z.infer<typeof SchemaSchema>

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

    changePassword: (password: string) =>
      fetchApi('/auth/change-password', {
        method: 'POST',
        body: JSON.stringify({ password }),
      }),
  },

  users: {
    list: (signal?: AbortSignal) =>
      fetchApi('/users', {}, UsersResponseSchema, signal),

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
      fetchApi(`/users/${encodeURIComponent(dn)}`, { method: 'DELETE' }),

    getGroups: (dn: string, signal?: AbortSignal) =>
      fetchApi(`/users/${encodeURIComponent(dn)}/groups`, {}, GroupsResponseSchema, signal),

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
    list: (signal?: AbortSignal) =>
      fetchApi('/groups', {}, GroupsResponseSchema, signal),

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
    list: (signal?: AbortSignal) =>
      fetchApi('/sudo-roles', {}, SudoRolesResponseSchema, signal),

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
    list: (signal?: AbortSignal) =>
      fetchApi('/password-policies', {}, PasswordPoliciesResponseSchema, signal),

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
    list: (params?: { limit?: number; offset?: number; actorDn?: string; resourceType?: string; action?: string }, signal?: AbortSignal) => {
      const searchParams = new URLSearchParams()
      if (params?.limit) searchParams.set('limit', params.limit.toString())
      if (params?.offset) searchParams.set('offset', params.offset.toString())
      if (params?.actorDn) searchParams.set('actorDn', params.actorDn)
      if (params?.resourceType) searchParams.set('resourceType', params.resourceType)
      if (params?.action) searchParams.set('action', params.action)

      const query = searchParams.toString()
      return fetchApi(`/audit-logs${query ? `?${query}` : ''}`, {}, AuditLogsResponseSchema, signal)
    },
  },

  nextIds: {
    get: (signal?: AbortSignal) =>
      fetchApi('/next-ids', {}, NextIDsSchema, signal),
  },

  admin: {
    getConfig: (signal?: AbortSignal) =>
      fetchApi('/admin/config', {}, ConfigResponseSchema, signal),

    scheduledTasks: {
      getConfig: (signal?: AbortSignal) =>
        fetchApi('/admin/scheduled-tasks/config', {}, ScheduledTasksConfigSchema, signal),

      getRuns: (taskName?: string, signal?: AbortSignal) =>
        fetchApi(`/admin/scheduled-tasks/runs${taskName ? `?taskName=${taskName}` : ''}`, {}, TaskRunsResponseSchema, signal),

      triggerUsersExpiration: () =>
        fetchApi('/admin/scheduled-tasks/users-expiration/trigger', { method: 'POST' }, TriggerTaskResponseSchema),

      triggerPasswordsExpiration: () =>
        fetchApi('/admin/scheduled-tasks/passwords-expiration/trigger', { method: 'POST' }, TriggerTaskResponseSchema),
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
