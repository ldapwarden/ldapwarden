import { createFileRoute, redirect } from '@tanstack/react-router'
import { CsvImport, type ImportField } from '@/components/csv-import'
import { api } from '@/lib/api'
import { requireModule } from '@/lib/use-server-info'

const RDN = /^[a-zA-Z0-9._-]+$/
const RDN_HINT = "must contain only letters, digits, '.', '_' or '-'"

const USER_FIELDS: ImportField[] = [
  { key: 'uid', label: 'Username (uid)', type: 'string', required: true, pattern: RDN, patternHint: RDN_HINT, aliases: ['username', 'login'], example: 'jdoe' },
  { key: 'givenName', label: 'First name', type: 'string', required: true, aliases: ['firstname', 'first name'], example: 'Jane' },
  { key: 'sn', label: 'Last name', type: 'string', required: true, aliases: ['surname', 'lastname', 'last name'], example: 'Doe' },
  { key: 'uidNumber', label: 'UID number', type: 'number', required: true, aliases: ['uidnumber'], example: '10001' },
  { key: 'gidNumber', label: 'GID number', type: 'number', required: true, aliases: ['gidnumber'], example: '10001' },
  { key: 'cn', label: 'Common name', type: 'string', example: 'Jane Doe' },
  { key: 'displayName', label: 'Display name', type: 'string', example: 'Jane Doe' },
  { key: 'mail', label: 'Email', type: 'string', aliases: ['email'], example: 'jane.doe@example.com' },
  { key: 'telephoneNumber', label: 'Phone', type: 'string', aliases: ['phone', 'telephone'], example: '+33 1 23 45 67 89' },
  { key: 'title', label: 'Title', type: 'string', example: 'Systems Engineer' },
  { key: 'departmentNumber', label: 'Department', type: 'string', aliases: ['department'], example: 'Infrastructure' },
  { key: 'o', label: 'Organization', type: 'string', aliases: ['organization', 'org'], example: 'Example Corp' },
  { key: 'employeeType', label: 'Employee type', type: 'string', example: 'permanent' },
  { key: 'description', label: 'Description', type: 'string', example: 'Imported from HR export' },
  { key: 'loginShell', label: 'Login shell', type: 'string', aliases: ['shell'], example: '/bin/bash' },
  { key: 'homeDirectory', label: 'Home directory', type: 'string', aliases: ['home'], example: '/home/jdoe' },
  { key: 'password', label: 'Password', type: 'string', example: 'ChangeMe-2026' },
  { key: 'groups', label: 'Groups (CNs)', type: 'stringArray', example: 'developers;oncall' },
  { key: 'expirationDate', label: 'Expiration date', type: 'string', aliases: ['expiration', 'expires'], example: '2027-01-31' },
]

export const Route = createFileRoute('/users/import')({
  beforeLoad: async ({ context }) => {
    if (!context.auth.isAuthenticated) throw redirect({ to: '/login' })
    await requireModule(context.queryClient, 'users')
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
