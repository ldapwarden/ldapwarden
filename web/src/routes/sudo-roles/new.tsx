import { createFileRoute, redirect, useRouter } from '@tanstack/react-router'
import { BackToList } from '@/components/back-to-list'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { encodeDN } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { MultiValueField } from '@/components/ui/multi-value-field'
import { NetgroupPicker, NetgroupResolutionNotice } from '@/components/netgroup-picker'
import { NetgroupUsageWarnings, NetgroupContents } from '@/components/netgroup-usage'
import { Callout } from '@/components/ui/callout'
import { Save, ShieldCheck, Plus, Users, Terminal, Clock } from 'lucide-react'
import { UnsavedChangesGuard } from '@/components/unsaved-changes-guard'
import { useCreationFormDirty } from '@/lib/form-sync'
import { useState } from 'react'

export const Route = createFileRoute('/sudo-roles/new')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
    if (!context.auth.hasPermission('users:write')) {
      throw redirect({ to: '/sudo-roles' })
    }
  },
  component: NewSudoRolePage,
})

function NewSudoRolePage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { hasPermission } = useAuth()
  const canWrite = hasPermission('users:write')

  const [formData, setFormData] = useState({
    cn: '',
    description: '',
    sudoUser: [] as string[],
    sudoHost: [] as string[],
    sudoCommand: [] as string[],
    sudoRunAs: [] as string[],
    sudoRunAsUser: [] as string[],
    sudoRunAsGroup: [] as string[],
    sudoOption: [] as string[],
    sudoOrder: 0,
    sudoNotBefore: '',
    sudoNotAfter: '',
  })

  // Input fields for adding new values
  const [newUser, setNewUser] = useState('')
  const [newHost, setNewHost] = useState('')
  const [newCommand, setNewCommand] = useState('')
  const [newRunAs, setNewRunAs] = useState('')
  const [newRunAsUser, setNewRunAsUser] = useState('')
  const [newRunAsGroup, setNewRunAsGroup] = useState('')
  const [newOption, setNewOption] = useState('')

  const createMutation = useMutation({
    mutationFn: () => api.sudoRoles.create(formData),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      toast.success('Sudo role created successfully')
      router.navigate({ to: '/sudo-roles/$dn', params: { dn: encodeDN(data.dn) } })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createMutation.mutate()
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

  // The picker hands over a finished value ("+ops"), so it does not go through
  // addToArray, which exists to drain a text input.
  // Shown only when the rule actually names one: the caveat is real but it is
  // noise on a rule that grants to plain usernames.
  const namesNetgroup =
    formData.sudoUser.some((v) => v.startsWith('+')) ||
    formData.sudoHost.some((v) => v.startsWith('+'))

  const addValue = (field: keyof typeof formData, value: string) => {
    const arr = formData[field] as string[]
    if (!arr.includes(value)) {
      setFormData({ ...formData, [field]: [...arr, value] })
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

  const isDirty = useCreationFormDirty(
    formData,
    createMutation.isPending || createMutation.isSuccess,
  )

  if (!canWrite) {
    return (
      <Callout variant="error">
        You don't have permission to create sudo roles.
      </Callout>
    )
  }

  return (
    <div className="space-y-4">
      <UnsavedChangesGuard when={isDirty} />
      <div className="flex items-center gap-4">
        <BackToList to="/sudo-roles" label="Back to sudo roles" />
        <ShieldCheck className="h-8 w-8 text-muted-foreground" />
        <div className="flex-1">
          <h1 className="text-2xl font-bold">New Sudo Role</h1>
          <p className="text-sm text-muted-foreground">Create a new sudo role with all schema fields</p>
        </div>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="grid gap-4 md:grid-cols-2">
          {/* Basic Info */}
          <Card>
            <CardHeader>
              <CardTitle>Basic Information</CardTitle>
              <CardDescription>Required fields and general settings</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="cn">Name (CN) *</Label>
                <Input
                  id="cn"
                  value={formData.cn}
                  onChange={(e) => setFormData({ ...formData, cn: e.target.value })}
                  placeholder="my-sudo-role"
                  required
                />
                <p className="text-xs text-muted-foreground">
                  Unique identifier for this sudo role
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="description">Description</Label>
                <Input
                  id="description"
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
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
              <CardDescription>When this sudo rule is valid (optional)</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="sudoNotBefore">Not Before</Label>
                <Input
                  id="sudoNotBefore"
                  value={formData.sudoNotBefore}
                  onChange={(e) => setFormData({ ...formData, sudoNotBefore: e.target.value })}
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
              <CardDescription>Who can run these commands (use ALL for everyone)</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                empty="None added yet"
                values={formData.sudoUser}
                onRemove={(v) => removeFromArray('sudoUser', v)}
              />
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
                <NetgroupPicker
                  side="sudoUser"
                  existing={formData.sudoUser}
                  onPick={(value) => addValue('sudoUser', value)}
                />
              </div>
            </CardContent>
          </Card>

          {/* Hosts */}
          <Card>
            <CardHeader>
              <CardTitle>Hosts ({formData.sudoHost.length})</CardTitle>
              <CardDescription>Where commands can be run (use ALL for any host)</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                empty="None added yet"
                values={formData.sudoHost}
                onRemove={(v) => removeFromArray('sudoHost', v)}
              />
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
                <NetgroupPicker
                  side="sudoHost"
                  existing={formData.sudoHost}
                  onPick={(value) => addValue('sudoHost', value)}
                />
              </div>
            </CardContent>
          </Card>

          {namesNetgroup && (
            <div className="space-y-3">
              <NetgroupUsageWarnings sudoUser={formData.sudoUser} sudoHost={formData.sudoHost} />
              <NetgroupContents sudoUser={formData.sudoUser} sudoHost={formData.sudoHost} />
              <NetgroupResolutionNotice />
            </div>
          )}

          {/* Commands */}
          <Card className="md:col-span-2">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Terminal className="h-5 w-5" />
                Commands ({formData.sudoCommand.length})
              </CardTitle>
              <CardDescription>What commands can be run (use ALL for all commands, or specify full paths)</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                empty="None added yet"
                values={formData.sudoCommand}
                onRemove={(v) => removeFromArray('sudoCommand', v)}
                mono
              />
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
            </CardContent>
          </Card>

          {/* Run As User */}
          <Card>
            <CardHeader>
              <CardTitle>Run As User ({formData.sudoRunAsUser.length})</CardTitle>
              <CardDescription>Users to impersonate (use ALL or root)</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                empty="None added yet"
                values={formData.sudoRunAsUser}
                onRemove={(v) => removeFromArray('sudoRunAsUser', v)}
              />
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
                empty="None added yet"
                values={formData.sudoRunAsGroup}
                onRemove={(v) => removeFromArray('sudoRunAsGroup', v)}
              />
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
            </CardContent>
          </Card>

          {/* Run As (deprecated) */}
          <Card>
            <CardHeader>
              <CardTitle>Run As (Deprecated) ({formData.sudoRunAs.length})</CardTitle>
              <CardDescription>Legacy field - prefer using Run As User instead</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                empty="None added yet"
                values={formData.sudoRunAs}
                onRemove={(v) => removeFromArray('sudoRunAs', v)}
              />
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
            </CardContent>
          </Card>

          {/* Options */}
          <Card>
            <CardHeader>
              <CardTitle>Options ({formData.sudoOption.length})</CardTitle>
              <CardDescription>Sudo options (e.g., !authenticate to skip password)</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <MultiValueField
                empty="None added yet"
                values={formData.sudoOption}
                onRemove={(v) => removeFromArray('sudoOption', v)}
                mono
              />
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
              <p className="text-xs text-muted-foreground">
                Common options: !authenticate (no password), env_keep (preserve env vars), setenv (allow setting env)
              </p>
            </CardContent>
          </Card>
        </div>

        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={() => router.navigate({ to: '/sudo-roles' })}>
            Cancel
          </Button>
          <Button type="submit" disabled={!formData.cn || createMutation.isPending}>
            <Save className="h-4 w-4 mr-1" />
            {createMutation.isPending ? 'Creating...' : 'Create Sudo Role'}
          </Button>
        </div>
      </form>
    </div>
  )
}
