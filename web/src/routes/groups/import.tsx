import { createFileRoute, redirect } from '@tanstack/react-router'
import { CsvImport, type ImportField } from '@/components/csv-import'
import { api } from '@/lib/api'
import { requireModule } from '@/lib/use-server-info'

const RDN = /^[a-zA-Z0-9._-]+$/
const RDN_HINT = "must contain only letters, digits, '.', '_' or '-'"

const GROUP_FIELDS: ImportField[] = [
  { key: 'cn', label: 'Name (cn)', type: 'string', required: true, pattern: RDN, patternHint: RDN_HINT, aliases: ['name', 'groupname'], example: 'developers' },
  { key: 'gidNumber', label: 'GID number', type: 'number', required: true, aliases: ['gidnumber', 'gid'], example: '20001' },
  { key: 'description', label: 'Description', type: 'string', example: 'Application developers' },
  { key: 'memberUid', label: 'Members (uids)', type: 'stringArray', aliases: ['members', 'memberuid'], example: 'jdoe;asmith' },
]

export const Route = createFileRoute('/groups/import')({
  beforeLoad: async ({ context }) => {
    if (!context.auth.isAuthenticated) throw redirect({ to: '/login' })
    await requireModule(context.queryClient, 'groups')
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
