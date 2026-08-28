import { DetailSkeleton } from '@/components/ui/skeleton'
import { createFileRoute, redirect, useRouter } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api, type NetgroupTriple } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { decodeDN } from '@/lib/utils'
import { useSyncedForm } from '@/lib/form-sync'
import { UnsavedChangesGuard } from '@/components/unsaved-changes-guard'
import { ReadOnlyNotice } from '@/components/read-only-notice'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Callout } from '@/components/ui/callout'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { MultiValueField } from '@/components/ui/multi-value-field'
import { TripleEditor } from '@/components/netgroup-triples'
import { NetgroupExpansionPanel } from '@/components/netgroup-expansion'
import { ArrowLeft, Save, Trash2, Plus } from 'lucide-react'
import { useState } from 'react'

export const Route = createFileRoute('/netgroups/$dn')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  component: NetgroupDetailPage,
})

function NetgroupDetailPage() {
  const { dn: encodedDN } = Route.useParams()
  const dn = decodeDN(encodedDN)
  const router = useRouter()
  const queryClient = useQueryClient()
  const { hasPermission } = useAuth()
  const canWrite = hasPermission('groups:write')
  const canDelete = hasPermission('groups:delete')

  const [deleteOpen, setDeleteOpen] = useState(false)
  const [newMember, setNewMember] = useState('')

  const { data: netgroup, isLoading, error } = useQuery({
    queryKey: ['netgroup', dn],
    queryFn: ({ signal }) => api.netgroups.get(dn, signal),
  })

  // Nesting resolved. Its own query because it reads the whole collection, and
  // because a stale expansion next to a fresh entry would be worse than a
  // moment's delay.
  const { data: expansion } = useQuery({
    queryKey: ['netgroup', dn, 'expand'],
    queryFn: ({ signal }) => api.netgroups.expand(dn, signal),
    enabled: !!netgroup,
  })

  const serverForm = netgroup && {
    description: netgroup.description || '',
    triples: netgroup.triples,
    members: netgroup.members || [],
  }
  const [form, setForm] = useState({
    description: '',
    triples: [] as NetgroupTriple[],
    members: [] as string[],
  })
  const isDirty = useSyncedForm(serverForm || undefined, form, setForm)

  const updateMutation = useMutation({
    mutationFn: () =>
      api.netgroups.update(dn, {
        description: form.description,
        triples: form.triples,
        members: form.members,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['netgroups'] })
      queryClient.invalidateQueries({ queryKey: ['netgroup', dn] })
      toast.success('Netgroup updated')
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const deleteMutation = useMutation({
    mutationFn: () => api.netgroups.delete(dn),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ['netgroups'] })
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      toast.success('Netgroup deleted')
      // Sudo names a netgroup by "+name" and nothing ties that string to the
      // entry, so the rules that named it now match nobody. Said out loud,
      // because the directory will not.
      if (res.warnings.length > 0) {
        toast.warning(res.warnings.join(' | '), { duration: 15000 })
      }
      router.navigate({ to: '/netgroups' })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  if (isLoading) return <DetailSkeleton cards={2} />
  if (error) {
    return <Callout variant="error">Failed to load netgroup: {error.message}</Callout>
  }

  return (
    <div className="space-y-4">
      <UnsavedChangesGuard when={isDirty} />

      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          aria-label="Back to netgroups"
          onClick={() => router.navigate({ to: '/netgroups' })}
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="min-w-0">
          <h1 className="text-2xl font-bold">{netgroup?.cn}</h1>
          <p className="break-all text-sm text-muted-foreground">{netgroup?.dn}</p>
        </div>
      </div>

      {!canWrite && <ReadOnlyNotice />}

      {netgroup && netgroup.malformed && netgroup.malformed.length > 0 && (
        <Callout variant="warning">
          <p className="font-medium">
            {netgroup.malformed.length} value{netgroup.malformed.length === 1 ? '' : 's'} in this
            entry could not be read
          </p>
          <p className="mt-1">
            They are kept exactly as they are and saving will not disturb them — but nothing here
            can edit them, and a user rename cannot migrate them either.
          </p>
          <ul className="mt-2 space-y-0.5 font-mono text-xs">
            {netgroup.malformed.map((value) => (
              <li key={value}>{value}</li>
            ))}
          </ul>
        </Callout>
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Information</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="cn">Name (CN)</Label>
                <Input id="cn" value={netgroup?.cn || ''} disabled />
                <p className="text-xs text-muted-foreground">
                  The name cannot be changed after creation — sudo rules and other netgroups
                  reference it by name.
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="description">Description</Label>
                <Input
                  id="description"
                  value={form.description}
                  onChange={(e) => setForm({ ...form, description: e.target.value })}
                  disabled={!canWrite}
                />
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Members ({form.triples.length})</CardTitle>
              <CardDescription>
                Each line is one triple: a user, a host, or a user pinned to a host
              </CardDescription>
            </CardHeader>
            <CardContent>
              <TripleEditor
                triples={form.triples}
                onChange={(triples) => setForm({ ...form, triples })}
                canWrite={canWrite}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Nested netgroups ({form.members.length})</CardTitle>
              <CardDescription>
                Everything these hold counts as part of this netgroup too
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <MultiValueField
                values={form.members}
                onRemove={(value) =>
                  setForm({ ...form, members: form.members.filter((m) => m !== value) })
                }
                canWrite={canWrite}
                label="nested netgroup"
                empty="None nested"
              />
              {canWrite && (
                <div className="flex gap-2">
                  <Input
                    value={newMember}
                    onChange={(e) => setNewMember(e.target.value)}
                    placeholder="Netgroup name"
                    onKeyDown={(e) => {
                      if (e.key !== 'Enter') return
                      e.preventDefault()
                      const value = newMember.trim()
                      if (value && !form.members.includes(value)) {
                        setForm({ ...form, members: [...form.members, value] })
                      }
                      setNewMember('')
                    }}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      const value = newMember.trim()
                      if (value && !form.members.includes(value)) {
                        setForm({ ...form, members: [...form.members, value] })
                      }
                      setNewMember('')
                    }}
                  >
                    <Plus className="h-4 w-4 mr-1" />
                    Add
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          {canWrite && (
            <div className="flex justify-end">
              <Button onClick={() => updateMutation.mutate()} disabled={updateMutation.isPending}>
                <Save className="h-4 w-4 mr-1" />
                {updateMutation.isPending ? 'Saving...' : 'Save Changes'}
              </Button>
            </div>
          )}
        </div>

        <div className="space-y-4">
          <NetgroupExpansionPanel expansion={expansion} />

          {canDelete && (
            <Card className="border-destructive/50">
              <CardHeader>
                <CardTitle className="text-destructive">Danger Zone</CardTitle>
                <CardDescription>Irreversible actions</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-destructive/30 bg-destructive/5 p-4">
                  <div>
                    <p className="font-medium">Delete this netgroup</p>
                    <p className="text-sm text-muted-foreground">
                      Any sudo rule naming “+{netgroup?.cn}” will keep the reference and match
                      nobody. You will be told which.
                    </p>
                  </div>
                  <ConfirmDialog
                    open={deleteOpen}
                    onOpenChange={setDeleteOpen}
                    trigger={
                      <Button variant="destructive">
                        <Trash2 className="h-4 w-4 mr-1" />
                        Delete
                      </Button>
                    }
                    title="Delete netgroup"
                    description={`Delete "${netgroup?.cn}"? Sudo rules and netgroups that name it keep pointing at a name that no longer exists — the directory does not track those references, so nothing will clean them up.`}
                    confirmLabel="Delete netgroup"
                    error={deleteMutation.error?.message}
                    isPending={deleteMutation.isPending}
                    onConfirm={() => deleteMutation.mutate()}
                  />
                </div>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
