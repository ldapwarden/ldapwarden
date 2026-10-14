import { createFileRoute, redirect } from '@tanstack/react-router'
import { CsvImport, type ImportField } from '@/components/csv-import'
import { api } from '@/lib/api'
import { NETGROUP_TRIPLE_TEXT } from '@/lib/netgroup'
import { requireModule } from '@/lib/use-server-info'

const RDN = /^[a-zA-Z0-9._-]+$/
const RDN_HINT = "must contain only letters, digits, '.', '_' or '-'"

const TRIPLE_HINT = 'must be written “(host,user,domain)”, with both commas'

const NETGROUP_FIELDS: ImportField[] = [
  {
    key: 'cn',
    label: 'Name (cn)',
    type: 'string',
    required: true,
    pattern: RDN,
    patternHint: RDN_HINT,
    aliases: ['name', 'netgroup'],
    example: 'ops-oncall',
  },
  {
    key: 'description',
    label: 'Description',
    type: 'string',
    example: 'On-call operators',
  },
  {
    key: 'triples',
    label: 'Triples',
    type: 'stringArray',
    pattern: NETGROUP_TRIPLE_TEXT,
    patternHint: TRIPLE_HINT,
    aliases: ['nisnetgrouptriple', 'triple', 'members'],
    example: '(,jdoe,);(,asmith,)',
  },
  {
    key: 'members',
    label: 'Nested netgroups',
    type: 'stringArray',
    pattern: RDN,
    patternHint: RDN_HINT,
    aliases: ['membernisnetgroup', 'nested'],
    example: 'ops',
  },
]

export const Route = createFileRoute('/netgroups/import')({
  beforeLoad: async ({ context }) => {
    if (!context.auth.isAuthenticated) throw redirect({ to: '/login' })
    await requireModule(context.queryClient, 'netgroups')
    if (!context.auth.hasPermission('groups:write')) throw redirect({ to: '/netgroups' })
  },
  component: () => (
    <CsvImport
      title="Import netgroups"
      noun="netgroups"
      fields={NETGROUP_FIELDS}
      backTo="/netgroups"
      submit={(rows) => api.netgroups.import(rows)}
      notice={
        <>
          A triple is written <code>(host,user,domain)</code> and an <strong>empty slot means
          “any”</strong>, never “none” — <code>(,jdoe,)</code> is jdoe on every host, and
          <code> (web01,,)</code> is every user on web01. Use <code>-</code> for a slot that must
          match nothing. Several triples in one cell are separated by “;”, and the cell needs
          quoting in most spreadsheets because triples contain commas.
        </>
      }
    />
  ),
})
