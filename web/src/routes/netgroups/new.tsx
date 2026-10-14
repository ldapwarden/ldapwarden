import { createFileRoute, redirect, useRouter } from '@tanstack/react-router'
import { encodeDN } from '@/lib/utils'
import { BackToList } from '@/components/back-to-list'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api, type NetgroupTriple } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Callout } from '@/components/ui/callout'
import { UnsavedChangesGuard } from '@/components/unsaved-changes-guard'
import { useCreationFormDirty } from '@/lib/form-sync'
import { TripleEditor } from '@/components/netgroup-triples'
import { netgroupKind, netgroupKindLabel } from '@/lib/netgroup'
import { Save } from 'lucide-react'
import { useState } from 'react'
import { requireModule } from '@/lib/use-server-info'

export const Route = createFileRoute('/netgroups/new')({
  beforeLoad: async ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
    await requireModule(context.queryClient, 'netgroups')
    if (!context.auth.hasPermission('groups:write')) {
      throw redirect({ to: '/netgroups' })
    }
  },
  component: NewNetgroupPage,
})

function NewNetgroupPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { hasPermission } = useAuth()

  const [cn, setCN] = useState('')
  const [description, setDescription] = useState('')
  const [triples, setTriples] = useState<NetgroupTriple[]>([])

  const createMutation = useMutation({
    mutationFn: () => api.netgroups.create({ cn, description: description || undefined, triples }),
    onSuccess: (netgroup) => {
      queryClient.invalidateQueries({ queryKey: ['netgroups'] })
      toast.success('Netgroup created')
      // To the new entry, not the list: the next step is almost always on it
      // (groups, keys, members), and the list would mean searching for it again.
      router.navigate({ to: '/netgroups/$dn', params: { dn: encodeDN(netgroup.dn) } })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const isDirty = useCreationFormDirty(
    { cn, description, triples },
    createMutation.isPending || createMutation.isSuccess,
  )

  if (!hasPermission('groups:write')) return null

  // The kind is derived from what has been entered so far, so the mismatch that
  // fails silently in sudo is named before the netgroup exists.
  const kind = netgroupKind({ dn: '', cn, triples })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createMutation.mutate()
  }

  return (
    <div className="space-y-4">
      <UnsavedChangesGuard when={isDirty} />

      <div className="flex items-center gap-4">
        <BackToList to="/netgroups" label="Back to netgroups" />
        <h1 className="text-2xl font-bold">Create Netgroup</h1>
      </div>

      <form onSubmit={handleSubmit}>
        <Card>
          <CardHeader>
            <CardTitle>Netgroup</CardTitle>
            <CardDescription>
              A collection of users or hosts a sudo rule can grant to at once, as “+name”
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="cn">Name (CN) *</Label>
              <Input
                id="cn"
                value={cn}
                onChange={(e) => setCN(e.target.value)}
                required
                autoFocus
                placeholder="ops"
              />
              <p className="text-xs text-muted-foreground">
                Referenced by name from sudo rules and other netgroups, and it cannot be changed
                afterwards.
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Input
                id="description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label>Members</Label>
              <TripleEditor triples={triples} onChange={setTriples} canWrite />
            </div>

            {kind === 'mixed' && (
              <Callout variant="warning">
                This netgroup names both users and hosts. That is legal, but a sudo rule reads only
                one side of it — the users when named in Users, the hosts when named in Hosts — and
                ignores the other half without saying so.
              </Callout>
            )}
            {(kind === 'users' || kind === 'hosts') && (
              <Callout variant="info">{netgroupKindLabel(kind)}</Callout>
            )}

            <div className="flex justify-end pt-2">
              <Button type="submit" disabled={createMutation.isPending || !cn.trim()}>
                <Save className="h-4 w-4 mr-1" />
                {createMutation.isPending ? 'Creating...' : 'Create Netgroup'}
              </Button>
            </div>
          </CardContent>
        </Card>
      </form>
    </div>
  )
}
