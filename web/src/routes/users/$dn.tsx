import { DetailSkeleton } from '@/components/ui/skeleton'
import { DNLine } from '@/components/dn-line'
import { DetailLoadError } from '@/components/detail-load-error'
import { FormActions } from '@/components/form-actions'
import { BackToList } from '@/components/back-to-list'
import { PosixIdsChange } from '@/components/posix-ids-change'
import { createFileRoute, redirect, useRouter, Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { showOutcome } from '@/lib/outcome'
import { api, userPhotoUrl, type User as UserDetail } from '@/lib/api'
import { useEnabledModules, useEnabledObjects, useServerInfo, requireModule } from '@/lib/use-server-info'
import { useNetgroupsEnabled } from '@/lib/use-server-info'
import { useAuth } from '@/lib/auth'
import { decodeDN, encodeDN, expiryInstantTitle, formatExpiryDate, formatLdapTimestamp, isLdapTimestampInFuture, ldapTimestampToDateString, rdnValue, sameDn } from '@/lib/utils'
import { hasLapsedLock, isLocked, lockedAt, lockoutLiftsAt } from '@/lib/account'
import { PickerOverflow } from '@/components/ui/picker-overflow'
import { usePickerSearch } from '@/lib/use-picker-search'

// A password-policy dropdown is not searched, so it takes a generous bound:
// enough that no real directory reaches it, small enough to stay a dropdown.
const POLICY_OPTIONS_LIMIT = 200
import { UnsavedChangesGuard } from '@/components/unsaved-changes-guard'
import { useSyncedForm } from '@/lib/form-sync'
import { ReadOnlyNotice } from '@/components/read-only-notice'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { FieldRow } from '@/components/ui/field-row'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Avatar } from '@/components/ui/avatar'
import { ImageDropTarget } from '@/components/ui/image-drop-target'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Select } from '@/components/ui/select'
import { Trash2, Lock, Unlock, Plus, X, Key, Users, Shield, User, Terminal, UserPlus, UserMinus, ShieldCheck, Mail, AlertTriangle, CalendarClock, Copy, Network } from 'lucide-react'
import { DatePicker } from '@/components/ui/date-picker'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Callout } from '@/components/ui/callout'
import { PasswordStrength } from '@/components/ui/password-strength'
import { PasswordRequirements } from '@/components/ui/password-requirements'
import { AccountStatus } from '@/components/account-status'
import { PasswordInput } from '@/components/ui/password-input'
import { useState, useMemo } from 'react'

// Tabs this page can show. Which ones are actually rendered depends on the
// configured object classes and modules; an unknown or disabled tab in the URL
// falls back to the first available one.
const TABS = ['identity', 'posix', 'shadow', 'samba', 'ssh-keys', 'sudo', 'security'] as const
type Tab = (typeof TABS)[number]

export const Route = createFileRoute('/users/$dn')({
  beforeLoad: async ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
    await requireModule(context.queryClient, 'users')
  },
  validateSearch: (raw: Record<string, unknown>): { tab?: Tab } => ({
    tab: typeof raw.tab === 'string' && (TABS as readonly string[]).includes(raw.tab)
      ? (raw.tab as Tab)
      : undefined,
  }),
  component: UserDetailPage,
})

