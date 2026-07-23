import { createFileRoute, redirect } from '@tanstack/react-router'
import { CsvImport, type ImportField } from '@/components/csv-import'
import { api } from '@/lib/api'

const RDN = /^[a-zA-Z0-9._-]+$/
const RDN_HINT = "must contain only letters, digits, '.', '_' or '-'"

const GROUP_FIELDS: ImportField[] = [
  { key: 'cn', label: 'Name (cn)', type: 'string', required: true, pattern: RDN, patternHint: RDN_HINT, aliases: ['name', 'groupname'] },
  { key: 'gidNumber', label: 'GID number', type: 'number', required: true, aliases: ['gidnumber', 'gid'] },
  { key: 'description', label: 'Description', type: 'string' },
  { key: 'memberUid', label: 'Members (uids)', type: 'stringArray', aliases: ['members', 'memberuid'] },
]

export const Route = createFileRoute('/groups/import')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) throw redirect({ to: '/login' })
    if (!context.auth.hasPermission('groups:write')) throw redirect({ to: '/groups' })
  },
  component: () => (
    <CsvImport
      title="Import groups"
      noun="groups"
      fields={GROUP_FIELDS}
      backTo="/groups"
      submit={(rows) => api.groups.import(rows)}
    />
  ),
})
