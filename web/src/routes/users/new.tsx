import { createFileRoute, redirect, useRouter } from '@tanstack/react-router'
import { encodeDN } from '@/lib/utils'
import { BackToList } from '@/components/back-to-list'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { useServerInfo, requireModule } from '@/lib/use-server-info'
import { useAuth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { FieldRow } from '@/components/ui/field-row'
import { PasswordInput } from '@/components/ui/password-input'
import { PasswordStrength } from '@/components/ui/password-strength'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { DatePicker } from '@/components/ui/date-picker'
import { Select } from '@/components/ui/select'
import { UnsavedChangesGuard } from '@/components/unsaved-changes-guard'
import { PickerOverflow } from '@/components/ui/picker-overflow'
import { useCreationFormDirty } from '@/lib/form-sync'
import { usePickerSearch } from '@/lib/use-picker-search'
import { Save, Wand2, Users, X, CalendarClock } from 'lucide-react'
import { useState, useMemo } from 'react'

// The primary-group dropdown is not searched, so it takes a generous bound —
// still a bound, because an unbounded list stops at LDAP_SEARCH_SIZE_LIMIT
// without saying so.
const PRIMARY_GROUP_OPTIONS_LIMIT = 200

export const Route = createFileRoute('/users/new')({
  beforeLoad: async ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
    await requireModule(context.queryClient, 'users')
    if (!context.auth.hasPermission('users:write')) {
      throw redirect({ to: '/users' })
    }
  },
  component: NewUserPage,
})

