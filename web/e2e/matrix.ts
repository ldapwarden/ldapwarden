// The axes of the RBAC matrix: which deployments, which accounts.

export interface Server {
  name: string
  port: number
  baseURL: string
  modules: string[]
}

// The defaults, and a deployment that only manages accounts: what a module
// switches off has to disappear from every way into it.
export const SERVERS: Server[] = [
  { name: 'all-modules', port: 8810, modules: ['users', 'groups', 'sudo', 'policies', 'netgroups'] },
  { name: 'users-groups', port: 8811, modules: ['users', 'groups'] },
].map((s) => ({ ...s, baseURL: `http://localhost:${s.port}` }))

export interface Account {
  role: string
  username: string
  password: string
  storageState: (server: Server) => string
}

// Seed accounts (ldap/seed.ldif): admin is in the admin group; viewer is in no
// group that grants anything, hence the read-only role.
export const ACCOUNTS: Account[] = [
  { role: 'admin', username: 'admin', password: 'admin123' },
  { role: 'readonly', username: 'viewer', password: 'viewer123' },
].map((a) => ({ ...a, storageState: (server: Server) => `e2e/.auth/${server.name}-${a.role}.json` }))