function UserDetailPage() {
  const { dn: encodedDN } = Route.useParams()
  const dn = decodeDN(encodedDN)
  const { hasPermission, session } = useAuth()
  const canWrite = hasPermission('users:write')
  const canDelete = hasPermission('users:delete')
  // Locking, deleting, clearing the password of or renaming the account you are
  // signed in with locks you out of the app — and if you are the only admin, the
  // way back is ldapmodify. The API refuses these outright (see
  // refuseSelfTarget); the UI explains rather than offering them.
  const isSelf = sameDn(session?.userDn, dn)

  const { data: user, isLoading, error } = useQuery({
    queryKey: ['user', dn],
    queryFn: ({ signal }) => api.users.get(dn, signal),
  })

  const { data: userGroups } = useQuery({
    queryKey: ['user', dn, 'groups'],
    queryFn: ({ signal }) => api.users.getGroups(dn, signal),
  })

  const serverInfo = useServerInfo()

  // From /server-info rather than /admin/config, which takes settings:read: a
  // read-only session could not read it and silently fell back to this list
  // instead of the deployment's own, so it saw the wrong tabs.
  const enabledObjects = useEnabledObjects('users', [
    'inetorgperson',
    'posixaccount',
    'ldappublickey',
  ])

  const enabledModules = new Set(useEnabledModules().map((m) => m.toLowerCase()))

  const showIdentityTab = enabledObjects.has('inetorgperson')
  const showPosixTab = enabledObjects.has('posixaccount')
  const showSSHKeysTab = enabledObjects.has('ldappublickey')
  const showSambaTab = enabledObjects.has('sambasamaccount')
  const showShadowTab = enabledObjects.has('shadowaccount')
  const showSudoTab = enabledModules.has('sudo')
  const showPoliciesModule = enabledModules.has('policies')
  // Scheduled expiry needs pwdEndTime in the directory (OpenLDAP >= 2.5). The
  // backend refuses to write one otherwise, so the controls stay hidden rather
  // than failing on submit.
  const canExpire = serverInfo?.capabilities?.scheduledExpirySupported ?? false
  // Only ever "not-enforced" on evidence — see PolicyEnforcementSchema. Anything
  // else, including the usual "unknown", stays silent rather than crying wolf.
  const policyEnforcement = serverInfo?.capabilities?.policyEnforcement ?? 'unknown'

  // Check which objectClasses the user actually has
  const userObjectClasses = useMemo(() => {
    const classes = new Set((user?.objectClasses || []).map(oc => oc.toLowerCase()))
    return classes
  }, [user?.objectClasses])

  const hasSambaSamAccount = userObjectClasses.has('sambasamaccount')
  const hasShadowAccount = userObjectClasses.has('shadowaccount')

  // Compute default tab from config
  const defaultTab = useMemo(() => {
    if (showIdentityTab) return 'identity'
    if (showPosixTab) return 'posix'
    if (showShadowTab) return 'shadow'
    if (showSambaTab) return 'samba'
    if (showSSHKeysTab) return 'ssh-keys'
    if (showSudoTab) return 'sudo'
    return 'security'
  }, [showIdentityTab, showPosixTab, showShadowTab, showSambaTab, showSSHKeysTab, showSudoTab])

  // The active tab lives in the URL. `replace` keeps tab switching out of the
  // history stack, so Back leaves the page rather than walking tabs.
  const navigate = Route.useNavigate()
  const { tab } = Route.useSearch()
  const activeTab = tab ?? defaultTab
  const setActiveTab = (next: string) => {
    navigate({ search: { tab: next as Tab }, replace: true })
  }

  if (isLoading) {
    return <DetailSkeleton cards={2} />
  }

  if (error) {
    return <DetailLoadError error={error} noun="user" list="/users" listLabel="users" />
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <BackToList to="/users" label="Back to users" />
        <Avatar
          src={user?.jpegPhoto}
          fallback={user?.displayName || user?.cn}
          size="lg"
        />
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold">{user?.displayName || user?.cn}</h1>
            {user && isLocked(user) && (
              <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-destructive/10 text-destructive">
                <Lock className="h-3 w-3 mr-1" />
                Locked
              </span>
            )}
          </div>
          <DNLine dn={user?.dn} />
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          {showIdentityTab && (
            <TabsTrigger value="identity">
              <User className="h-4 w-4 mr-1" />
              Identity
            </TabsTrigger>
          )}
          {showPosixTab && (
            <TabsTrigger value="posix">
              <Terminal className="h-4 w-4 mr-1" />
              POSIX
            </TabsTrigger>
          )}
          {showShadowTab && (
            <TabsTrigger value="shadow">
              <svg className="h-4 w-4 mr-1" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="10" />
                <path d="M12 2a7 7 0 0 0 0 14 7 7 0 0 0 0-14" fill="currentColor" opacity="0.3" />
                <path d="M12 6v6l4 2" />
              </svg>
              Shadow
            </TabsTrigger>
          )}
          {showSambaTab && (
            <TabsTrigger value="samba">
              <svg className="h-4 w-4 mr-1" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M4 22h14a2 2 0 0 0 2-2V7.5L14.5 2H6a2 2 0 0 0-2 2v4" />
                <polyline points="14 2 14 8 20 8" />
                <path d="M2 15h10" />
                <path d="m9 18 3-3-3-3" />
              </svg>
              Samba
            </TabsTrigger>
          )}
          {showSSHKeysTab && (
            <TabsTrigger value="ssh-keys">
              <Key className="h-4 w-4 mr-1" />
              SSH Keys
            </TabsTrigger>
          )}
          {showSudoTab && (
            <TabsTrigger value="sudo">
              <ShieldCheck className="h-4 w-4 mr-1" />
              Sudo
            </TabsTrigger>
          )}
          <TabsTrigger value="security">
            <Shield className="h-4 w-4 mr-1" />
            Security
          </TabsTrigger>
        </TabsList>

        {showIdentityTab && (
          <TabsContent value="identity">
            <IdentityTab user={user!} dn={dn} canWrite={canWrite} groups={userGroups?.data || []} />
          </TabsContent>
        )}

        {showPosixTab && (
          <TabsContent value="posix">
            <PosixTab user={user!} dn={dn} canWrite={canWrite} />
          </TabsContent>
        )}

        {showShadowTab && (
          <TabsContent value="shadow">
            <ShadowTab user={user!} dn={dn} canWrite={canWrite} hasObjectClass={hasShadowAccount} isSelf={isSelf} />
          </TabsContent>
        )}

        {showSambaTab && (
          <TabsContent value="samba">
            <SambaTab user={user!} dn={dn} canWrite={canWrite} hasObjectClass={hasSambaSamAccount} />
          </TabsContent>
        )}

        {showSSHKeysTab && (
          <TabsContent value="ssh-keys">
            <SSHKeysTab user={user!} dn={dn} canWrite={canWrite} />
          </TabsContent>
        )}

        {showSudoTab && (
          <TabsContent value="sudo">
            <SudoTab user={user!} dn={dn} canWrite={canWrite} />
          </TabsContent>
        )}

        <TabsContent value="security">
          <SecurityTab
            user={user!}
            dn={dn}
            canWrite={canWrite}
            canDelete={canDelete}
            showPoliciesModule={showPoliciesModule}
            canExpire={canExpire}
            policyEnforcement={policyEnforcement}
            isSelf={isSelf}
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// The editable inetOrgPerson fields, normalised so an absent attribute and an
// empty field compare equal. Both the dirty check and the save diff read it.
function identityForm(user: UserDetail) {
  return {
    givenName: user.givenName || '',
    sn: user.sn || '',
    cn: user.cn || '',
    displayName: user.displayName || '',
    mail: user.mail || '',
    telephoneNumber: user.telephoneNumber || '',
    title: user.title || '',
    departmentNumber: user.departmentNumber || '',
    o: user.o || '',
    employeeNumber: user.employeeNumber || '',
    employeeType: user.employeeType || '',
    initials: user.initials || '',
    manager: user.manager || '',
    description: user.description || '',
  }
}

function posixForm(user: UserDetail) {
  return {
    homeDirectory: user.homeDirectory || '',
    loginShell: user.loginShell || '',
    gecos: user.gecos || '',
  }
}

function sambaForm(user: UserDetail) {
  return {
    sambaSID: user.sambaSID || '',
    sambaPrimaryGroupSID: user.sambaPrimaryGroupSID || '',
    sambaAcctFlags: user.sambaAcctFlags || '',
    sambaHomePath: user.sambaHomePath || '',
    sambaHomeDrive: user.sambaHomeDrive || '',
    sambaLogonScript: user.sambaLogonScript || '',
    sambaProfilePath: user.sambaProfilePath || '',
    sambaDomainName: user.sambaDomainName || '',
  }
}

// Shadow attributes are integers in the directory but strings in the form, so
// an emptied field stays distinguishable from a zero.
function shadowForm(user: UserDetail) {
  return {
    shadowLastChange: user.shadowLastChange?.toString() || '',
    shadowMin: user.shadowMin?.toString() || '',
    shadowMax: user.shadowMax?.toString() || '',
    shadowWarning: user.shadowWarning?.toString() || '',
    shadowInactive: user.shadowInactive?.toString() || '',
    shadowExpire: user.shadowExpire?.toString() || '',
    shadowFlag: user.shadowFlag?.toString() || '',
  }
}

/** The fields of `form` that differ from `server`, for a partial update. */
function changedFields<T extends Record<string, string>>(form: T, server: T): Partial<T> {
  const changed: Partial<T> = {}
  for (const key of Object.keys(form) as Array<keyof T>) {
    if (form[key] !== server[key]) changed[key] = form[key]
  }
  return changed
}

// Identity Tab (inetOrgPerson)
function IdentityTab({ user, dn, canWrite, groups }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean, groups: Array<{ dn: string; cn: string; gidNumber: number }> }) {
  const queryClient = useQueryClient()
  const [addGroupOpen, setAddGroupOpen] = useState(false)
  const [selectedGroups, setSelectedGroups] = useState<string[]>([])
  // Group pending removal confirmation (drives one controlled dialog).
  const [confirmRemoveGroup, setConfirmRemoveGroup] = useState<{ dn: string; cn: string } | null>(null)
  const [managerDropdownOpen, setManagerDropdownOpen] = useState(false)

  // Both pickers search the directory instead of filtering a local copy of it.
  // Headroom covers the candidates dropped below (groups already joined, the
  // user being edited), so the list is not empty while matches remain.
  const groupPicker = usePickerSearch(['groups', 'picker', dn], api.groups.list, {
    sort: 'cn',
    headroom: groups.length,
    enabled: addGroupOpen,
  })
  const managerPicker = usePickerSearch(['users', 'picker', 'manager', dn], api.users.list, {
    sort: 'displayName',
    headroom: 1,
    enabled: managerDropdownOpen,
  })

  // Nobody can manage themselves.
  const availableManagers = useMemo(
    () => managerPicker.items.filter(u => u.dn !== dn),
    [managerPicker.items, dn],
  )

  // Groups the user is NOT a member of
  const availableGroups = useMemo(() => {
    const memberGroupDns = new Set(groups.map(g => g.dn))
    return groupPicker.items.filter(g => !memberGroupDns.has(g.dn))
  }, [groupPicker.items, groups])

  const addToGroupMutation = useMutation({
    mutationFn: async (groupDns: string[]) => {
      // Add to each group sequentially
      for (const groupDn of groupDns) {
        await api.groups.addMember(groupDn, user.uid)
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn, 'groups'] })
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      setAddGroupOpen(false)
      const count = selectedGroups.length
      setSelectedGroups([])
      groupPicker.reset()
      toast.success(`User added to ${count} group${count > 1 ? 's' : ''}`)
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const removeFromGroupMutation = useMutation({
    mutationFn: (groupDn: string) => api.groups.removeMember(groupDn, user.uid),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn, 'groups'] })
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      toast.success('User removed from group')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  // The photo is held apart from the synced form on purpose: useSyncedForm
  // compares by JSON, and a multi-MB base64 has no business being stringified
  // on every render. `photoChanged` carries its dirty state instead.
  const [photo, setPhoto] = useState(user.jpegPhoto || '')
  const [photoChanged, setPhotoChanged] = useState(false)

  const [formData, setFormData] = useState(() => identityForm(user))

  // Adopt the server's copy on refetch while the form is untouched, and report
  // unsaved edits. Comparing against a snapshot by hand (which is what this
  // replaced) left the form pinned to whatever the entry looked like when the
  // tab mounted: after a refetch — window focus is enough — it kept stale
  // values, claimed unsaved changes the operator had never made, and would
  // write the old copy back over the new one on save.
  const textFieldsDirty = useSyncedForm(identityForm(user), formData, setFormData)

  const handlePhotoFile = (file: File) => {
    // A dropped file skips the picker's `accept`, so the type is checked here.
    if (!file.type.startsWith('image/')) {
      toast.error('Please choose an image file.')
      return
    }
    // The API caps the request body at 10 MiB; base64 inflates by ~33%, so keep
    // the raw file comfortably under that.
    const maxBytes = 5 * 1024 * 1024
    if (file.size > maxBytes) {
      toast.error('Image is too large (max 5 MB).')
      return
    }

    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result as string | null
      const base64 = result?.split(',')[1]
      if (!base64) {
        toast.error('Could not read the selected image.')
        return
      }
      setPhoto(base64)
      setPhotoChanged(true)
    }
    reader.onerror = () => toast.error('Could not read the selected image.')
    reader.readAsDataURL(file)
  }

  const handlePhotoRemove = () => {
    setPhoto('')
    setPhotoChanged(true)
  }

  const updateMutation = useMutation({
    mutationFn: () => {
      // Diffed against the same projection the dirty check uses, so "the form
      // is unchanged" and "there is nothing to send" can no longer disagree.
      // Hand-written per-field comparisons had drifted: half tested against
      // `user.x` and half against `user.x || ''`, so an unset attribute made
      // the field look edited and got written back on every save.
      const updateData: Record<string, string | undefined> = changedFields(
        formData,
        identityForm(user),
      )
      if (photoChanged) updateData.jpegPhoto = photo
      return api.users.update(dn, updateData)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      setPhotoChanged(false)
      toast.success('User updated successfully')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    updateMutation.mutate()
  }

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <UnsavedChangesGuard when={textFieldsDirty || photoChanged} />
      <form onSubmit={handleSubmit} className="space-y-4">
        {!canWrite && <ReadOnlyNotice />}
        <Card>
          <CardHeader>
            <CardTitle>Personal Information</CardTitle>
            <CardDescription>inetOrgPerson attributes</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {canWrite && (
              <div className="space-y-2">
                <Label>Photo</Label>
                <div className="flex items-end gap-4">
                  <ImageDropTarget
                    label="Photo"
                    accept="image/jpeg,image/png,image/gif"
                    onFile={handlePhotoFile}
                    className="h-28 w-28 rounded-full"
                  >
                    {photo && (
                      <Avatar
                        src={photo}
                        fallback={user?.displayName || user?.cn}
                        className="h-full w-full"
                      />
                    )}
                  </ImageDropTarget>
                  <div className="flex gap-2">
                    {photo && (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={handlePhotoRemove}
                      >
                        <Trash2 className="h-4 w-4 mr-1" />
                        Remove
                      </Button>
                    )}
                  </div>
                </div>
              </div>
            )}

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="uid">Username (UID)</Label>
                <Input id="uid" value={user?.uid || ''} disabled />
                <p className="text-xs text-muted-foreground">To rename the account, use “Change username” in the Security tab.</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="cn">Common Name (CN)</Label>
                <Input
                  id="cn"
                  value={formData.cn}
                  onChange={(e) => setFormData({ ...formData, cn: e.target.value })}
                  disabled={!canWrite}
                />
              </div>
            </FieldRow>

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="givenName">First Name</Label>
                <Input
                  id="givenName"
                  value={formData.givenName}
                  onChange={(e) => setFormData({ ...formData, givenName: e.target.value })}
                  disabled={!canWrite}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="sn">Last Name</Label>
                <Input
                  id="sn"
                  value={formData.sn}
                  onChange={(e) => setFormData({ ...formData, sn: e.target.value })}
                  disabled={!canWrite}
                />
              </div>
            </FieldRow>

            <div className="space-y-2">
              <Label htmlFor="displayName">Display Name</Label>
              <Input
                id="displayName"
                value={formData.displayName}
                onChange={(e) => setFormData({ ...formData, displayName: e.target.value })}
                disabled={!canWrite}
              />
            </div>

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="mail">Email</Label>
                <Input
                  id="mail"
                  type="email"
                  value={formData.mail}
                  onChange={(e) => setFormData({ ...formData, mail: e.target.value })}
                  disabled={!canWrite}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="telephoneNumber">Phone</Label>
                <Input
                  id="telephoneNumber"
                  value={formData.telephoneNumber}
                  onChange={(e) => setFormData({ ...formData, telephoneNumber: e.target.value })}
                  disabled={!canWrite}
                />
              </div>
            </FieldRow>

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="title">Title</Label>
                <Input
                  id="title"
                  value={formData.title}
                  onChange={(e) => setFormData({ ...formData, title: e.target.value })}
                  disabled={!canWrite}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="departmentNumber">Department</Label>
                <Input
                  id="departmentNumber"
                  value={formData.departmentNumber}
                  onChange={(e) => setFormData({ ...formData, departmentNumber: e.target.value })}
                  disabled={!canWrite}
                />
              </div>
            </FieldRow>

            <div className="space-y-2">
              <Label htmlFor="o">Organization</Label>
              <Input
                id="o"
                value={formData.o}
                onChange={(e) => setFormData({ ...formData, o: e.target.value })}
                disabled={!canWrite}
              />
            </div>

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="employeeNumber">Employee Number</Label>
                <Input
                  id="employeeNumber"
                  value={formData.employeeNumber}
                  onChange={(e) => setFormData({ ...formData, employeeNumber: e.target.value })}
                  disabled={!canWrite}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="employeeType">Employee Type</Label>
                <Input
                  id="employeeType"
                  value={formData.employeeType}
                  onChange={(e) => setFormData({ ...formData, employeeType: e.target.value })}
                  disabled={!canWrite}
                />
              </div>
            </FieldRow>

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="initials">Initials</Label>
                <Input
                  id="initials"
                  value={formData.initials}
                  onChange={(e) => setFormData({ ...formData, initials: e.target.value })}
                  disabled={!canWrite}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="manager">Manager</Label>
                <Dialog open={managerDropdownOpen} onOpenChange={setManagerDropdownOpen}>
                  <DialogTrigger asChild>
                    <Button
                      type="button"
                      variant="outline"
                      className="w-full justify-start font-normal"
                      disabled={!canWrite}
                    >
                      {formData.manager ? (
                        (() => {
                          // The candidate list is one search's worth, so the
                          // current manager is often not in it. Its uid, taken
                          // from the DN, reads better than the DN itself.
                          const mgr = managerPicker.items.find(u => u.dn === formData.manager)
                          return mgr
                            ? `${mgr.displayName || mgr.cn} (${mgr.uid})`
                            : rdnValue(formData.manager)
                        })()
                      ) : (
                        <span className="text-muted-foreground">Select manager...</span>
                      )}
                    </Button>
                  </DialogTrigger>
                  <DialogContent className="max-w-md">
                    <DialogHeader>
                      <DialogTitle>Select Manager</DialogTitle>
                      <DialogDescription>
                        Choose a manager for this user.
                      </DialogDescription>
                    </DialogHeader>
                    <div className="space-y-4">
                      <Input
                        placeholder="Search users..."
                        value={managerPicker.input}
                        onChange={(e) => managerPicker.setInput(e.target.value)}
                      />
                      <div className="max-h-64 overflow-y-auto space-y-1">
                        {formData.manager && (
                          <button
                            type="button"
                            onClick={() => {
                              setFormData({ ...formData, manager: '' })
                              setManagerDropdownOpen(false)
                              managerPicker.reset()
                            }}
                            className="w-full flex items-center gap-3 p-2 rounded-md text-left transition-colors hover:bg-muted text-muted-foreground"
                          >
                            <X className="h-4 w-4" />
                            <span>Clear manager</span>
                          </button>
                        )}
                        {availableManagers.length > 0 ? (
                          availableManagers.map((mgr) => (
                            <button
                              key={mgr.dn}
                              type="button"
                              aria-pressed={formData.manager === mgr.dn}
                              onClick={() => {
                                setFormData({ ...formData, manager: mgr.dn })
                                setManagerDropdownOpen(false)
                                managerPicker.reset()
                              }}
                              className={`w-full flex items-center gap-3 p-2 rounded-md text-left transition-colors ${
                                formData.manager === mgr.dn
                                  ? 'bg-primary text-primary-foreground'
                                  : 'hover:bg-muted'
                              }`}
                            >
                              <Avatar
                                url={mgr.hasPhoto ? userPhotoUrl(mgr.dn) : undefined}
                                fallback={mgr.displayName || mgr.cn}
                                size="sm"
                              />
                              <div className="flex-1 min-w-0">
                                <div className="flex items-center gap-1.5">
                                  <span className="font-medium truncate">
                                    {mgr.displayName || mgr.cn}
                                  </span>
                                  {/* A locked account can still be someone's
                                      manager, but it is worth knowing before
                                      picking one. */}
                                  <AccountStatus user={mgr} quiet />
                                </div>
                                <div className={`text-sm truncate ${
                                  formData.manager === mgr.dn
                                    ? 'text-primary-foreground/70'
                                    : 'text-muted-foreground'
                                }`}>
                                  {mgr.uid}
                                </div>
                              </div>
                            </button>
                          ))
                        ) : (
                          <p className="text-sm text-muted-foreground text-center py-4">
                            {managerPicker.isFetching
                              ? 'Searching...'
                              : managerPicker.input
                                ? 'No user matches this search.'
                                : 'No other user to pick from.'}
                          </p>
                        )}
                      </div>
                      <PickerOverflow count={managerPicker.overflow} />
                    </div>
                  </DialogContent>
                </Dialog>
              </div>
            </FieldRow>

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Input
                id="description"
                value={formData.description}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                disabled={!canWrite}
              />
            </div>

            {canWrite && (
              <div className="flex justify-end gap-2 pt-4">
                <FormActions
                  dirty={textFieldsDirty || photoChanged}
                  pending={updateMutation.isPending}
                  onDiscard={() => {
                    setFormData(identityForm(user))
                    setPhoto(user.jpegPhoto || '')
                    setPhotoChanged(false)
                  }}
                />
              </div>
            )}
          </CardContent>
        </Card>
      </form>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="flex items-center gap-2">
            <Users className="h-5 w-5" />
            Group Memberships ({groups.length})
          </CardTitle>
          {canWrite && (
            <Dialog open={addGroupOpen} onOpenChange={setAddGroupOpen}>
              <DialogTrigger asChild>
                <Button size="sm">
                  <UserPlus className="h-4 w-4 mr-1" />
                  Add to Group
                </Button>
              </DialogTrigger>
              <DialogContent className="max-w-md">
                <DialogHeader>
                  <DialogTitle>Add to Groups</DialogTitle>
                  <DialogDescription>
                    Select one or more groups to add this user to.
                  </DialogDescription>
                </DialogHeader>
                <div className="space-y-4">
                  <div className="relative">
                    <Input
                      placeholder="Search groups..."
                      value={groupPicker.input}
                      onChange={(e) => groupPicker.setInput(e.target.value)}
                    />
                  </div>
                  {selectedGroups.length > 0 && (
                    <div className="text-sm text-muted-foreground">
                      {selectedGroups.length} group{selectedGroups.length > 1 ? 's' : ''} selected
                    </div>
                  )}
                  <div className="max-h-64 overflow-y-auto space-y-1">
                    {availableGroups.length > 0 ? (
                      availableGroups.map((group) => {
                        const isSelected = selectedGroups.includes(group.dn)
                        return (
                          <button
                            key={group.dn}
                            type="button"
                            aria-pressed={isSelected}
                            onClick={() => {
                              if (isSelected) {
                                setSelectedGroups(selectedGroups.filter(g => g !== group.dn))
                              } else {
                                setSelectedGroups([...selectedGroups, group.dn])
                              }
                            }}
                            className={`w-full flex items-center gap-3 p-2 rounded-md text-left transition-colors ${
                              isSelected
                                ? 'bg-primary text-primary-foreground'
                                : 'hover:bg-muted'
                            }`}
                          >
                            <div className={`h-4 w-4 rounded border flex items-center justify-center shrink-0 ${
                              isSelected ? 'bg-primary-foreground border-primary-foreground' : 'border-muted-foreground'
                            }`}>
                              {isSelected && (
                                <svg className="h-3 w-3 text-primary" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                                  <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                                </svg>
                              )}
                            </div>
                            <Users className="h-4 w-4 shrink-0" />
                            <span className="font-medium">{group.cn}</span>
                          </button>
                        )
                      })
                    ) : (
                      <p className="text-sm text-muted-foreground text-center py-4">
                        {groupPicker.isFetching
                          ? 'Searching...'
                          : groupPicker.input
                            ? 'No group matches this search.'
                            : 'This user is already in every group.'}
                      </p>
                    )}
                  </div>
                  <PickerOverflow count={groupPicker.overflow} />
                </div>
                <div className="flex justify-end gap-2">
                  <Button variant="outline" onClick={() => {
                    setAddGroupOpen(false)
                    setSelectedGroups([])
                    groupPicker.reset()
                  }}>
                    Cancel
                  </Button>
                  <Button
                    onClick={() => addToGroupMutation.mutate(selectedGroups)}
                    disabled={selectedGroups.length === 0 || addToGroupMutation.isPending}
                  >
                    {addToGroupMutation.isPending ? 'Adding...' : `Add to ${selectedGroups.length || ''} Group${selectedGroups.length !== 1 ? 's' : ''}`}
                  </Button>
                </div>
              </DialogContent>
            </Dialog>
          )}
        </CardHeader>
        <CardContent>
          {(addToGroupMutation.error || removeFromGroupMutation.error) && (
            <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md mb-4">
              {(addToGroupMutation.error || removeFromGroupMutation.error)?.message}
            </div>
          )}
          {groups.length > 0 ? (
            <ul className="space-y-2">
              {[...groups].sort((a, b) => a.cn.localeCompare(b.cn)).map((group) => (
                <li
                  key={group.dn}
                  className="flex items-center justify-between p-2 rounded-md bg-muted"
                >
                  <Link
                    to="/groups/$dn"
                    params={{ dn: encodeDN(group.dn) }}
                    className="flex items-center gap-2 hover:underline"
                  >
                    <Users className="h-4 w-4 text-muted-foreground" />
                    <span className="font-medium">{group.cn}</span>
                  </Link>
                  {canWrite && (
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Remove from group"
                      onClick={() => setConfirmRemoveGroup({ dn: group.dn, cn: group.cn })}
                      disabled={removeFromGroupMutation.isPending}
                    >
                      <UserMinus className="h-4 w-4 text-destructive" />
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground text-center py-4">
              Not a member of any groups
            </p>
          )}
        </CardContent>
        <ConfirmDialog
          open={confirmRemoveGroup !== null}
          onOpenChange={(open) => { if (!open) setConfirmRemoveGroup(null) }}
          title="Remove from group"
          description={`Remove this user from group "${confirmRemoveGroup?.cn}"? They will lose the access this group grants.`}
          confirmLabel="Remove"
          pendingLabel="Removing..."
          error={removeFromGroupMutation.error?.message}
          isPending={removeFromGroupMutation.isPending}
          onConfirm={() => {
            if (confirmRemoveGroup) removeFromGroupMutation.mutate(confirmRemoveGroup.dn)
            setConfirmRemoveGroup(null)
          }}
        />
      </Card>
    </div>
  )
}

// POSIX Tab (posixAccount)
function PosixTab({ user, dn, canWrite }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean }) {
  const queryClient = useQueryClient()

  const [formData, setFormData] = useState(() => posixForm(user))
  const isDirty = useSyncedForm(posixForm(user), formData, setFormData)

  const updateMutation = useMutation({
    mutationFn: () => api.users.update(dn, changedFields(formData, posixForm(user))),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      toast.success('POSIX settings updated')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    updateMutation.mutate()
  }

  return (
    <form onSubmit={handleSubmit}>
      <UnsavedChangesGuard when={isDirty} />
      <Card>
        <CardHeader>
          <CardTitle>POSIX Account</CardTitle>
          <CardDescription>posixAccount attributes for Unix/Linux systems</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <FieldRow>
            <div className="space-y-2">
              <Label htmlFor="uidNumber">UID Number</Label>
              <Input
                id="uidNumber"
                value={user?.uidNumber ?? ''}
                disabled
                className={!user?.uidNumber ? 'text-muted-foreground italic' : ''}
                placeholder="Not set"
              />
              <p className="text-xs text-muted-foreground">
                Unique numeric user ID. It is what the filesystem stores, so it is changed on its
                own, below.
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="gidNumber">GID Number</Label>
              <Input
                id="gidNumber"
                value={user?.gidNumber ?? ''}
                disabled
                className={!user?.gidNumber ? 'text-muted-foreground italic' : ''}
                placeholder="Not set"
              />
              <p className="text-xs text-muted-foreground">Primary group ID.</p>
            </div>
          </FieldRow>

          {canWrite && (
            <PosixIdsChange
              dn={dn}
              uid={user.uid}
              uidNumber={user.uidNumber}
              gidNumber={user.gidNumber}
            />
          )}

          <div className="space-y-2">
            <Label htmlFor="homeDirectory">Home Directory</Label>
            <Input
              id="homeDirectory"
              value={formData.homeDirectory}
              onChange={(e) => setFormData({ ...formData, homeDirectory: e.target.value })}
              disabled={!canWrite}
              placeholder="/home/username"
              className={!formData.homeDirectory ? 'text-muted-foreground' : ''}
            />
            <p className="text-xs text-muted-foreground">User's home directory path</p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="loginShell">Login Shell</Label>
            <Input
              id="loginShell"
              value={formData.loginShell}
              onChange={(e) => setFormData({ ...formData, loginShell: e.target.value })}
              disabled={!canWrite}
              placeholder="/bin/bash"
              className={!formData.loginShell ? 'text-muted-foreground' : ''}
            />
            <p className="text-xs text-muted-foreground">Default shell for the user (e.g., /bin/bash, /bin/zsh, /bin/false)</p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="gecos">GECOS</Label>
            <Input
              id="gecos"
              value={formData.gecos}
              onChange={(e) => setFormData({ ...formData, gecos: e.target.value })}
              disabled={!canWrite}
              placeholder="Full Name,Room,Phone,Other"
              className={!formData.gecos ? 'text-muted-foreground' : ''}
            />
            <p className="text-xs text-muted-foreground">General information about the user (typically: Full Name,Room Number,Work Phone,Home Phone)</p>
          </div>

          {canWrite && (
            <div className="flex justify-end gap-2 pt-4">
              <FormActions
                dirty={isDirty}
                pending={updateMutation.isPending}
                onDiscard={() => setFormData(posixForm(user))}
              />
            </div>
          )}
        </CardContent>
      </Card>
    </form>
  )
}

// SSH Keys Tab (ldapPublicKey)
function SSHKeysTab({ user, dn, canWrite }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean }) {
  const queryClient = useQueryClient()
  const [newKey, setNewKey] = useState('')

  const addKeyMutation = useMutation({
    mutationFn: (key: string) => api.users.addSSHKey(dn, key),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      setNewKey('')
      toast.success('SSH key added')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const removeKeyMutation = useMutation({
    mutationFn: (key: string) => api.users.removeSSHKey(dn, key),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      toast.success('SSH key removed')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handleAddKey = (e: React.FormEvent) => {
    e.preventDefault()
    if (newKey.trim()) {
      addKeyMutation.mutate(newKey.trim())
    }
  }

  const parseKeyInfo = (key: string) => {
    const parts = key.split(' ')
    const type = parts[0] || 'unknown'
    const comment = parts.length > 2 ? parts.slice(2).join(' ') : ''
    const fingerprint = parts[1] ? `${parts[1].slice(0, 20)}...` : ''
    return { type, comment, fingerprint }
  }

  const copyKey = async (key: string) => {
    try {
      await navigator.clipboard.writeText(key)
      toast.success('SSH key copied to clipboard')
    } catch {
      toast.error('Could not copy to clipboard')
    }
  }

  const sshKeys = user?.sshPublicKey || []

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Key className="h-5 w-5" />
          SSH Public Keys ({sshKeys.length})
        </CardTitle>
        <CardDescription>ldapPublicKey attributes for SSH authentication</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {sshKeys.length > 0 ? (
          <ul className="space-y-2">
            {sshKeys.map((key, index) => {
              const { type, comment, fingerprint } = parseKeyInfo(key)
              return (
                <li
                  key={index}
                  className="flex items-center justify-between p-3 rounded-md bg-muted"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <Key className="h-4 w-4 text-muted-foreground shrink-0" />
                      <span className="font-mono text-sm">{type}</span>
                      {comment && (
                        <span className="text-sm text-muted-foreground truncate">
                          {comment}
                        </span>
                      )}
                    </div>
                    <p className="text-xs text-muted-foreground font-mono mt-1 truncate">
                      {fingerprint}
                    </p>
                  </div>
                  <div className="flex items-center shrink-0">
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Copy SSH public key"
                      title="Copy public key"
                      onClick={() => copyKey(key)}
                    >
                      <Copy className="h-4 w-4" />
                    </Button>
                    {canWrite && (
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label="Remove SSH key"
                        onClick={() => removeKeyMutation.mutate(key)}
                        disabled={removeKeyMutation.isPending}
                      >
                        <X className="h-4 w-4 text-destructive" />
                      </Button>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground text-center py-4">
            No SSH keys configured
          </p>
        )}

        {canWrite && (
          <form onSubmit={handleAddKey} className="space-y-2">
            <Label htmlFor="newKey">Add SSH Public Key</Label>
            <div className="flex gap-2">
              <Input
                id="newKey"
                value={newKey}
                onChange={(e) => setNewKey(e.target.value)}
                placeholder="ssh-rsa AAAA... user@host"
                className="font-mono text-sm"
              />
              <Button type="submit" disabled={!newKey.trim() || addKeyMutation.isPending}>
                <Plus className="h-4 w-4 mr-1" />
                Add
              </Button>
            </div>
          </form>
        )}
      </CardContent>
    </Card>
  )
}

// Samba Tab (sambaSamAccount)
function SambaTab({ user, dn, canWrite, hasObjectClass }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean, hasObjectClass: boolean }) {
  const queryClient = useQueryClient()
  const [isAddingObjectClass, setIsAddingObjectClass] = useState(false)

  const [formData, setFormData] = useState(() => sambaForm(user))
  // Only synced once the entry actually carries the object class: while the
  // "Add Samba Account" form is open, the server copy is all empty strings and
  // adopting it would wipe the defaults just filled in.
  const isDirty = useSyncedForm(
    hasObjectClass && !isAddingObjectClass ? sambaForm(user) : undefined,
    formData,
    setFormData,
  )

  // Initialize form with default values when entering add mode
  const handleAddSambaAccount = () => {
    setFormData({
      sambaSID: `S-1-5-21-0-0-0-${user.uidNumber}`,
      sambaPrimaryGroupSID: '',
      sambaAcctFlags: '[U          ]',
      sambaHomePath: '',
      sambaHomeDrive: '',
      sambaLogonScript: '',
      sambaProfilePath: '',
      sambaDomainName: '',
    })
    setIsAddingObjectClass(true)
  }

  const updateMutation = useMutation({
    mutationFn: () => api.users.updateSamba(dn, changedFields(formData, sambaForm(user))),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      setIsAddingObjectClass(false)
      toast.success(isAddingObjectClass ? 'Samba account added' : 'Samba settings updated')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    updateMutation.mutate()
  }

  // Show "Add ObjectClass" UI if user doesn't have sambaSamAccount and not in add mode
  if (!hasObjectClass && !isAddingObjectClass) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Samba Account</CardTitle>
          <CardDescription>sambaSamAccount attributes for Windows/Samba integration</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <div className="rounded-full bg-muted p-6 mb-4">
              <svg className="h-12 w-12 text-muted-foreground" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                <path d="M4 22h14a2 2 0 0 0 2-2V7.5L14.5 2H6a2 2 0 0 0-2 2v4" />
                <polyline points="14 2 14 8 20 8" />
                <path d="M2 15h10" />
                <path d="m9 18 3-3-3-3" />
              </svg>
            </div>
            <h3 className="text-lg font-medium mb-2">Samba Not Enabled</h3>
            <p className="text-muted-foreground mb-6 max-w-sm">
              This user does not have the <code className="text-xs bg-muted px-1 py-0.5 rounded">sambaSamAccount</code> objectClass. Add it to enable Windows/Samba integration.
            </p>
            {canWrite ? (
              <Button
                onClick={handleAddSambaAccount}
                size="lg"
              >
                <Plus className="h-5 w-5 mr-2" />
                Add Samba Account
              </Button>
            ) : (
              <p className="text-sm text-muted-foreground">You don't have permission to add objectClasses.</p>
            )}
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <form onSubmit={handleSubmit}>
      <UnsavedChangesGuard when={isDirty} />
      <Card>
        <CardHeader>
          <CardTitle>{isAddingObjectClass ? 'Add Samba Account' : 'Samba Account'}</CardTitle>
          <CardDescription>
            {isAddingObjectClass
              ? 'Configure Samba attributes to enable Windows/Samba integration for this user'
              : 'sambaSamAccount attributes for Windows/Samba integration'}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {isAddingObjectClass && (
            <Callout variant="info">
              Fill in the Samba attributes below and click "Save Changes" to add the sambaSamAccount objectClass to this user.
            </Callout>
          )}

          <div className="space-y-2">
            <Label htmlFor="sambaSID">SID</Label>
            <Input
              id="sambaSID"
              value={formData.sambaSID}
              onChange={(e) => setFormData({ ...formData, sambaSID: e.target.value })}
              disabled={!canWrite}
              placeholder="S-1-5-21-..."
            />
            <p className="text-xs text-muted-foreground">Samba Security Identifier (required for Samba)</p>
          </div>

          <FieldRow>
            <div className="space-y-2">
              <Label htmlFor="sambaPrimaryGroupSID">Primary Group SID</Label>
              <Input
                id="sambaPrimaryGroupSID"
                value={formData.sambaPrimaryGroupSID}
                onChange={(e) => setFormData({ ...formData, sambaPrimaryGroupSID: e.target.value })}
                disabled={!canWrite}
                placeholder="S-1-5-21-...-513"
              />
              <p className="text-xs text-muted-foreground">Primary group's SID</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="sambaAcctFlags">Account Flags</Label>
              <Input
                id="sambaAcctFlags"
                value={formData.sambaAcctFlags}
                onChange={(e) => setFormData({ ...formData, sambaAcctFlags: e.target.value })}
                disabled={!canWrite}
                placeholder="[U          ]"
              />
              <p className="text-xs text-muted-foreground">Account flags (e.g., [U] for user)</p>
            </div>
          </FieldRow>

          <FieldRow>
            <div className="space-y-2">
              <Label htmlFor="sambaHomePath">Home Path</Label>
              <Input
                id="sambaHomePath"
                value={formData.sambaHomePath}
                onChange={(e) => setFormData({ ...formData, sambaHomePath: e.target.value })}
                disabled={!canWrite}
                placeholder="\\server\homes\username"
              />
              <p className="text-xs text-muted-foreground">Windows home directory path</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="sambaHomeDrive">Home Drive</Label>
              <Input
                id="sambaHomeDrive"
                value={formData.sambaHomeDrive}
                onChange={(e) => setFormData({ ...formData, sambaHomeDrive: e.target.value })}
                disabled={!canWrite}
                placeholder="H:"
              />
              <p className="text-xs text-muted-foreground">Drive letter for home</p>
            </div>
          </FieldRow>

          <FieldRow>
            <div className="space-y-2">
              <Label htmlFor="sambaLogonScript">Logon Script</Label>
              <Input
                id="sambaLogonScript"
                value={formData.sambaLogonScript}
                onChange={(e) => setFormData({ ...formData, sambaLogonScript: e.target.value })}
                disabled={!canWrite}
                placeholder="logon.bat"
              />
              <p className="text-xs text-muted-foreground">Script to run at logon</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="sambaProfilePath">Profile Path</Label>
              <Input
                id="sambaProfilePath"
                value={formData.sambaProfilePath}
                onChange={(e) => setFormData({ ...formData, sambaProfilePath: e.target.value })}
                disabled={!canWrite}
                placeholder="\\server\profiles\username"
              />
              <p className="text-xs text-muted-foreground">Roaming profile path</p>
            </div>
          </FieldRow>

          <div className="space-y-2">
            <Label htmlFor="sambaDomainName">Domain Name</Label>
            <Input
              id="sambaDomainName"
              value={formData.sambaDomainName}
              onChange={(e) => setFormData({ ...formData, sambaDomainName: e.target.value })}
              disabled={!canWrite}
              placeholder="DOMAIN"
            />
            <p className="text-xs text-muted-foreground">Samba domain name</p>
          </div>

          {user?.sambaPwdLastSet && !isAddingObjectClass && (
            <div className="p-3 bg-muted rounded-md text-sm">
              <p><span className="font-medium">Password Last Set:</span> {new Date(parseInt(user.sambaPwdLastSet) * 1000).toLocaleString()}</p>
            </div>
          )}

          {canWrite && (
            <div className="flex justify-end gap-2 pt-4">
              {/* In add mode the defaults are the change: nothing is stored yet to
                  compare them with, so Save stays available. */}
              <FormActions
                dirty={isDirty || isAddingObjectClass}
                pending={updateMutation.isPending}
                onDiscard={isAddingObjectClass ? undefined : () => setFormData(sambaForm(user))}
              >
                {isAddingObjectClass && (
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => setIsAddingObjectClass(false)}
                  >
                    Cancel
                  </Button>
                )}
              </FormActions>
            </div>
          )}
        </CardContent>
      </Card>
    </form>
  )
}

// Shadow Tab (shadowAccount)
function ShadowTab({ user, dn, canWrite, hasObjectClass, isSelf }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean, hasObjectClass: boolean, isSelf: boolean }) {
  const queryClient = useQueryClient()
  const [isAddingObjectClass, setIsAddingObjectClass] = useState(false)

  const [formData, setFormData] = useState(() => shadowForm(user))
  // As with Samba: not synced while the add-object-class form is open, whose
  // prefilled defaults are not on the server yet.
  const isDirty = useSyncedForm(
    hasObjectClass && !isAddingObjectClass ? shadowForm(user) : undefined,
    formData,
    setFormData,
  )

  // Initialize form with default values when entering add mode
  const handleAddShadowAccount = () => {
    // Calculate days since epoch for today
    const daysSinceEpoch = Math.floor(Date.now() / (1000 * 60 * 60 * 24))
    setFormData({
      shadowLastChange: daysSinceEpoch.toString(),
      shadowMin: '0',
      shadowMax: '99999',
      shadowWarning: '7',
      shadowInactive: '',
      shadowExpire: '',
      shadowFlag: '0',
    })
    setIsAddingObjectClass(true)
  }

  const updateMutation = useMutation({
    mutationFn: () => {
      const changed = changedFields(formData, shadowForm(user))
      const updateData: Record<string, number | undefined> = {}
      for (const [key, value] of Object.entries(changed)) {
        updateData[key] = value === '' ? undefined : parseInt(value, 10)
      }
      return api.users.updateShadow(dn, updateData)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      setIsAddingObjectClass(false)
      toast.success(isAddingObjectClass ? 'Shadow account added' : 'Shadow settings updated')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    updateMutation.mutate()
  }

  // Convert days since epoch to readable date
  const daysToDate = (days: number): string => {
    if (!days) return 'Not set'
    const date = new Date(days * 24 * 60 * 60 * 1000)
    return date.toLocaleDateString()
  }

  // Show "Add ObjectClass" UI if user doesn't have shadowAccount and not in add mode
  if (!hasObjectClass && !isAddingObjectClass) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Shadow Account</CardTitle>
          <CardDescription>shadowAccount attributes for Unix password aging</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <div className="rounded-full bg-muted p-6 mb-4">
              <svg className="h-12 w-12 text-muted-foreground" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="10" />
                <path d="M12 2a7 7 0 0 0 0 14 7 7 0 0 0 0-14" fill="currentColor" opacity="0.2" />
                <path d="M12 6v6l4 2" />
              </svg>
            </div>
            <h3 className="text-lg font-medium mb-2">Shadow Not Enabled</h3>
            <p className="text-muted-foreground mb-6 max-w-sm">
              This user does not have the <code className="text-xs bg-muted px-1 py-0.5 rounded">shadowAccount</code> objectClass. Add it to enable Unix password aging.
            </p>
            {canWrite ? (
              <Button
                onClick={handleAddShadowAccount}
                size="lg"
              >
                <Plus className="h-5 w-5 mr-2" />
                Add Shadow Account
              </Button>
            ) : (
              <p className="text-sm text-muted-foreground">You don't have permission to add objectClasses.</p>
            )}
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <form onSubmit={handleSubmit}>
      <UnsavedChangesGuard when={isDirty} />
      <Card>
        <CardHeader>
          <CardTitle>{isAddingObjectClass ? 'Add Shadow Account' : 'Shadow Account'}</CardTitle>
          <CardDescription>
            {isAddingObjectClass
              ? 'Configure shadow attributes to enable Unix password aging for this user'
              : 'shadowAccount attributes for Unix password aging and expiration'}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {isAddingObjectClass && (
            <Callout variant="info">
              Fill in the shadow attributes below and click "Save Changes" to add the shadowAccount objectClass to this user.
            </Callout>
          )}

          <div className="space-y-2">
            <Label htmlFor="shadowLastChange">Last Password Change</Label>
            <Input
              id="shadowLastChange"
              type="number"
              value={formData.shadowLastChange}
              onChange={(e) => setFormData({ ...formData, shadowLastChange: e.target.value })}
              disabled={!canWrite}
              placeholder="Days since Jan 1, 1970"
            />
            <p className="text-xs text-muted-foreground">
              Number of days since epoch when password was last changed
              {formData.shadowLastChange && ` (${daysToDate(parseInt(formData.shadowLastChange))})`}
            </p>
          </div>

          <FieldRow>
            <div className="space-y-2">
              <Label htmlFor="shadowMin">Minimum Age</Label>
              <Input
                id="shadowMin"
                type="number"
                value={formData.shadowMin}
                onChange={(e) => setFormData({ ...formData, shadowMin: e.target.value })}
                disabled={!canWrite}
                placeholder="0"
              />
              <p className="text-xs text-muted-foreground">Minimum days before password can be changed</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="shadowMax">Maximum Age</Label>
              <Input
                id="shadowMax"
                type="number"
                value={formData.shadowMax}
                onChange={(e) => setFormData({ ...formData, shadowMax: e.target.value })}
                disabled={!canWrite}
                placeholder="99999"
              />
              <p className="text-xs text-muted-foreground">Maximum days before password must be changed</p>
            </div>
          </FieldRow>

          <FieldRow>
            <div className="space-y-2">
              <Label htmlFor="shadowWarning">Warning Period</Label>
              <Input
                id="shadowWarning"
                type="number"
                value={formData.shadowWarning}
                onChange={(e) => setFormData({ ...formData, shadowWarning: e.target.value })}
                disabled={!canWrite}
                placeholder="7"
              />
              <p className="text-xs text-muted-foreground">Days of warning before password expires</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="shadowInactive">Inactive Period</Label>
              <Input
                id="shadowInactive"
                type="number"
                value={formData.shadowInactive}
                onChange={(e) => setFormData({ ...formData, shadowInactive: e.target.value })}
                disabled={!canWrite}
                placeholder="Not set"
              />
              <p className="text-xs text-muted-foreground">Days after expiry before account disabled</p>
            </div>
          </FieldRow>

          <FieldRow>
            <div className="space-y-2">
              <Label htmlFor="shadowExpire">Account Expiration</Label>
              <Input
                id="shadowExpire"
                type="number"
                value={formData.shadowExpire}
                onChange={(e) => setFormData({ ...formData, shadowExpire: e.target.value })}
                disabled={!canWrite || isSelf}
                placeholder="Not set"
              />
              <p className="text-xs text-muted-foreground">
                {isSelf
                  ? 'Expiring your own account would lock you out, so this field is read-only here.'
                  : 'Days since epoch when account expires'}
                {!isSelf && formData.shadowExpire && ` (${daysToDate(parseInt(formData.shadowExpire))})`}
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="shadowFlag">Flag</Label>
              <Input
                id="shadowFlag"
                type="number"
                value={formData.shadowFlag}
                onChange={(e) => setFormData({ ...formData, shadowFlag: e.target.value })}
                disabled={!canWrite}
                placeholder="0"
              />
              <p className="text-xs text-muted-foreground">Reserved field (usually 0)</p>
            </div>
          </FieldRow>

          {canWrite && (
            <div className="flex justify-end gap-2 pt-4">
              {/* In add mode the defaults are the change: nothing is stored yet to
                  compare them with, so Save stays available. */}
              <FormActions
                dirty={isDirty || isAddingObjectClass}
                pending={updateMutation.isPending}
                onDiscard={isAddingObjectClass ? undefined : () => setFormData(shadowForm(user))}
              >
                {isAddingObjectClass && (
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => setIsAddingObjectClass(false)}
                  >
                    Cancel
                  </Button>
                )}
              </FormActions>
            </div>
          )}
        </CardContent>
      </Card>
    </form>
  )
}

// Sudo Tab (sudoRole)
function SudoTab({ user, dn, canWrite }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean }) {
  const queryClient = useQueryClient()
  const [addRoleOpen, setAddRoleOpen] = useState(false)
  const [selectedRole, setSelectedRole] = useState('')

  // Fetch user's sudo roles
  const { data: userSudoRoles, isLoading: loadingUserRoles } = useQuery({
    queryKey: ['user', dn, 'sudo-roles'],
    queryFn: ({ signal }) => api.users.getSudoRoles(dn, signal),
  })

  // Netgroups reaching this user. Skipped where the directory has no
  // nisNetgroup — the endpoint answers 501 and an empty card would suggest the
  // user is in none — and where the module is off, since there would be no
  // Netgroups page to open from here.
  const netgroupsSupported = useNetgroupsEnabled()
  const { data: userNetgroups } = useQuery({
    queryKey: ['user', dn, 'netgroups'],
    queryFn: ({ signal }) => api.users.getNetgroups(dn, signal),
    enabled: netgroupsSupported,
  })

  // Candidate roles come from the directory; headroom covers the ones the user
  // already holds, dropped just below.
  const rolePicker = usePickerSearch(['sudo-roles', 'picker', dn], api.sudoRoles.list, {
    sort: 'cn',
    headroom: userSudoRoles?.data.length ?? 0,
    enabled: addRoleOpen,
  })

  const availableRoles = useMemo(() => {
    const userRoleDns = new Set(userSudoRoles?.data.map(r => r.dn) ?? [])
    return rolePicker.items.filter(r => !userRoleDns.has(r.dn))
  }, [rolePicker.items, userSudoRoles?.data])

  const addToRoleMutation = useMutation({
    mutationFn: (roleDn: string) => api.sudoRoles.addUser(roleDn, user.uid),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn, 'sudo-roles'] })
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      setAddRoleOpen(false)
      setSelectedRole('')
      rolePicker.reset()
      toast.success('User added to sudo role')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const removeFromRoleMutation = useMutation({
    mutationFn: (roleDn: string) => api.sudoRoles.removeUser(roleDn, user.uid),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn, 'sudo-roles'] })
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      toast.success('User removed from sudo role')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const sudoRoles = userSudoRoles?.data || []

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheck className="h-5 w-5" />
            Sudo Roles ({sudoRoles.length})
          </CardTitle>
          <CardDescription>sudoRole memberships for sudo privileges</CardDescription>
        </div>
        {canWrite && (
          <Dialog open={addRoleOpen} onOpenChange={setAddRoleOpen}>
            <DialogTrigger asChild>
              <Button size="sm">
                <Plus className="h-4 w-4 mr-1" />
                Add to Sudo Role
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-md">
              <DialogHeader>
                <DialogTitle>Add to Sudo Role</DialogTitle>
                <DialogDescription>
                  Select a sudo role to grant this user sudo privileges.
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4">
                <div className="relative">
                  <Input
                    placeholder="Search sudo roles..."
                    value={rolePicker.input}
                    onChange={(e) => rolePicker.setInput(e.target.value)}
                  />
                </div>
                <div className="max-h-64 overflow-y-auto space-y-1">
                  {availableRoles.length > 0 ? (
                    availableRoles.map((role) => (
                      <button
                        key={role.dn}
                        type="button"
                        aria-pressed={selectedRole === role.dn}
                        onClick={() => setSelectedRole(role.dn)}
                        className={`w-full flex items-start gap-3 p-2 rounded-md text-left transition-colors ${
                          selectedRole === role.dn
                            ? 'bg-primary text-primary-foreground'
                            : 'hover:bg-muted'
                        }`}
                      >
                        <ShieldCheck className="h-4 w-4 shrink-0 mt-0.5" />
                        <div className="min-w-0 flex-1">
                          <span className="font-medium block">{role.cn}</span>
                          {role.sudoCommand && role.sudoCommand.length > 0 && (
                            <span className={`text-xs truncate block ${
                              selectedRole === role.dn ? 'text-primary-foreground/70' : 'text-muted-foreground'
                            }`}>
                              Commands: {role.sudoCommand.slice(0, 2).join(', ')}
                              {role.sudoCommand.length > 2 ? ` +${role.sudoCommand.length - 2} more` : ''}
                            </span>
                          )}
                        </div>
                      </button>
                    ))
                  ) : (
                    <p className="text-sm text-muted-foreground text-center py-4">
                      {rolePicker.isFetching
                        ? 'Searching...'
                        : rolePicker.input
                          ? 'No sudo role matches this search.'
                          : 'This user is already in every sudo role.'}
                    </p>
                  )}
                </div>
                <PickerOverflow count={rolePicker.overflow} />
              </div>
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => {
                  setAddRoleOpen(false)
                  setSelectedRole('')
                  rolePicker.reset()
                }}>
                  Cancel
                </Button>
                <Button
                  onClick={() => addToRoleMutation.mutate(selectedRole)}
                  disabled={!selectedRole || addToRoleMutation.isPending}
                >
                  {addToRoleMutation.isPending ? 'Adding...' : 'Add'}
                </Button>
              </div>
            </DialogContent>
          </Dialog>
        )}
      </CardHeader>
      <CardContent>
        {netgroupsSupported && (userNetgroups?.data.length ?? 0) > 0 && (
          <div className="mb-4 space-y-1.5 rounded-md border p-3">
            <p className="flex items-center gap-1.5 text-sm font-medium">
              <Network className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
              Netgroups
            </p>
            <p className="text-xs text-muted-foreground">
              A sudo rule naming one of these as “+name” grants to this user.
            </p>
            <ul className="flex flex-wrap gap-1.5 pt-1">
              {userNetgroups?.data.map((membership) => (
                <li key={membership.cn}>
                  <span
                    className="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-sm"
                    title={
                      membership.direct
                        ? 'Named directly by this netgroup.'
                        : 'Reached only through a nesting parent — edit the netgroup that names the user, not this one.'
                    }
                  >
                    {membership.cn}
                    {!membership.direct && (
                      <span className="text-xs text-muted-foreground">nested</span>
                    )}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        )}

        {(addToRoleMutation.error || removeFromRoleMutation.error) && (
          <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md mb-4">
            {(addToRoleMutation.error || removeFromRoleMutation.error)?.message}
          </div>
        )}

        {loadingUserRoles ? (
          <div className="flex items-center justify-center py-8">
            <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary"></div>
          </div>
        ) : sudoRoles.length > 0 ? (
          <div className="space-y-3">
            {sudoRoles.map((role) => (
              <div
                key={role.dn}
                className="flex items-start justify-between p-3 rounded-md bg-muted"
              >
                <div className="flex-1 min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <ShieldCheck className="h-4 w-4 text-muted-foreground" />
                    <span className="font-medium">{role.cn}</span>
                    {/* Why the role reaches this user. A role granted through a
                        group or a netgroup is removed from there, not here — the
                        remove button below only touches a direct grant. */}
                    {role.via.map((token) => (
                      <GrantOrigin key={token} token={token} uid={user.uid} />
                    ))}
                  </div>
                  <div className="text-sm text-muted-foreground space-y-0.5">
                    {role.sudoHost && role.sudoHost.length > 0 && (
                      <p><span className="font-medium">Hosts:</span> {role.sudoHost.join(', ')}</p>
                    )}
                    {role.sudoCommand && role.sudoCommand.length > 0 && (
                      <p><span className="font-medium">Commands:</span> {role.sudoCommand.join(', ')}</p>
                    )}
                    {role.sudoRunAsUser && role.sudoRunAsUser.length > 0 && (
                      <p><span className="font-medium">Run as:</span> {role.sudoRunAsUser.join(', ')}</p>
                    )}
                    {role.sudoOption && role.sudoOption.length > 0 && (
                      <p><span className="font-medium">Options:</span> {role.sudoOption.join(', ')}</p>
                    )}
                  </div>
                </div>
                {canWrite && role.via.includes(user.uid) && (
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Remove from sudo role"
                    onClick={() => removeFromRoleMutation.mutate(role.dn)}
                    disabled={removeFromRoleMutation.isPending}
                    className="shrink-0"
                  >
                    <X className="h-4 w-4 text-destructive" />
                  </Button>
                )}
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground text-center py-4">
            No sudo roles assigned
          </p>
        )}
      </CardContent>
    </Card>
  )
}

/**
 * Why a sudo role reaches this user.
 *
 * The three routes are edited in three different places, and the stored value
 * does not distinguish them to the eye: "+ops" reads like a username unless you
 * know what ops is. Naming the route is what stops someone looking for a user in
 * a rule that never mentioned them.
 */
function GrantOrigin({ token, uid }: { token: string; uid: string }) {
  if (token === uid) return null // direct: the default, not worth a badge
  const [label, title] =
    token === 'ALL'
      ? ['everyone', 'This rule applies to every user (sudoUser: ALL).']
      : token.startsWith('%')
        ? [`via group ${token.slice(1)}`, 'Granted through a group. Remove the user from that group to take it away.']
        : token.startsWith('+')
          ? [`via netgroup ${token.slice(1)}`, 'Granted through a netgroup, possibly a nested one. Edit that netgroup to take it away.']
          : [`via ${token}`, 'Granted through this sudoUser value.']
  return (
    <span
      className="rounded-full border px-2 py-0.5 text-xs text-muted-foreground"
      title={title}
    >
      {label}
    </span>
  )
}

// Helper function to extract CN from a DN
function extractCN(dn: string): string {
  const match = dn.match(/^cn=([^,]+)/i)
  return match ? match[1] : dn
}

// Security Tab (password, lock/unlock, delete)
function SecurityTab({ user, dn, canWrite, canDelete, showPoliciesModule, canExpire, policyEnforcement, isSelf }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean, canDelete: boolean, showPoliciesModule: boolean, canExpire: boolean, policyEnforcement: string, isSelf: boolean }) {
  const queryClient = useQueryClient()
  const router = useRouter()
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [removePasswordDialogOpen, setRemovePasswordDialogOpen] = useState(false)
  const [editingExpiration, setEditingExpiration] = useState(false)
  const [changeUidOpen, setChangeUidOpen] = useState(false)
  const [newUid, setNewUid] = useState('')
  const [confirmUid, setConfirmUid] = useState('')

  const uidPattern = /^[a-zA-Z0-9._-]+$/
  const newUidValid = uidPattern.test(newUid) && newUid !== user.uid

  const changeUidMutation = useMutation({
    mutationFn: (uid: string) => api.users.changeUid(dn, uid),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      toast.success(`Username changed to ${res.user.uid}`)
      showOutcome({ title: `Username changed to ${res.user.uid}: references not migrated`, items: res.warnings })
      setChangeUidOpen(false)
      // The DN changed; move to the renamed user's page.
      router.navigate({ to: '/users/$dn', params: { dn: encodeDN(res.user.dn) } })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  // Determine if the account has a future expiration date
  // Expiry and lock are independent: pwdEndTime says when the directory will
  // start refusing the account, pwdAccountLockedTime says it already does.
  const hasFutureExpiration = user?.pwdEndTime && isLdapTimestampInFuture(user.pwdEndTime)
  const hasPastExpiration = user?.pwdEndTime && !isLdapTimestampInFuture(user.pwdEndTime)

  // A lock has three questions behind it, and pwdAccountLockedTime answers only
  // the first: is a value there (hasLockFlag), is it still refusing binds
  // (lockedNow), and when does the policy lift it (liftsAt). ppolicy leaves a
  // lapsed lockout in the entry until the next bind on it, so the first two
  // answers differ for as long as nobody tries to log in.
  const hasLockFlag = Boolean(user?.pwdAccountLockedTime)
  const lockedNow = user ? isLocked(user) : false
  const lapsedLock = user ? hasLapsedLock(user) : false
  const liftsAt = user ? lockoutLiftsAt(user) : null
  // Null for a manual lock: 000001010000Z is a sentinel, not an instant, and the
  // directory records no date for one. See lockedAt in lib/account.ts.
  const lockInstant = user ? lockedAt(user) : null
  const lockDetail = ((): string | null => {
    switch (user?.lockout?.state) {
      case 'manual':
        return 'Locked by an administrator, so it does not expire on its own: it holds until someone unlocks it here.'
      case 'active':
        return liftsAt
          ? `Locked by the password policy. The directory stops refusing this account at ${liftsAt.toLocaleString()}.`
          : 'Locked by the password policy, whose lockout duration is 0 — it never lifts on its own.'
      case 'lifted':
        return `The policy lifted this lockout${liftsAt ? ` at ${liftsAt.toLocaleString()}` : ''}. The attribute is still on the entry: the directory drops it at the next login attempt, and Unlock clears it now.`
      case 'unknown':
        return 'Whether this lockout has already expired cannot be told from here, because the policy that applies to the entry is not known. Set LDAP_DEFAULT_POLICY_DN to the DN in olcPPolicyDefault, or give this entry a policy of its own below.'
      default:
        return null
    }
  })()

  const [expirationDate, setExpirationDate] = useState(
    user?.pwdEndTime && hasFutureExpiration
      ? ldapTimestampToDateString(user.pwdEndTime)
      : ''
  )

  // A dropdown, not a searchable picker: a directory has a handful of policies.
  // Bounded and sorted all the same, because an unbounded list endpoint stops at
  // LDAP_SEARCH_SIZE_LIMIT without saying so.
  const { data: passwordPolicies } = useQuery({
    queryKey: ['password-policies', 'options'],
    queryFn: ({ signal }) =>
      api.passwordPolicies.list({ sort: 'cn', order: 'asc', limit: POLICY_OPTIONS_LIMIT }, signal),
    enabled: showPoliciesModule,
  })

  // With no pwdPolicySubentry of its own, an entry follows the directory's
  // default policy — LDAP_DEFAULT_POLICY_DN, since olcPPolicyDefault itself is
  // not readable. Naming it here is the difference between "Default (none)" as
  // "no policy at all" and as "the one every other entry uses".
  const defaultPolicyDn = user?.pwdPolicySubentry ? undefined : user?.lockout?.policyDn

  const policyOptions = useMemo(() => {
    const listed = passwordPolicies?.data ?? []
    const options = [
      { value: '', label: defaultPolicyDn ? `Default (${extractCN(defaultPolicyDn)})` : 'Default (none)' },
      ...listed.map((policy) => ({
        value: policy.dn,
        label: policy.cn + (policy.description ? ` - ${policy.description}` : ''),
      })),
    ]
    // The applied policy may be outside the page we fetched. Without an option
    // for it the select would show a different policy than the one in force.
    const applied = user.pwdPolicySubentry
    if (applied && !listed.some((policy) => policy.dn === applied)) {
      options.push({ value: applied, label: extractCN(applied) })
    }
    return options
  }, [passwordPolicies?.data, user.pwdPolicySubentry, defaultPolicyDn])

  // The policy in force: the entry's own subentry, or the directory default the
  // lockout resolver reports. Looked up in the page already fetched for the
  // dropdown below; when the applied policy falls outside it, the requirements
  // panel stays silent rather than showing another policy's rules.
  const appliedPolicyDn = user?.pwdPolicySubentry || user?.lockout?.policyDn
  const appliedPolicy = useMemo(
    () => passwordPolicies?.data.find((policy) => sameDn(policy.dn, appliedPolicyDn)),
    [passwordPolicies?.data, appliedPolicyDn],
  )

  // Held locally so the change is confirmed, and re-adopted whenever the server
  // value moves under us (another admin, or the refetch after applying).
  // Adjusting state during render is how the rest of the app follows an external
  // value; an effect would cost a render showing the stale choice.
  const serverPolicy = user?.pwdPolicySubentry || ''
  const [policyChoice, setPolicyChoice] = useState(serverPolicy)
  const [lastServerPolicy, setLastServerPolicy] = useState(serverPolicy)
  if (serverPolicy !== lastServerPolicy) {
    setLastServerPolicy(serverPolicy)
    setPolicyChoice(serverPolicy)
  }
  const policyDirty = policyChoice !== serverPolicy

  const policiesNotShown = Math.max(
    0,
    (passwordPolicies?.total ?? 0) - (passwordPolicies?.data.length ?? 0),
  )

  const updatePolicyMutation = useMutation({
    mutationFn: (pwdPolicySubentry: string | undefined) =>
      api.users.update(dn, { pwdPolicySubentry: pwdPolicySubentry || '' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      toast.success('Password policy updated')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const removePasswordMutation = useMutation({
    mutationFn: () => api.users.removePassword(dn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      toast.success('Password removed successfully')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const changePasswordMutation = useMutation({
    mutationFn: (newPassword: string) => api.users.changePassword(dn, newPassword),
    onSuccess: () => {
      setPassword('')
      setConfirmPassword('')
      toast.success('Password changed successfully')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  // Both ask first, as the bulk actions do: a lock turns the account away from
  // every service the directory backs, and an unlock is not an undo — it also
  // clears the failed-bind record ppolicy kept.
  const [lockDialog, setLockDialog] = useState<'lock' | 'unlock' | null>(null)

  const lockMutation = useMutation({
    mutationFn: () => api.users.lock(dn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setLockDialog(null)
      toast.success('Account locked')
    },
  })

  const unlockMutation = useMutation({
    mutationFn: () => api.users.unlock(dn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setLockDialog(null)
      toast.success('Account unlocked')
    },
  })

  const setExpirationMutation = useMutation({
    mutationFn: (date: string | null) => api.users.setExpiration(dn, date),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setEditingExpiration(false)
      toast.success(expirationDate ? 'Expiration date set' : 'Expiration date cleared')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => api.users.delete(dn),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      // Deleting a user also strips their uid from groups and sudo roles. Report
      // what that turned up — a step that failed, or a sudo role left with no
      // user at all — since the account is gone either way and the leftovers are
      // now the operator's to reconcile.
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      toast.success('User deleted')
      showOutcome({ title: `User ${user.uid} deleted: references left behind`, items: res.warnings })
      router.navigate({ to: '/users' })
    },
    // No toast: the confirm dialog stays open and shows the error itself.
  })

  const sendPasswordResetMutation = useMutation({
    mutationFn: () => api.users.sendPasswordReset(dn),
    onSuccess: () => {
      toast.success('Password reset email sent')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handlePasswordChange = (e: React.FormEvent) => {
    e.preventDefault()
    if (password && password === confirmPassword) {
      changePasswordMutation.mutate(password)
    }
  }

  const passwordsMatch = password && confirmPassword && password === confirmPassword
  const passwordsDontMatch = password && confirmPassword && password !== confirmPassword

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Change Password</CardTitle>
          <CardDescription>Set a new password for this user</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handlePasswordChange} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="password">New Password</Label>
              <PasswordInput
                id="password"
                generate={!isSelf}
                value={password}
                onValueChange={setPassword}
                onGenerate={setConfirmPassword}
                disabled={!canWrite}
                autoComplete="new-password"
              />
              <PasswordStrength password={password} />
              {/* The meter above is this app's heuristic; these are the rules
                  the directory will actually enforce. */}
              <PasswordRequirements policy={appliedPolicy} password={password} />
            </div>

            <div className="space-y-2">
              <Label htmlFor="confirmPassword">Confirm Password</Label>
              <PasswordInput
                id="confirmPassword"
                value={confirmPassword}
                onValueChange={setConfirmPassword}
                disabled={!canWrite}
                autoComplete="new-password"
              />
              {passwordsDontMatch && (
                <p className="text-xs text-destructive">Passwords do not match</p>
              )}
              {passwordsMatch && (
                <p className="text-xs text-success">Passwords match</p>
              )}
            </div>

            {canWrite && (
              <div className="flex gap-2">
                <Button
                  type="submit"
                  disabled={!passwordsMatch || changePasswordMutation.isPending}
                >
                  <Key className="h-4 w-4 mr-1" />
                  {changePasswordMutation.isPending ? 'Changing...' : 'Change Password'}
                </Button>
                {isSelf ? (
                  <p className="self-center text-sm text-muted-foreground">
                    Your own password cannot be removed here.
                  </p>
                ) : user.hasPassword ? (
                  <Dialog open={removePasswordDialogOpen} onOpenChange={setRemovePasswordDialogOpen}>
                    <DialogTrigger asChild>
                      <Button type="button" variant="outline" disabled={removePasswordMutation.isPending}>
                        <X className="h-4 w-4 mr-1" />
                        {removePasswordMutation.isPending ? 'Removing...' : 'Remove Password'}
                      </Button>
                    </DialogTrigger>
                    <DialogContent>
                      <DialogHeader>
                        <DialogTitle>Remove Password</DialogTitle>
                        <DialogDescription>
                          Are you sure you want to remove the password for "{user.displayName || user.uid}"?
                          They will no longer be able to log in.
                        </DialogDescription>
                      </DialogHeader>
                      <div className="flex justify-end gap-2">
                        <Button variant="outline" onClick={() => setRemovePasswordDialogOpen(false)}>
                          Cancel
                        </Button>
                        <Button
                          variant="destructive"
                          onClick={() => removePasswordMutation.mutate(undefined, {
                            onSuccess: () => setRemovePasswordDialogOpen(false),
                          })}
                          disabled={removePasswordMutation.isPending}
                        >
                          {removePasswordMutation.isPending ? 'Removing...' : 'Remove Password'}
                        </Button>
                      </div>
                    </DialogContent>
                  </Dialog>
                ) : (
                  <p className="text-sm text-muted-foreground self-center">No password set</p>
                )}
              </div>
            )}
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Mail className="h-5 w-5" />
            Password Reset Email
          </CardTitle>
          <CardDescription>Send a password reset link to the user's email</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {!user?.mail ? (
            <div className="p-3 text-sm text-muted-foreground bg-muted rounded-md">
              This user has no email address configured. Add an email address in the Identity tab to enable password reset emails.
            </div>
          ) : (
            <div className="flex items-center gap-4 p-4 rounded-lg border">
              <div className="p-3 rounded-full bg-primary/10">
                <Mail className="h-6 w-6 text-primary" />
              </div>
              <div className="flex-1">
                <p className="font-medium">Send Reset Link</p>
                <p className="text-sm text-muted-foreground">
                  An email with a password reset link will be sent to {user.mail}
                </p>
              </div>
            </div>
          )}

          {canWrite && user?.mail && (
            <Button
              onClick={() => sendPasswordResetMutation.mutate()}
              disabled={sendPasswordResetMutation.isPending}
            >
              <Mail className="h-4 w-4 mr-1" />
              {sendPasswordResetMutation.isPending ? 'Sending...' : 'Send Password Reset Email'}
            </Button>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Account Status</CardTitle>
          <CardDescription>Lock or unlock user account</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-4 p-4 rounded-lg border">
            <div className={`p-3 rounded-full ${lockedNow || hasPastExpiration ? 'bg-destructive/10' : hasFutureExpiration ? 'bg-warning/15' : 'bg-success/15'}`}>
              {lockedNow ? (
                <Lock className="h-6 w-6 text-destructive" />
              ) : hasFutureExpiration ? (
                <CalendarClock className="h-6 w-6 text-warning" />
              ) : (
                <Unlock className="h-6 w-6 text-success" />
              )}
            </div>
            <div className="flex-1">
              <p className="font-medium">
                {lockedNow
                  ? 'Account Locked'
                  : hasPastExpiration
                    ? 'Account Expired'
                    : lapsedLock
                      ? 'Account Active (Lockout Expired)'
                      : hasFutureExpiration
                        ? 'Account Active (Expires Soon)'
                        : 'Account Active'}
              </p>
              <p className="text-sm text-muted-foreground">
                {lockedNow
                  ? 'User cannot log in until the account is unlocked'
                  : hasPastExpiration
                    ? 'The directory refuses this account: its expiry has passed'
                    : lapsedLock
                      ? 'The lockout has run its course, so the user can log in again'
                      : hasFutureExpiration
                        ? 'The directory will refuse this account from the expiry date'
                        : 'User can log in normally'}
              </p>
              {hasLockFlag && lockInstant && (
                <p className="text-xs text-muted-foreground mt-1">
                  {lapsedLock ? 'Was locked on' : 'Locked on'}: {lockInstant.toLocaleString()}
                </p>
              )}
              {lockDetail && (
                <p className="text-xs text-muted-foreground mt-1">{lockDetail}</p>
              )}
              {(hasFutureExpiration || hasPastExpiration) && (
                <p className="text-xs text-warning mt-1" title={expiryInstantTitle(user.pwdEndTime!)}>
                  {hasPastExpiration ? 'Expired on' : 'Expires on'}: {formatExpiryDate(user.pwdEndTime!)}
                </p>
              )}
            </div>
          </div>

          {canWrite && isSelf && !hasLockFlag && (
            <Callout variant="info">
              This is the account you are signed in with, so it cannot be locked here.
            </Callout>
          )}

          {canWrite && !(isSelf && !hasLockFlag) && (
            <div className="flex gap-2 flex-wrap">
              {hasLockFlag ? (
                <Button variant="default" onClick={() => setLockDialog('unlock')}>
                  <Unlock className="h-4 w-4 mr-1" />
                  Unlock Account
                </Button>
              ) : (
                <Button variant="destructive" onClick={() => setLockDialog('lock')}>
                  <Lock className="h-4 w-4 mr-1" />
                  Lock Account
                </Button>
              )}
            </div>
          )}

          <ConfirmDialog
            open={lockDialog !== null}
            onOpenChange={(open) => {
              if (!open) {
                setLockDialog(null)
                lockMutation.reset()
                unlockMutation.reset()
              }
            }}
            title={lockDialog === 'unlock' ? 'Unlock account' : 'Lock account'}
            description={
              lockDialog === 'unlock'
                ? `Unlock ${user.uid}? Any lockout the password policy applied after failed sign-ins is cleared too, along with the record of those attempts.`
                : `Lock ${user.uid}? The account will be refused by every service that authenticates against this directory until someone unlocks it.`
            }
            confirmLabel={lockDialog === 'unlock' ? 'Unlock' : 'Lock'}
            pendingLabel={lockDialog === 'unlock' ? 'Unlocking...' : 'Locking...'}
            confirmVariant={lockDialog === 'unlock' ? 'default' : 'destructive'}
            error={(lockDialog === 'unlock' ? unlockMutation.error : lockMutation.error)?.message}
            isPending={lockMutation.isPending || unlockMutation.isPending}
            onConfirm={() => (lockDialog === 'unlock' ? unlockMutation.mutate() : lockMutation.mutate())}
          />

          {/* The controls above write attributes the directory only honours while a
              password policy applies to the entry. When we know it does not, say so
              here rather than let the status column claim a lock that isn't one. An
              entry carrying its own pwdPolicySubentry is covered whatever the
              directory-wide answer is, so it is exempt. */}
          {policyEnforcement === 'not-enforced' && !user.pwdPolicySubentry && (
            <div className="pt-4 border-t">
              <Callout variant="error">
                No password policy applies to this directory, so locking and account expiry are
                written but <strong>not enforced</strong> — the account will keep authenticating.
                Set <code>olcPPolicyDefault</code> on the ppolicy overlay, or a{' '}
                <code>pwdPolicySubentry</code> on this entry.
              </Callout>
            </div>
          )}

          {/* An expiration writes pwdEndTime, not the lock attribute, but the
              directory refuses the bind either way — so on your own account it is
              the same lockout by another name and the API refuses it (see
              refuseSelfTarget). Clearing an existing one stays available. */}
          {!canExpire && canWrite && (
            <div className="pt-4 border-t">
              <Callout variant="info">
                This directory cannot schedule account expiry: its ppolicy overlay has no{' '}
                <code>pwdEndTime</code> (OpenLDAP 2.5 or later). Locking is unaffected.
              </Callout>
            </div>
          )}

          {canExpire && canWrite && isSelf && hasFutureExpiration && (
            <div className="pt-4 border-t space-y-2">
              <Label className="flex items-center gap-2">
                <CalendarClock className="h-4 w-4" />
                Account Expiration
              </Label>
              <Callout variant="warning">
                This account expires on{' '}
                <span title={expiryInstantTitle(user.pwdEndTime!)}>
                  {formatExpiryDate(user.pwdEndTime!)}
                </span>
                . It is the account you are signed in with, so only clearing the date is available
                here.
              </Callout>
              <Button
                variant="outline"
                onClick={() => setExpirationMutation.mutate(null)}
                disabled={setExpirationMutation.isPending}
              >
                {setExpirationMutation.isPending ? 'Clearing...' : 'Clear expiration'}
              </Button>
            </div>
          )}

          {canExpire && canWrite && isSelf && !hasFutureExpiration && (
            <div className="pt-4 border-t">
              <Callout variant="info">
                Setting an expiration date on the account you are signed in with would lock you out,
                so it is not available here.
              </Callout>
            </div>
          )}

          {canExpire && canWrite && !isSelf && (
            <div className="pt-4 border-t">
              <div className="flex items-center justify-between mb-2">
                <Label className="flex items-center gap-2">
                  <CalendarClock className="h-4 w-4" />
                  Account Expiration
                </Label>
                {hasFutureExpiration && !editingExpiration && (
                  <Button variant="ghost" size="sm" onClick={() => setEditingExpiration(true)}>
                    Edit
                  </Button>
                )}
              </div>

              {editingExpiration || !hasFutureExpiration ? (
                <div className="flex gap-2 items-end">
                  <div className="flex-1">
                    <DatePicker
                      value={expirationDate}
                      onChange={setExpirationDate}
                      min={new Date().toISOString().split('T')[0]}
                    />
                    {/* Say which day boundary is meant. pwdEndTime is an instant,
                        and "the 20th" has to end somewhere. */}
                    <p className="text-xs text-muted-foreground mt-1">
                      The account stops working at the end of this day, UTC.
                    </p>
                  </div>
                  <Button
                    variant="outline"
                    onClick={() => setExpirationMutation.mutate(expirationDate || null)}
                    disabled={setExpirationMutation.isPending}
                  >
                    {setExpirationMutation.isPending ? 'Saving...' : expirationDate ? 'Set Expiration' : 'Clear'}
                  </Button>
                  {editingExpiration && (
                    <Button
                      variant="ghost"
                      onClick={() => {
                        setEditingExpiration(false)
                        if (user?.pwdEndTime && hasFutureExpiration) {
                          setExpirationDate(ldapTimestampToDateString(user.pwdEndTime))
                        }
                      }}
                    >
                      Cancel
                    </Button>
                  )}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">
                  This account is scheduled to expire on {formatExpiryDate(user.pwdEndTime!)}.
                </p>
              )}
              <p className="text-xs text-muted-foreground mt-2">
                Set a future date to automatically lock this account. Leave empty for no expiration.
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      {showPoliciesModule && (
        <Card>
          <CardHeader>
            <CardTitle>Password Policy Status</CardTitle>
            <CardDescription>Password policy information (ppolicy overlay)</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {hasFutureExpiration && (
              <div className="flex items-center gap-3 p-3 rounded-lg bg-warning/10 border border-warning/30">
                <CalendarClock className="h-5 w-5 text-warning shrink-0" />
                <div>
                  <p className="font-medium text-warning">Scheduled Expiration</p>
                  <p className="text-sm text-muted-foreground">
                    Account expires on: {formatExpiryDate(user.pwdEndTime!)}
                  </p>
                </div>
              </div>
            )}

            {hasLockFlag && lockedNow && (
              <div className="flex items-center gap-3 p-3 rounded-lg bg-destructive/10 border border-destructive/30">
                <Lock className="h-5 w-5 text-destructive shrink-0" />
                <div>
                  <p className="font-medium text-destructive">
                    {user?.lockout?.state === 'manual'
                      ? 'Locked by an Administrator'
                      : user?.lockout?.state === 'unknown'
                        ? 'Locked, Duration Unknown'
                        : 'Policy Locked'}
                  </p>
                  <p className="text-sm text-muted-foreground">
                    {lockInstant
                      ? `Locked at: ${lockInstant.toLocaleString()}`
                      : user?.lockout?.state === 'manual'
                        ? 'No lock date: the directory stores a permanent-lock marker, not an instant'
                        : `Locked at: ${user?.pwdAccountLockedTime} — not a readable date`}
                    {liftsAt && ` — lifts at ${liftsAt.toLocaleString()}`}
                  </p>
                </div>
              </div>
            )}

            {/* A lockout the policy has already lifted. It is not a lock any more,
                so it is not shown as one — but the attribute is still there, and
                anything reading the entry directly will report the account as
                locked until the next bind clears it. */}
            {lapsedLock && (
              <div className="flex items-center gap-3 p-3 rounded-lg bg-warning/10 border border-warning/30">
                <Unlock className="h-5 w-5 text-warning shrink-0" />
                <div>
                  <p className="font-medium text-warning">Lockout Expired</p>
                  <p className="text-sm text-muted-foreground">
                    {lockInstant && `Locked at: ${lockInstant.toLocaleString()}`}
                    {liftsAt && `${lockInstant ? ', lifted' : 'Lifted'} at ${liftsAt.toLocaleString()}`}
                  </p>
                </div>
              </div>
            )}

            {user?.pwdReset && (
              <div className="flex items-center gap-3 p-3 rounded-lg bg-warning/10 border border-warning/30">
                <AlertTriangle className="h-5 w-5 text-warning shrink-0" />
                <div>
                  <p className="font-medium text-warning">Password Reset Required</p>
                  <p className="text-sm text-muted-foreground">
                    User must change password on next login
                  </p>
                </div>
              </div>
            )}

            <FieldRow className="text-sm">
              <div>
                <p className="text-muted-foreground">Password Changed</p>
                <p className="font-medium">
                  {user?.pwdChangedTime ? formatLdapTimestamp(user.pwdChangedTime) : 'N/A'}
                </p>
              </div>
              <div>
                <Label className="text-muted-foreground">Applied Policy</Label>
                {canWrite ? (
                  <>
                    <Select
                      value={policyChoice}
                      onChange={(e) => setPolicyChoice(e.target.value)}
                      options={policyOptions}
                      className="mt-1"
                      disabled={updatePolicyMutation.isPending}
                    />
                    {/* Confirmed rather than written on `change`: every other
                        field on this page needs an explicit save, and on a
                        closed native select an arrow key is enough to move the
                        selection — which would silently rewrite the policy that
                        governs the account. */}
                    {policyDirty && (
                      <div className="mt-2 flex flex-wrap items-center gap-2">
                        <Button
                          size="sm"
                          onClick={() => updatePolicyMutation.mutate(policyChoice || undefined)}
                          disabled={updatePolicyMutation.isPending}
                        >
                          {updatePolicyMutation.isPending ? 'Applying...' : 'Apply policy'}
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setPolicyChoice(serverPolicy)}
                          disabled={updatePolicyMutation.isPending}
                        >
                          Cancel
                        </Button>
                      </div>
                    )}
                  </>
                ) : (
                  <p className="font-medium font-mono text-xs mt-1">
                    {user?.pwdPolicySubentry ? extractCN(user.pwdPolicySubentry) : 'Default'}
                  </p>
                )}
                {canWrite && policiesNotShown > 0 && (
                  <p className="text-xs text-muted-foreground mt-1">
                    {policiesNotShown} further {policiesNotShown === 1 ? 'policy is' : 'policies are'} not
                    listed here — the directory returned as many as it will.
                  </p>
                )}
              </div>
            </FieldRow>

            {user?.pwdFailureTime && user.pwdFailureTime.length > 0 && (
              <div className="space-y-2">
                <p className="text-sm text-muted-foreground">Recent Failed Logins</p>
                <div className="flex flex-wrap gap-2">
                  {user.pwdFailureTime.slice(-5).map((time, i) => (
                    <Badge key={i} variant="outline" className="text-xs">
                      {formatLdapTimestamp(time)}
                    </Badge>
                  ))}
                  {user.pwdFailureTime.length > 5 && (
                    <Badge variant="secondary" className="text-xs">
                      +{user.pwdFailureTime.length - 5} more
                    </Badge>
                  )}
                </div>
              </div>
            )}

            {user?.pwdGraceUseTime && user.pwdGraceUseTime.length > 0 && (
              <div className="space-y-2">
                <p className="text-sm text-muted-foreground">Grace Logins Used</p>
                <div className="flex flex-wrap gap-2">
                  {user.pwdGraceUseTime.map((time, i) => (
                    <Badge key={i} variant="outline" className="text-xs">
                      {formatLdapTimestamp(time)}
                    </Badge>
                  ))}
                </div>
              </div>
            )}

            {!user?.pwdAccountLockedTime && !user?.pwdReset && (!user?.pwdFailureTime || user.pwdFailureTime.length === 0) && !user?.pwdChangedTime && (
              <p className="text-sm text-muted-foreground">
                No password policy data available. This may indicate the ppolicy overlay is not enabled or this user has no policy-related activity.
              </p>
            )}
          </CardContent>
        </Card>
      )}

      {canWrite && isSelf && (
        <Card className="md:col-span-2">
          <CardHeader>
            <CardTitle>Change username</CardTitle>
            <CardDescription>Not available for your own account</CardDescription>
          </CardHeader>
          <CardContent>
            <Callout variant="info">
              Renaming an account changes its directory identity, which would invalidate the session
              you are using. Ask another administrator to rename it.
            </Callout>
          </CardContent>
        </Card>
      )}

      {canWrite && !isSelf && (
        <Card className="md:col-span-2 border-warning/50">
          <CardHeader>
            <CardTitle>Change username</CardTitle>
            <CardDescription>Rename the account's uid — its directory identity</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex items-center justify-between p-4 rounded-lg border border-warning/30 bg-warning/5">
              <div>
                <p className="font-medium">Change this user's username</p>
                <p className="text-sm text-muted-foreground">
                  Renames the LDAP entry and migrates group, sudo, manager and home-directory
                  references. The user must sign in again.
                </p>
              </div>
              <Dialog
                open={changeUidOpen}
                onOpenChange={(o) => {
                  setChangeUidOpen(o)
                  if (!o) { setNewUid(''); setConfirmUid('') }
                }}
              >
                <DialogTrigger asChild>
                  <Button variant="outline">Change username</Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Change username</DialogTitle>
                    <DialogDescription>
                      Renaming <span className="font-mono">{user.uid}</span> cannot be undone automatically.
                    </DialogDescription>
                  </DialogHeader>
                  <div className="space-y-4">
                    <Callout variant="warning">
                      <p className="font-medium">This will:</p>
                      <ul className="list-disc pl-5 mt-1 space-y-0.5">
                        <li>rename the directory entry (its DN changes)</li>
                        <li>migrate group memberships, sudo rules, manager links and a <code>/home/&lt;uid&gt;</code> home directory</li>
                        <li>sign the user out of all active sessions</li>
                      </ul>
                      <p className="mt-2">References outside those (external systems, scripts, ACLs) keep pointing at the old username.</p>
                    </Callout>
                    <div className="space-y-2">
                      <Label htmlFor="newUid">New username</Label>
                      <Input
                        id="newUid"
                        value={newUid}
                        onChange={(e) => setNewUid(e.target.value)}
                        placeholder="new-username"
                        autoComplete="off"
                      />
                      {newUid && !uidPattern.test(newUid) && (
                        <p className="text-xs text-destructive">Only letters, digits, '.', '_' or '-' are allowed.</p>
                      )}
                      {newUid && newUid === user.uid && (
                        <p className="text-xs text-destructive">This is already the current username.</p>
                      )}
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="confirmUid">
                        Type the current username <span className="font-mono">{user.uid}</span> to confirm
                      </Label>
                      <Input
                        id="confirmUid"
                        value={confirmUid}
                        onChange={(e) => setConfirmUid(e.target.value)}
                        autoComplete="off"
                      />
                    </div>
                  </div>
                  <DialogFooter>
                    <Button variant="outline" onClick={() => setChangeUidOpen(false)}>
                      Cancel
                    </Button>
                    <Button
                      variant="destructive"
                      disabled={!newUidValid || confirmUid !== user.uid || changeUidMutation.isPending}
                      onClick={() => changeUidMutation.mutate(newUid)}
                    >
                      {changeUidMutation.isPending ? 'Renaming...' : 'Change username'}
                    </Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </div>
          </CardContent>
        </Card>
      )}

      {canDelete && isSelf && (
        <Card className="md:col-span-2">
          <CardHeader>
            <CardTitle>Danger Zone</CardTitle>
            <CardDescription>Not available for your own account</CardDescription>
          </CardHeader>
          <CardContent>
            <Callout variant="info">
              You cannot delete the account you are signed in with. Ask another administrator, or
              sign in with a different account.
            </Callout>
          </CardContent>
        </Card>
      )}

      {canDelete && !isSelf && (
        <Card className="md:col-span-2 border-destructive/50">
          <CardHeader>
            <CardTitle className="text-destructive">Danger Zone</CardTitle>
            <CardDescription>Irreversible actions</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex items-center justify-between p-4 rounded-lg border border-destructive/30 bg-destructive/5">
              <div>
                <p className="font-medium">Delete this user</p>
                <p className="text-sm text-muted-foreground">
                  Permanently remove this user from the directory. This action cannot be undone.
                </p>
              </div>
              <ConfirmDialog
                open={deleteDialogOpen}
                onOpenChange={setDeleteDialogOpen}
                trigger={
                  <Button variant="destructive">
                    <Trash2 className="h-4 w-4 mr-1" />
                    Delete User
                  </Button>
                }
                title="Delete User"
                description={
                  <>
                    Delete {user.displayName || user.uid} ({user.uid})? The account is removed, and
                    its uid is taken out of every group&apos;s members, every sudo rule and every
                    netgroup that names it — what cannot be reached is listed afterwards. This
                    cannot be undone.
                  </>
                }
                confirmText={user.uid}
                confirmLabel="Delete User"
                error={deleteMutation.error?.message}
                isPending={deleteMutation.isPending}
                onConfirm={() => deleteMutation.mutate()}
              />
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
