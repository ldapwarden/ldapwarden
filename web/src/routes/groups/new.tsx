import { createFileRoute, redirect, useRouter } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ArrowLeft, Save, Wand2 } from 'lucide-react'
import { UnsavedChangesGuard } from '@/components/unsaved-changes-guard'
import { useCreationFormDirty } from '@/lib/form-sync'
import { useState } from 'react'

export const Route = createFileRoute('/groups/new')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
    if (!context.auth.hasPermission('groups:write')) {
      throw redirect({ to: '/groups' })
    }
  },
  component: NewGroupPage,
})

function NewGroupPage() {
  const router = useRouter()
  const { hasPermission } = useAuth()
  const queryClient = useQueryClient()

  const { data: nextIds } = useQuery({
    queryKey: ['nextIds'],
    queryFn: ({ signal }) => api.nextIds.get(signal),
  })

  const [formData, setFormData] = useState({
    cn: '',
    gidNumber: '',
    description: '',
  })

  const createMutation = useMutation({
    mutationFn: () => api.groups.create({
      cn: formData.cn,
      // Left as the server's job when untouched: 0 means "allocate one". Filling
      // it in here replayed a number read when the page loaded, which two people
      // creating groups at once would both have sent.
      gidNumber: formData.gidNumber === '' ? 0 : parseInt(formData.gidNumber),
      description: formData.description || undefined,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      toast.success('Group created successfully')
      router.navigate({ to: '/groups' })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createMutation.mutate()
  }

  const isDirty = useCreationFormDirty(
    formData,
    createMutation.isPending || createMutation.isSuccess,
  )

  if (!hasPermission('groups:write')) {
    return null
  }

  return (
    <div className="space-y-4">
      <UnsavedChangesGuard when={isDirty} />
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" aria-label="Back to groups" onClick={() => router.navigate({ to: '/groups' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <h1 className="text-2xl font-bold">Create New Group</h1>
      </div>

      <form onSubmit={handleSubmit}>
        <Card className="max-w-xl">
          <CardHeader>
            <CardTitle>Group Information</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="cn">Name (CN) *</Label>
              <Input
                id="cn"
                value={formData.cn}
                onChange={(e) => setFormData({ ...formData, cn: e.target.value })}
                required
                autoFocus
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="gidNumber">GID Number *</Label>
              <div className="flex gap-2">
                <Input
                  id="gidNumber"
                  type="number"
                  value={formData.gidNumber || nextIds?.nextGid?.toString() || ''}
                  onChange={(e) => setFormData({ ...formData, gidNumber: e.target.value })}
                  required
                  min={nextIds?.minGid}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label="Use the next free GID"
                  title="Use the next free GID"
                  // Back to automatic, rather than copying nextGid into the field:
                  // a copied number is the stale one the server would then have
                  // to refuse if someone took it in the meantime.
                  onClick={() => setFormData({ ...formData, gidNumber: '' })}
                  disabled={formData.gidNumber === ''}
                >
                  <Wand2 className="h-4 w-4" />
                </Button>
              </div>
              {nextIds && (
                <p className="text-xs text-muted-foreground">
                  Min: {nextIds.minGid}. Leave it as it is and the next free number is picked
                  when the group is created, so two people creating groups at once cannot
                  both get {nextIds.nextGid}.
                </p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Input
                id="description"
                value={formData.description}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
              />
            </div>

            <div className="flex justify-end pt-4">
              <Button type="submit" disabled={createMutation.isPending}>
                <Save className="h-4 w-4 mr-1" />
                {createMutation.isPending ? 'Creating...' : 'Create Group'}
              </Button>
            </div>
          </CardContent>
        </Card>
      </form>
    </div>
  )
}