function NewUserPage() {
  const router = useRouter()
  const { hasPermission } = useAuth()
  const queryClient = useQueryClient()

  const { data: nextIds } = useQuery({
    queryKey: ['nextIds'],
    queryFn: ({ signal }) => api.nextIds.get(signal),
  })

  // Two different needs, two different requests: the primary-group dropdown has
  // to list groups without a search term, while the membership box searches.
  const { data: allGroups } = useQuery({
    queryKey: ['groups', 'options'],
    queryFn: ({ signal }) =>
      api.groups.list({ sort: 'cn', order: 'asc', limit: PRIMARY_GROUP_OPTIONS_LIMIT }, signal),
  })

  // Scheduled expiry needs pwdEndTime in the directory (OpenLDAP >= 2.5); the
  // backend refuses to write one otherwise, so the field is not offered.
  const canExpire = useServerInfo()?.capabilities?.scheduledExpirySupported ?? false

  const [primaryGroup, setPrimaryGroup] = useState('')
  const [selectedGroups, setSelectedGroups] = useState<string[]>([])

  // Options for the primary group dropdown
  const primaryGroupOptions = useMemo(() => {
    const groups = allGroups?.data ?? []
    return [
      { value: '', label: 'None — create new group' },
      ...[...groups].sort((a, b) => a.cn.localeCompare(b.cn)).map(g => ({
        value: g.cn,
        label: `${g.cn} (${g.gidNumber})`,
      })),
    ]
  }, [allGroups?.data])

  const groupsNotListed = Math.max(0, (allGroups?.total ?? 0) - (allGroups?.data.length ?? 0))

  // The selected primary group's gidNumber, or 0 when the account gets a new
  // group of its own: the server allocates that one at write time, like the
  // uidNumber, rather than taking a nextGid read when the page loaded.
  const resolvedGidNumber = useMemo(() => {
    if (primaryGroup) {
      const group = allGroups?.data?.find(g => g.cn === primaryGroup)
      if (group) return group.gidNumber
    }
    return 0
  }, [primaryGroup, allGroups?.data])

  // Membership candidates come from the directory. Headroom covers the groups
  // already picked, which are filtered out below.
  const groupPicker = usePickerSearch(['groups', 'picker', 'new-user'], api.groups.list, {
    sort: 'cn',
    headroom: selectedGroups.length,
  })

  const availableGroups = useMemo(
    () => groupPicker.items.filter(g => !selectedGroups.includes(g.cn)),
    [groupPicker.items, selectedGroups],
  )

  const [formData, setFormData] = useState({
    uid: '',
    givenName: '',
    sn: '',
    cn: '',
    displayName: '',
    mail: '',
    telephoneNumber: '',
    title: '',
    departmentNumber: '',
    o: '',
    employeeType: '',
    uidNumber: '',
    homeDirectory: '',
    loginShell: '/bin/bash',
    description: '',
    expirationDate: '',
    password: '',
  })

  const createMutation = useMutation({
    mutationFn: () => api.users.create({
      uid: formData.uid,
      givenName: formData.givenName,
      sn: formData.sn,
      cn: formData.cn || `${formData.givenName} ${formData.sn}`,
      displayName: formData.displayName || `${formData.givenName} ${formData.sn}`,
      mail: formData.mail || undefined,
      telephoneNumber: formData.telephoneNumber || undefined,
      title: formData.title || undefined,
      departmentNumber: formData.departmentNumber || undefined,
      o: formData.o || undefined,
      employeeType: formData.employeeType || undefined,
      // Left as the server's job when untouched: 0 means "allocate one". Filling
      // it in here replayed a number read when the page loaded, which two people
      // creating accounts at once would both have sent.
      uidNumber: formData.uidNumber === '' ? 0 : parseInt(formData.uidNumber),
      gidNumber: resolvedGidNumber,
      homeDirectory: formData.homeDirectory || `/home/${formData.uid}`,
      loginShell: formData.loginShell,
      description: formData.description || undefined,
      password: formData.password || undefined,
      groups: selectedGroups.length > 0 ? selectedGroups : undefined,
      createPrimaryGroup: !primaryGroup || undefined,
      expirationDate: formData.expirationDate || undefined,
    }),
    onSuccess: (user) => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      toast.success('User created successfully')
      // To the new entry, not the list: the next step is almost always on it
      // (groups, keys, members), and the list would mean searching for it again.
      router.navigate({ to: '/users/$dn', params: { dn: encodeDN(user.dn) } })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createMutation.mutate()
  }

  // Group selections are part of the form, so a half-filled membership list
  // counts as unsaved work like any typed field.
  const isDirty = useCreationFormDirty(
    { ...formData, primaryGroup, selectedGroups },
    createMutation.isPending || createMutation.isSuccess,
  )

  if (!hasPermission('users:write')) {
    return null
  }

  return (
    <div className="space-y-4">
      <UnsavedChangesGuard when={isDirty} />
      <div className="flex items-center gap-4">
        <BackToList to="/users" label="Back to users" />
        <h1 className="text-2xl font-bold">Create New User</h1>
      </div>

      <form onSubmit={handleSubmit}>
        <Card>
          <CardHeader>
            <CardTitle>User Information</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="uid">Username (UID) *</Label>
              <Input
                id="uid"
                value={formData.uid}
                onChange={(e) => setFormData({ ...formData, uid: e.target.value })}
                required
                autoFocus
              />
            </div>

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="givenName">First Name *</Label>
                <Input
                  id="givenName"
                  value={formData.givenName}
                  onChange={(e) => setFormData({ ...formData, givenName: e.target.value })}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="sn">Last Name *</Label>
                <Input
                  id="sn"
                  value={formData.sn}
                  onChange={(e) => setFormData({ ...formData, sn: e.target.value })}
                  required
                />
              </div>
            </FieldRow>

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="cn">Common Name (CN)</Label>
                <Input
                  id="cn"
                  value={formData.cn}
                  onChange={(e) => setFormData({ ...formData, cn: e.target.value })}
                  placeholder="Auto-generated if empty"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="displayName">Display Name</Label>
                <Input
                  id="displayName"
                  value={formData.displayName}
                  onChange={(e) => setFormData({ ...formData, displayName: e.target.value })}
                  placeholder="Auto-generated if empty"
                />
              </div>
            </FieldRow>

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="uidNumber">UID Number *</Label>
                <div className="flex gap-2">
                  <Input
                    id="uidNumber"
                    type="number"
                    value={formData.uidNumber || nextIds?.nextUid?.toString() || ''}
                    onChange={(e) => setFormData({ ...formData, uidNumber: e.target.value })}
                    required
                    min={nextIds?.minUid}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    aria-label="Use the next free UID"
                    title="Use the next free UID"
                    // Back to automatic, rather than copying nextUid into the field:
                    // a copied number is the stale one the server would then have
                    // to refuse if someone took it in the meantime.
                    onClick={() => setFormData({ ...formData, uidNumber: '' })}
                    disabled={formData.uidNumber === ''}
                  >
                    <Wand2 className="h-4 w-4" />
                  </Button>
                </div>
                {nextIds && (
                  <p className="text-xs text-muted-foreground">
                    Min: {nextIds.minUid}. Leave it as it is and the next free number is picked
                    when the account is created, so two people creating accounts at once cannot
                    both get {nextIds.nextUid}.
                  </p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="primaryGroup" className="flex items-center gap-2">
                  <Users className="h-4 w-4" />
                  Primary Group
                </Label>
                <Select
                  id="primaryGroup"
                  value={primaryGroup}
                  onChange={(e) => setPrimaryGroup(e.target.value)}
                  options={primaryGroupOptions}
                />
                <p className="text-xs text-muted-foreground">
                  If left empty, a new group will be created with the user's UID as name.
                </p>
                {groupsNotListed > 0 && (
                  <p className="text-xs text-muted-foreground">
                    {groupsNotListed} further {groupsNotListed === 1 ? 'group is' : 'groups are'} not
                    listed here — search for them under Additional Groups instead.
                  </p>
                )}
              </div>
            </FieldRow>

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="mail">Email</Label>
                <Input
                  id="mail"
                  type="email"
                  value={formData.mail}
                  onChange={(e) => setFormData({ ...formData, mail: e.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="telephoneNumber">Phone</Label>
                <Input
                  id="telephoneNumber"
                  value={formData.telephoneNumber}
                  onChange={(e) => setFormData({ ...formData, telephoneNumber: e.target.value })}
                />
              </div>
            </FieldRow>

            <FieldRow cols={4}>
              <div className="space-y-2">
                <Label htmlFor="title">Title</Label>
                <Input
                  id="title"
                  value={formData.title}
                  onChange={(e) => setFormData({ ...formData, title: e.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="departmentNumber">Department</Label>
                <Input
                  id="departmentNumber"
                  value={formData.departmentNumber}
                  onChange={(e) => setFormData({ ...formData, departmentNumber: e.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="o">Organization</Label>
                <Input
                  id="o"
                  value={formData.o}
                  onChange={(e) => setFormData({ ...formData, o: e.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="employeeType">Employee Type</Label>
                <Input
                  id="employeeType"
                  value={formData.employeeType}
                  onChange={(e) => setFormData({ ...formData, employeeType: e.target.value })}
                />
              </div>
            </FieldRow>

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="homeDirectory">Home Directory</Label>
                <Input
                  id="homeDirectory"
                  value={formData.homeDirectory}
                  onChange={(e) => setFormData({ ...formData, homeDirectory: e.target.value })}
                  placeholder={`/home/${formData.uid || 'username'}`}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="loginShell">Login Shell</Label>
                <Input
                  id="loginShell"
                  value={formData.loginShell}
                  onChange={(e) => setFormData({ ...formData, loginShell: e.target.value })}
                />
              </div>
            </FieldRow>

            <div className="space-y-2">
              <Label htmlFor="password">Initial password</Label>
              <PasswordInput
                id="password"
                generate
                value={formData.password}
                onValueChange={(password) => setFormData({ ...formData, password })}
                autoComplete="new-password"
                placeholder="Leave empty to set one later"
              />
              <PasswordStrength password={formData.password} />
              {!formData.password && (
                <p className="text-xs text-muted-foreground">
                  Without a password the account cannot sign in. You can set one later, or send the
                  user a reset link from their Security tab.
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

            {canExpire && (
              <div className="space-y-2">
                <Label htmlFor="expirationDate" className="flex items-center gap-2">
                  <CalendarClock className="h-4 w-4" />
                  Account Expiration Date
                </Label>
                <DatePicker
                  id="expirationDate"
                  value={formData.expirationDate}
                  onChange={(value) => setFormData({ ...formData, expirationDate: value })}
                  min={new Date().toISOString().split('T')[0]} // Minimum is today
                />
                <p className="text-xs text-muted-foreground">
                  Optional. The directory refuses this account from that date on.
                </p>
              </div>
            )}

            <div className="space-y-2">
              <Label className="flex items-center gap-2">
                <Users className="h-4 w-4" />
                Group Memberships
              </Label>
              {selectedGroups.length > 0 && (
                <div className="flex flex-wrap gap-2 mb-2">
                  {selectedGroups.map(groupCn => (
                    <span
                      key={groupCn}
                      className="inline-flex items-center gap-1 px-2 py-1 bg-primary text-primary-foreground text-sm rounded-md"
                    >
                      {groupCn}
                      <button
                        type="button"
                        onClick={() => setSelectedGroups(selectedGroups.filter(g => g !== groupCn))}
                        aria-label={`Remove ${groupCn}`}
                        // 24px of target around a 12px glyph, pulled into the
                        // chip's padding so the chip keeps its height.
                        className="-my-1 -mr-1.5 rounded p-1.5 hover:bg-primary-foreground/20"
                      >
                        <X className="h-3 w-3" aria-hidden="true" />
                      </button>
                    </span>
                  ))}
                </div>
              )}
              <div className="relative">
                <Input
                  placeholder="Search groups to add..."
                  value={groupPicker.input}
                  onChange={(e) => groupPicker.setInput(e.target.value)}
                />
              </div>
              {groupPicker.input && (
                <>
                  <div className="max-h-40 overflow-y-auto border rounded-md bg-background">
                    {availableGroups.length > 0 ? (
                      availableGroups.map((group) => (
                        <button
                          key={group.dn}
                          type="button"
                          onClick={() => {
                            setSelectedGroups([...selectedGroups, group.cn])
                            groupPicker.reset()
                          }}
                          className="w-full flex items-center gap-2 p-2 text-left hover:bg-muted transition-colors"
                        >
                          <Users className="h-4 w-4 text-muted-foreground" />
                          <span className="font-medium">{group.cn}</span>
                        </button>
                      ))
                    ) : (
                      <p className="text-sm text-muted-foreground text-center py-2">
                        {groupPicker.isFetching ? 'Searching...' : 'No matching groups'}
                      </p>
                    )}
                  </div>
                  <PickerOverflow count={groupPicker.overflow} />
                </>
              )}
              <p className="text-xs text-muted-foreground">
                Additional groups the user will be added to after creation.
              </p>
            </div>

            <div className="flex justify-end pt-4">
              <Button type="submit" disabled={createMutation.isPending}>
                <Save className="h-4 w-4 mr-1" />
                {createMutation.isPending ? 'Creating...' : 'Create User'}
              </Button>
            </div>
          </CardContent>
        </Card>
      </form>
    </div>
  )
}
