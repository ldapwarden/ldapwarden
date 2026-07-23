import { createFileRoute, redirect } from '@tanstack/react-router'
import { CsvImport, type ImportField } from '@/components/csv-import'
import { api } from '@/lib/api'

const RDN = /^[a-zA-Z0-9._-]+$/
const RDN_HINT = "must contain only letters, digits, '.', '_' or '-'"

const USER_FIELDS: ImportField[] = [
  { key: 'uid', label: 'Username (uid)', type: 'string', required: true, pattern: RDN, patternHint: RDN_HINT, aliases: ['username', 'login'] },
  { key: 'givenName', label: 'First name', type: 'string', required: true, aliases: ['firstname', 'first name'] },
  { key: 'sn', label: 'Last name', type: 'string', required: true, aliases: ['surname', 'lastname', 'last name'] },
  { key: 'uidNumber', label: 'UID number', type: 'number', required: true, aliases: ['uidnumber'] },
  { key: 'gidNumber', label: 'GID number', type: 'number', required: true, aliases: ['gidnumber'] },
  { key: 'cn', label: 'Common name', type: 'string' },
  { key: 'displayName', label: 'Display name', type: 'string' },
  { key: 'mail', label: 'Email', type: 'string', aliases: ['email'] },
  { key: 'telephoneNumber', label: 'Phone', type: 'string', aliases: ['phone', 'telephone'] },
  { key: 'title', label: 'Title', type: 'string' },
  { key: 'departmentNumber', label: 'Department', type: 'string', aliases: ['department'] },
  { key: 'o', label: 'Organization', type: 'string', aliases: ['organization', 'org'] },
  { key: 'employeeType', label: 'Employee type', type: 'string' },
  { key: 'description', label: 'Description', type: 'string' },
  { key: 'loginShell', label: 'Login shell', type: 'string', aliases: ['shell'] },
  { key: 'homeDirectory', label: 'Home directory', type: 'string', aliases: ['home'] },
  { key: 'password', label: 'Password', type: 'string' },
  { key: 'groups', label: 'Groups (CNs)', type: 'stringArray' },
  { key: 'expirationDate', label: 'Expiration date', type: 'string', aliases: ['expiration', 'expires'] },
]

export const Route = createFileRoute('/users/import')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) throw redirect({ to: '/login' })
    if (!context.auth.hasPermission('users:write')) throw redirect({ to: '/users' })
  },
  component: () => (
    <CsvImport
      title="Import users"
      noun="users"
      fields={USER_FIELDS}
      backTo="/users"
      submit={(rows) => api.users.import(rows)}
    />
  ),
})
