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
import { useState, useEffect } from 'react'

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

  // Auto-fill GID when data is available
  useEffect(() => {
    if (nextIds && !formData.gidNumber) {
      setFormData(prev => ({
        ...prev,
        gidNumber: nextIds.nextGid.toString(),
      }))
    }
  }, [nextIds, formData.gidNumber])

  const createMutation = useMutation({
    mutationFn: () => api.groups.create({
      cn: formData.cn,
      gidNumber: parseInt(formData.gidNumber),
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

  if (!hasPermission('groups:write')) {
    return null
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => router.navigate({ to: '/groups' })}>
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
            {createMutation.error && (
              <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
                {createMutation.error.message}
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="cn">Name (CN) *</Label>
              <Input
                id="cn"
                value={formData.cn}
                onChange={(e) => setFormData({ ...formData, cn: e.target.value })}
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="gidNumber">GID Number *</Label>
              <div className="flex gap-2">
                <Input
                  id="gidNumber"
                  type="number"
                  value={formData.gidNumber}
                  onChange={(e) => setFormData({ ...formData, gidNumber: e.target.value })}
                  required
                  min={nextIds?.minGid}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  title="Auto-generate next available GID"
                  onClick={() => nextIds && setFormData({ ...formData, gidNumber: nextIds.nextGid.toString() })}
                  disabled={!nextIds}
                >
                  <Wand2 className="h-4 w-4" />
                </Button>
              </div>
              {nextIds && <p className="text-xs text-muted-foreground">Min: {nextIds.minGid}, Next available: {nextIds.nextGid}</p>}
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
