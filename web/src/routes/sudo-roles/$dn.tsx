import { InlineSpinner } from '@/components/inline-spinner'
import { createFileRoute, redirect, useRouter } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { decodeDN } from '@/lib/utils'
import { useSyncedForm, useUnsavedChangesPrompt } from '@/lib/form-sync'
import { ReadOnlyNotice } from '@/components/read-only-notice'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { ArrowLeft, Save, ShieldCheck, Plus, X, Users, Terminal, Clock } from 'lucide-react'
import { useState } from 'react'

export const Route = createFileRoute('/sudo-roles/$dn')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  component: SudoRoleDetailPage,
})

function SudoRoleDetailPage() {
  const { dn: encodedDN } = Route.useParams()
  const dn = decodeDN(encodedDN)
  const router = useRouter()
  const queryClient = useQueryClient()
  const { hasPermission } = useAuth()
  const canWrite = hasPermission('users:write')

  const { data: role, isLoading, error } = useQuery({
    queryKey: ['sudo-role', dn],
    queryFn: ({ signal }) => api.sudoRoles.get(dn, signal),
  })

  const [formData, setFormData] = useState({
    description: role?.description || '',
    sudoUser: role?.sudoUser || [] as string[],
    sudoHost: role?.sudoHost || [] as string[],
    sudoCommand: role?.sudoCommand || [] as string[],
    sudoRunAs: role?.sudoRunAs || [] as string[],
    sudoRunAsUser: role?.sudoRunAsUser || [] as string[],
    sudoRunAsGroup: role?.sudoRunAsGroup || [] as string[],
    sudoOption: role?.sudoOption || [] as string[],
    sudoOrder: role?.sudoOrder || 0,
    sudoNotBefore: role?.sudoNotBefore || '',
    sudoNotAfter: role?.sudoNotAfter || '',
  })

  // Input fields for adding new values
  const [newUser, setNewUser] = useState('')
  const [newHost, setNewHost] = useState('')
  const [newCommand, setNewCommand] = useState('')
  const [newRunAs, setNewRunAs] = useState('')
  const [newRunAsUser, setNewRunAsUser] = useState('')
  const [newRunAsGroup, setNewRunAsGroup] = useState('')
  const [newOption, setNewOption] = useState('')

  const updateMutation = useMutation({
    mutationFn: () => api.sudoRoles.update(dn, formData),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      queryClient.invalidateQueries({ queryKey: ['sudo-role', dn] })
      toast.success('Sudo role updated successfully')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  // Populate the form from server data once it loads (and re-sync on refetch
  // while pristine), and warn before navigating away with unsaved edits.
  const serverForm = role && {
    description: role.description || '',
    sudoUser: role.sudoUser || [],
    sudoHost: role.sudoHost || [],
    sudoCommand: role.sudoCommand || [],
    sudoRunAs: role.sudoRunAs || [],
    sudoRunAsUser: role.sudoRunAsUser || [],
    sudoRunAsGroup: role.sudoRunAsGroup || [],
    sudoOption: role.sudoOption || [],
    sudoOrder: role.sudoOrder || 0,
    sudoNotBefore: role.sudoNotBefore || '',
    sudoNotAfter: role.sudoNotAfter || '',
  }
  const isDirty = useSyncedForm(serverForm || undefined, formData, setFormData)
  useUnsavedChangesPrompt(isDirty)

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    updateMutation.mutate()
  }

  const addToArray = (field: keyof typeof formData, value: string, setter: (v: string) => void) => {
    if (value.trim() && Array.isArray(formData[field])) {
      const arr = formData[field] as string[]
      if (!arr.includes(value.trim())) {
        setFormData({ ...formData, [field]: [...arr, value.trim()] })
      }
      setter('')
    }
  }

  const removeFromArray = (field: keyof typeof formData, value: string) => {
    if (Array.isArray(formData[field])) {
      setFormData({
        ...formData,
        [field]: (formData[field] as string[]).filter(v => v !== value)
      })
    }
  }

  if (isLoading) {
    return (
      <InlineSpinner />
    )
  }

  if (error) {
    return (
      <div className="p-4 text-destructive bg-destructive/10 rounded-md">
        Failed to load sudo role: {error.message}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" aria-label="Back to sudo roles" onClick={() => router.navigate({ to: '/sudo-roles' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <ShieldCheck className="h-8 w-8 text-muted-foreground" />
        <div className="flex-1">
          <h1 className="text-2xl font-bold">{role?.cn}</h1>
          <p className="text-sm text-muted-foreground">{role?.dn}</p>
        </div>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        {!canWrite && <ReadOnlyNotice />}
        {updateMutation.error && (
          <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
            {updateMutation.error.message}
          </div>
        )}

        {updateMutation.isSuccess && (
          <div className="p-3 text-sm text-green-700 bg-green-100 dark:text-green-300 dark:bg-green-950 rounded-md">
            Sudo role updated successfully
          </div>
        )}

        <div className="grid gap-4 md:grid-cols-2">
          {/* Basic Info */}
          <Card>
            <CardHeader>
              <CardTitle>Basic Information</CardTitle>
              <CardDescription>General sudo role settings</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="cn">Name (CN)</Label>
                <Input id="cn" value={role?.cn || ''} disabled />
              </div>

              <div className="space-y-2">
                <Label htmlFor="description">Description</Label>
                <Input
                  id="description"
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                  disabled={!canWrite}
                  placeholder="Optional description"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="sudoOrder">Order (Priority)</Label>
                <Input
                  id="sudoOrder"
                  type="number"
                  value={formData.sudoOrder || ''}
                  onChange={(e) => setFormData({ ...formData, sudoOrder: parseInt(e.target.value) || 0 })}
                  disabled={!canWrite}
                  placeholder="0"
                />
                <p className="text-xs text-muted-foreground">
                  Higher values have higher priority when multiple rules match
                </p>
              </div>
            </CardContent>
          </Card>

          {/* Time Restrictions */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Clock className="h-5 w-5" />
                Time Restrictions
              </CardTitle>
              <CardDescription>When this sudo rule is valid</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="sudoNotBefore">Not Before</Label>
                <Input
                  id="sudoNotBefore"
                  value={formData.sudoNotBefore}
                  onChange={(e) => setFormData({ ...formData, sudoNotBefore: e.target.value })}
                  disabled={!canWrite}
                  placeholder="YYYYMMDDHHmmssZ (e.g., 20240101000000Z)"
                />
                <p className="text-xs text-muted-foreground">
                  Generalized time format: YYYYMMDDHHmmssZ
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="sudoNotAfter">Not After</Label>
                <Input
                  id="sudoNotAfter"
                  value={formData.sudoNotAfter}
                  onChange={(e) => setFormData({ ...formData, sudoNotAfter: e.target.value })}
                  disabled={!canWrite}
                  placeholder="YYYYMMDDHHmmssZ (e.g., 20251231235959Z)"
                />
              </div>
            </CardContent>
          </Card>

          {/* Users */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Users className="h-5 w-5" />
                Users ({formData.sudoUser.length})
              </CardTitle>
              <CardDescription>Who can run these commands</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                values={formData.sudoUser}
                onRemove={(v) => removeFromArray('sudoUser', v)}
                canWrite={canWrite}
              />
              {canWrite && (
                <div className="flex gap-2">
                  <Input
                    value={newUser}
                    onChange={(e) => setNewUser(e.target.value)}
                    placeholder="Username or ALL"
                    onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), addToArray('sudoUser', newUser, setNewUser))}
                  />
                  <Button type="button" variant="outline" onClick={() => addToArray('sudoUser', newUser, setNewUser)}>
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Hosts */}
          <Card>
            <CardHeader>
              <CardTitle>Hosts ({formData.sudoHost.length})</CardTitle>
              <CardDescription>Where commands can be run</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                values={formData.sudoHost}
                onRemove={(v) => removeFromArray('sudoHost', v)}
                canWrite={canWrite}
              />
              {canWrite && (
                <div className="flex gap-2">
                  <Input
                    value={newHost}
                    onChange={(e) => setNewHost(e.target.value)}
                    placeholder="Hostname or ALL"
                    onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), addToArray('sudoHost', newHost, setNewHost))}
                  />
                  <Button type="button" variant="outline" onClick={() => addToArray('sudoHost', newHost, setNewHost)}>
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Commands */}
          <Card className="md:col-span-2">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Terminal className="h-5 w-5" />
                Commands ({formData.sudoCommand.length})
              </CardTitle>
              <CardDescription>What commands can be run (use ALL for all commands)</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                values={formData.sudoCommand}
                onRemove={(v) => removeFromArray('sudoCommand', v)}
                canWrite={canWrite}
                mono
              />
              {canWrite && (
                <div className="flex gap-2">
                  <Input
                    value={newCommand}
                    onChange={(e) => setNewCommand(e.target.value)}
                    placeholder="/usr/bin/command or ALL"
                    className="font-mono"
                    onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), addToArray('sudoCommand', newCommand, setNewCommand))}
                  />
                  <Button type="button" variant="outline" onClick={() => addToArray('sudoCommand', newCommand, setNewCommand)}>
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Run As (deprecated) */}
          <Card>
            <CardHeader>
              <CardTitle>Run As (Deprecated) ({formData.sudoRunAs.length})</CardTitle>
              <CardDescription>Legacy field - use Run As User instead</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                values={formData.sudoRunAs}
                onRemove={(v) => removeFromArray('sudoRunAs', v)}
                canWrite={canWrite}
              />
              {canWrite && (
                <div className="flex gap-2">
                  <Input
                    value={newRunAs}
                    onChange={(e) => setNewRunAs(e.target.value)}
                    placeholder="Username"
                    onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), addToArray('sudoRunAs', newRunAs, setNewRunAs))}
                  />
                  <Button type="button" variant="outline" onClick={() => addToArray('sudoRunAs', newRunAs, setNewRunAs)}>
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Run As User */}
          <Card>
            <CardHeader>
              <CardTitle>Run As User ({formData.sudoRunAsUser.length})</CardTitle>
              <CardDescription>Users to impersonate</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                values={formData.sudoRunAsUser}
                onRemove={(v) => removeFromArray('sudoRunAsUser', v)}
                canWrite={canWrite}
              />
              {canWrite && (
                <div className="flex gap-2">
                  <Input
                    value={newRunAsUser}
                    onChange={(e) => setNewRunAsUser(e.target.value)}
                    placeholder="Username or ALL"
                    onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), addToArray('sudoRunAsUser', newRunAsUser, setNewRunAsUser))}
                  />
                  <Button type="button" variant="outline" onClick={() => addToArray('sudoRunAsUser', newRunAsUser, setNewRunAsUser)}>
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Run As Group */}
          <Card>
            <CardHeader>
              <CardTitle>Run As Group ({formData.sudoRunAsGroup.length})</CardTitle>
              <CardDescription>Groups to impersonate</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                values={formData.sudoRunAsGroup}
                onRemove={(v) => removeFromArray('sudoRunAsGroup', v)}
                canWrite={canWrite}
              />
              {canWrite && (
                <div className="flex gap-2">
                  <Input
                    value={newRunAsGroup}
                    onChange={(e) => setNewRunAsGroup(e.target.value)}
                    placeholder="Group name or ALL"
                    onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), addToArray('sudoRunAsGroup', newRunAsGroup, setNewRunAsGroup))}
                  />
                  <Button type="button" variant="outline" onClick={() => addToArray('sudoRunAsGroup', newRunAsGroup, setNewRunAsGroup)}>
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Options */}
          <Card>
            <CardHeader>
              <CardTitle>Options ({formData.sudoOption.length})</CardTitle>
              <CardDescription>Sudo options (e.g., !authenticate, env_keep)</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                values={formData.sudoOption}
                onRemove={(v) => removeFromArray('sudoOption', v)}
                canWrite={canWrite}
                mono
              />
              {canWrite && (
                <div className="flex gap-2">
                  <Input
                    value={newOption}
                    onChange={(e) => setNewOption(e.target.value)}
                    placeholder="!authenticate, env_keep, etc."
                    className="font-mono"
                    onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), addToArray('sudoOption', newOption, setNewOption))}
                  />
                  <Button type="button" variant="outline" onClick={() => addToArray('sudoOption', newOption, setNewOption)}>
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        {canWrite && (
          <div className="flex justify-end">
            <Button type="submit" disabled={updateMutation.isPending}>
              <Save className="h-4 w-4 mr-1" />
              {updateMutation.isPending ? 'Saving...' : 'Save Changes'}
            </Button>
          </div>
        )}
      </form>
    </div>
  )
}

function MultiValueField({
  values,
  onRemove,
  canWrite,
  mono = false,
}: {
  values: string[]
  onRemove: (value: string) => void
  canWrite: boolean
  mono?: boolean
}) {
  if (values.length === 0) {
    return <p className="text-sm text-muted-foreground">None configured</p>
  }

  return (
    <div className="flex flex-wrap gap-2">
      {values.map((value, index) => (
        <span
          key={index}
          className={`inline-flex items-center gap-1 px-2 py-1 rounded-md bg-muted text-sm ${mono ? 'font-mono' : ''}`}
        >
          {value}
          {canWrite && (
            <button
              type="button"
              onClick={() => onRemove(value)}
              className="hover:text-destructive"
            >
              <X className="h-3 w-3" />
            </button>
          )}
        </span>
      ))}
    </div>
  )
}
