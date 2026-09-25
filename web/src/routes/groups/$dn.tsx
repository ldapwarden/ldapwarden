import { DetailSkeleton } from '@/components/ui/skeleton'
import { DNLine } from '@/components/dn-line'
import { DetailLoadError } from '@/components/detail-load-error'
import { FormActions } from '@/components/form-actions'
import { BackToList } from '@/components/back-to-list'
import { createFileRoute, redirect, useRouter } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { showOutcome } from '@/lib/outcome'
import { api } from '@/lib/api'
import { useEnabledModules, useEnabledObjects } from '@/lib/use-server-info'
import { useAuth } from '@/lib/auth'
import { UnsavedChangesGuard } from '@/components/unsaved-changes-guard'
import { useSyncedForm } from '@/lib/form-sync'
import { PickerOverflow } from '@/components/ui/picker-overflow'
import { usePickerSearch } from '@/lib/use-picker-search'
import { userPhotoUrl, type GroupMember, type UserListItem } from '@/lib/api'
import { decodeDN } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { FieldRow } from '@/components/ui/field-row'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Avatar } from '@/components/ui/avatar'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Callout } from '@/components/ui/callout'
import { ReadOnlyNotice } from '@/components/read-only-notice'
import { GroupRenameCard } from '@/components/group-rename'
import { AccountStatus } from '@/components/account-status'
import { UserPlus, UserMinus, Users, Search, Info, Shield, Trash2, ShieldCheck, Plus, X } from 'lucide-react'
import { useState, useMemo } from 'react'

// Tabs this page can show. Which ones are actually rendered depends on the
// configured object classes and modules; an unknown or disabled tab in the URL
// falls back to the first available one.
const TABS = ['information', 'members', 'samba', 'sudo', 'security'] as const
type Tab = (typeof TABS)[number]

export const Route = createFileRoute('/groups/$dn')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  validateSearch: (raw: Record<string, unknown>): { tab?: Tab } => ({
    tab: typeof raw.tab === 'string' && (TABS as readonly string[]).includes(raw.tab)
      ? (raw.tab as Tab)
      : undefined,
  }),
  component: GroupDetailPage,
})

function GroupDetailPage() {
  const { dn: encodedDN } = Route.useParams()
  const dn = decodeDN(encodedDN)
  const router = useRouter()
  const { hasPermission } = useAuth()
  const queryClient = useQueryClient()
  const canWrite = hasPermission('groups:write')
  const canDelete = hasPermission('groups:delete')

  const [newMember, setNewMember] = useState('')
  const [addMemberOpen, setAddMemberOpen] = useState(false)

  const { data: group, isLoading, error } = useQuery({
    queryKey: ['group', dn],
    queryFn: ({ signal }) => api.groups.get(dn, signal),
  })

  // The membership, resolved to names by the directory. The browser used to do
  // this by downloading every user and matching uids locally, which only worked
  // as far as LDAP_SEARCH_SIZE_LIMIT.
  const { data: membersData } = useQuery({
    queryKey: ['group', dn, 'members'],
    queryFn: ({ signal }) => api.groups.members(dn, signal),
  })

  // From /server-info rather than /admin/config, which takes settings:read: a
  // read-only session could not read it and silently fell back to a hardcoded
  // list instead of the deployment's own, so it saw the wrong tabs.
  const enabledObjects = useEnabledObjects('groups', ['posixgroup'])

  const enabledModules = new Set(useEnabledModules().map((m) => m.toLowerCase()))

  const showPosixGroupTabs = enabledObjects.has('posixgroup')
  const showSambaTab = enabledObjects.has('sambagroupmapping')
  const showSudoTab = enabledModules.has('sudo')

  // Check which objectClasses the group actually has
  const groupObjectClasses = useMemo(() => {
    const classes = new Set((group?.objectClasses || []).map(oc => oc.toLowerCase()))
    return classes
  }, [group?.objectClasses])

  const hasSambaGroupMapping = groupObjectClasses.has('sambagroupmapping')

  // Compute default tab from config
  const defaultTab = useMemo(() => {
    if (showPosixGroupTabs) return 'information'
    if (showSambaTab) return 'samba'
    if (showSudoTab) return 'sudo'
    return 'security'
  }, [showPosixGroupTabs, showSambaTab, showSudoTab])

  // The active tab lives in the URL. `replace` keeps tab switching out of the
  // history stack, so Back leaves the page rather than walking tabs.
  const navigate = Route.useNavigate()
  const { tab } = Route.useSearch()
  const activeTab = tab ?? defaultTab
  const setActiveTab = (next: string) => {
    navigate({ search: { tab: next as Tab }, replace: true })
  }

  const updateMutation = useMutation({
    mutationFn: (description: string) => api.groups.update(dn, { description }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      queryClient.invalidateQueries({ queryKey: ['group', dn] })
      toast.success('Group updated successfully')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const addMemberMutation = useMutation({
    mutationFn: (memberUid: string) => api.groups.addMember(dn, memberUid),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['group', dn] })
      // Keep the dialog open and the search intact, clearing only the current
      // selection, so several members can be added in a row without reopening.
      setNewMember('')
      toast.success('Member added to group')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const removeMemberMutation = useMutation({
    mutationFn: (memberUid: string) => api.groups.removeMember(dn, memberUid),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['group', dn] })
      toast.success('Member removed from group')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => api.groups.delete(dn),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      toast.success('Group deleted')
      // Sudo grants to "%cn" and primary members hold the gidNumber: neither
      // goes away with the entry, and the directory will not say so.
      showOutcome({ title: `Group ${group?.cn ?? ''} deleted: references left behind`, items: res.warnings })
      router.navigate({ to: '/groups' })
    },
    // No toast: the confirm dialog stays open and shows the error itself.
  })

  const handleSubmit = (e: React.FormEvent, description: string) => {
    e.preventDefault()
    updateMutation.mutate(description)
  }

  // Candidates come from the directory, not from a local copy of it. The
  // headroom covers the members filtered out just below: without it a term whose
  // first hits are all already in the group would show an empty picker.
  const memberPicker = usePickerSearch(['users', 'picker', dn], api.users.list, {
    sort: 'displayName',
    headroom: group?.memberUid?.length ?? 0,
    enabled: addMemberOpen,
  })

  const alreadyMembers = useMemo(
    () => new Set(group?.memberUid ?? []),
    [group?.memberUid],
  )
  const availableUsers = useMemo(
    () => memberPicker.items.filter((user) => !alreadyMembers.has(user.uid)),
    [memberPicker.items, alreadyMembers],
  )

  if (isLoading) {
    return <DetailSkeleton cards={2} />
  }

  if (error) {
    return <DetailLoadError error={error} noun="group" list="/groups" listLabel="groups" />
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <BackToList to="/groups" label="Back to groups" />
        <div>
          <h1 className="text-2xl font-bold">{group?.cn}</h1>
          <DNLine dn={group?.dn} />
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          {showPosixGroupTabs && (
            <TabsTrigger value="information" className="flex items-center gap-1">
              <Info className="h-4 w-4" />
              Information
            </TabsTrigger>
          )}
          {showPosixGroupTabs && (
            <TabsTrigger value="members" className="flex items-center gap-1">
              <Users className="h-4 w-4" />
              Members
            </TabsTrigger>
          )}
          {showSambaTab && (
            <TabsTrigger value="samba" className="flex items-center gap-1">
              <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M4 22h14a2 2 0 0 0 2-2V7.5L14.5 2H6a2 2 0 0 0-2 2v4" />
                <polyline points="14 2 14 8 20 8" />
                <path d="M2 15h10" />
                <path d="m9 18 3-3-3-3" />
              </svg>
              Samba
            </TabsTrigger>
          )}
          {showSudoTab && (
            <TabsTrigger value="sudo" className="flex items-center gap-1">
              <ShieldCheck className="h-4 w-4" />
              Sudo
            </TabsTrigger>
          )}
          <TabsTrigger value="security" className="flex items-center gap-1">
            <Shield className="h-4 w-4" />
            Security
          </TabsTrigger>
        </TabsList>

        {showPosixGroupTabs && (
          <TabsContent value="information">
            <InformationTab
              group={group!}
              canWrite={canWrite}
              updateMutation={updateMutation}
              onSubmit={handleSubmit}
            />
          </TabsContent>
        )}

        {showPosixGroupTabs && (
          <TabsContent value="members">
            <MembersTab
              group={group!}
              members={membersData?.data}
              availableUsers={availableUsers}
              picker={memberPicker}
              canWrite={canWrite}
              addMemberOpen={addMemberOpen}
              setAddMemberOpen={setAddMemberOpen}
              newMember={newMember}
              setNewMember={setNewMember}
              addMemberMutation={addMemberMutation}
              removeMemberMutation={removeMemberMutation}
            />
          </TabsContent>
        )}

        {showSambaTab && (
          <TabsContent value="samba">
            <SambaTab group={group!} dn={dn} canWrite={canWrite} hasObjectClass={hasSambaGroupMapping} />
          </TabsContent>
        )}

        {showSudoTab && (
          <TabsContent value="sudo">
            <SudoTab group={group!} dn={dn} canWrite={canWrite} />
          </TabsContent>
        )}

        <TabsContent value="security">
          <SecurityTab
            group={group!}
            canDelete={canDelete}
            deleteMutation={deleteMutation}
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// Information Tab
function InformationTab({
  group,
  canWrite,
  updateMutation,
  onSubmit,
}: {
  group: NonNullable<ReturnType<typeof api.groups.get> extends Promise<infer T> ? T : never>
  canWrite: boolean
  updateMutation: ReturnType<typeof useMutation<unknown, Error, string>>
  onSubmit: (e: React.FormEvent, description: string) => void
}) {
  const [form, setForm] = useState(() => ({ description: group.description || '' }))
  // Adopts the server's copy on refetch while untouched. Comparing against the
  // value captured at mount instead left the field stale after a background
  // refetch, and claiming unsaved changes that were really someone else's edit.
  const isDirty = useSyncedForm({ description: group.description || '' }, form, setForm)
  const description = form.description
  const setDescription = (value: string) => setForm({ description: value })

  return (
    <form onSubmit={(e) => onSubmit(e, description)} className="space-y-4">
      {/* The guard lives here rather than on the page: this is where the edited
          value is held, and the tab unmounts when you switch away — which is
          exactly the navigation worth intercepting. */}
      <UnsavedChangesGuard when={isDirty} />
      {!canWrite && <ReadOnlyNotice />}
      <Card>
        <CardHeader>
          <CardTitle>Group Information</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="cn">Name (CN)</Label>
            <Input id="cn" value={group?.cn || ''} disabled />
            <p className="text-xs text-muted-foreground">
              The name is the entry's identity, so changing it renames the entry. Use “Change
              name” below.
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="gidNumber">GID Number</Label>
            <Input id="gidNumber" value={group?.gidNumber || ''} disabled />
            <p className="text-xs text-muted-foreground">The GID cannot be changed after creation.</p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="description">Description</Label>
            <Input
              id="description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={!canWrite}
            />
          </div>

          {canWrite && (
            <div className="flex justify-end gap-2 pt-4">
              <FormActions
                dirty={isDirty}
                pending={updateMutation.isPending}
                onDiscard={() => setForm({ description: group.description || '' })}
              />
            </div>
          )}
        </CardContent>
      </Card>

      {canWrite && <GroupRenameCard dn={group.dn} cn={group.cn} />}
    </form>
  )
}

// Members Tab
function MembersTab({
  group,
  members,
  availableUsers,
  picker,
  canWrite,
  addMemberOpen,
  setAddMemberOpen,
  newMember,
  setNewMember,
  addMemberMutation,
  removeMemberMutation,
}: {
  group: NonNullable<ReturnType<typeof api.groups.get> extends Promise<infer T> ? T : never>
  members: GroupMember[] | undefined
  // The list endpoint's own row type rather than a hand-written subset of it:
  // the subset silently dropped the lockout fields, so the picker could not say
  // that a candidate is locked even though the payload carried it.
  availableUsers: UserListItem[]
  picker: ReturnType<typeof usePickerSearch<UserListItem>>
  canWrite: boolean
  addMemberOpen: boolean
  setAddMemberOpen: (open: boolean) => void
  newMember: string
  setNewMember: (uid: string) => void
  addMemberMutation: ReturnType<typeof useMutation<unknown, Error, string>>
  removeMemberMutation: ReturnType<typeof useMutation<unknown, Error, string>>
}) {
  // Confirm before removing a member: the uid being confirmed drives a single
  // controlled dialog rather than one per row.
  const [confirmRemoveUid, setConfirmRemoveUid] = useState<string | null>(null)

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="flex items-center gap-2">
          <Users className="h-5 w-5" />
          Members ({group?.memberUid?.length ?? 0})
        </CardTitle>
        {canWrite && (
          <Dialog open={addMemberOpen} onOpenChange={setAddMemberOpen}>
            <DialogTrigger asChild>
              <Button size="sm">
                <UserPlus className="h-4 w-4 mr-1" />
                Add Member
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-md">
              <DialogHeader>
                <DialogTitle>Add Member</DialogTitle>
                <DialogDescription>
                  Select a user to add to this group.
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4">
                <div className="relative">
                  <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                  <Input
                    placeholder="Search users..."
                    value={picker.input}
                    onChange={(e) => picker.setInput(e.target.value)}
                    className="pl-8"
                  />
                </div>
                <div className="max-h-64 overflow-y-auto space-y-1">
                  {availableUsers.length > 0 ? (
                    availableUsers.map((user) => (
                      <button
                        key={user.uid}
                        type="button"
                        // The highlight is colour alone; this says it to a screen reader.
                        aria-pressed={newMember === user.uid}
                        onClick={() => setNewMember(user.uid)}
                        className={`w-full flex items-center gap-3 p-2 rounded-md text-left transition-colors ${
                          newMember === user.uid
                            ? 'bg-primary text-primary-foreground'
                            : 'hover:bg-muted'
                        }`}
                      >
                        <Avatar
                          url={user.hasPhoto ? userPhotoUrl(user.dn) : undefined}
                          fallback={user.displayName || user.cn}
                          size="sm"
                        />
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-1.5">
                            <span className="font-medium truncate">
                              {user.displayName || user.cn}
                            </span>
                            {/* Adding a locked account to a group is rarely what
                                was meant; say so before the click, not after. */}
                            <AccountStatus user={user} quiet />
                          </div>
                          <div className={`text-sm truncate ${
                            newMember === user.uid
                              ? 'text-primary-foreground/70'
                              : 'text-muted-foreground'
                          }`}>
                            {user.uid}
                          </div>
                        </div>
                      </button>
                    ))
                  ) : (
                    <p className="text-sm text-muted-foreground text-center py-4">
                      {picker.isFetching
                        ? 'Searching...'
                        : picker.input
                          ? 'No user matches this search.'
                          : 'Every user is already a member.'}
                    </p>
                  )}
                </div>
                <PickerOverflow count={picker.overflow} />
              </div>
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => {
                  setAddMemberOpen(false)
                  setNewMember('')
                  picker.reset()
                }}>
                  Close
                </Button>
                <Button
                  onClick={() => addMemberMutation.mutate(newMember)}
                  disabled={!newMember || addMemberMutation.isPending}
                >
                  {addMemberMutation.isPending ? 'Adding...' : 'Add'}
                </Button>
              </div>
            </DialogContent>
          </Dialog>
        )}
      </CardHeader>
      <CardContent>
        {group?.memberUid && group.memberUid.length > 0 ? (
          <ul className="space-y-2">
            {/* The resolved membership when it has arrived, the raw uids until
                then, so the list never waits on the name lookup to render. */}
            {(members ?? [...group.memberUid].sort((a, b) => a.localeCompare(b)).map((uid) => ({ uid }) as GroupMember)).map((member) => {
              const uid = member.uid
              const name = member.displayName || member.cn
              return (
                <li
                  key={uid}
                  className="flex items-center justify-between p-2 rounded-md bg-muted"
                >
                  <div className="flex items-center gap-3">
                    <Avatar
                      url={member.hasPhoto && member.dn ? userPhotoUrl(member.dn) : undefined}
                      fallback={name || uid}
                      size="sm"
                    />
                    <div>
                      <span className="flex items-center gap-1.5">
                        <span className="font-medium">{uid}</span>
                        {/* Quiet: only a member who cannot use what this group
                            grants is worth stopping the eye on. */}
                        <AccountStatus user={member} quiet />
                      </span>
                      {name && (
                        <span className="text-sm text-muted-foreground">
                          {name}
                        </span>
                      )}
                    </div>
                  </div>
                  {canWrite && (
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Remove member"
                      onClick={() => setConfirmRemoveUid(uid)}
                      disabled={removeMemberMutation.isPending}
                    >
                      <UserMinus className="h-4 w-4 text-destructive" />
                    </Button>
                  )}
                </li>
              )
            })}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground text-center py-4">
            No members in this group
          </p>
        )}
      </CardContent>
      <ConfirmDialog
        open={confirmRemoveUid !== null}
        onOpenChange={(open) => { if (!open) setConfirmRemoveUid(null) }}
        title="Remove member"
        description={`Remove "${confirmRemoveUid}" from group "${group.cn}"? They will lose the access this group grants.`}
        confirmLabel="Remove"
        pendingLabel="Removing..."
        error={removeMemberMutation.error?.message}
        isPending={removeMemberMutation.isPending}
        onConfirm={() => {
          if (confirmRemoveUid) removeMemberMutation.mutate(confirmRemoveUid)
          setConfirmRemoveUid(null)
        }}
      />
    </Card>
  )
}

function groupSambaForm(group: { sambaSID?: string; sambaGroupType?: string; displayName?: string }) {
  return {
    sambaSID: group.sambaSID || '',
    sambaGroupType: group.sambaGroupType || '',
    displayName: group.displayName || '',
  }
}

// Samba Tab (sambaGroupMapping)
function SambaTab({ group, dn, canWrite, hasObjectClass }: { group: NonNullable<ReturnType<typeof api.groups.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean, hasObjectClass: boolean }) {
  const queryClient = useQueryClient()
  const [isAddingObjectClass, setIsAddingObjectClass] = useState(false)

  const [formData, setFormData] = useState(() => groupSambaForm(group))
  // As on the user's Samba tab: synced only once the entry carries the object
  // class, since in add mode the server copy is all empty strings and adopting
  // it would wipe the defaults just filled in.
  const isDirty = useSyncedForm(
    hasObjectClass && !isAddingObjectClass ? groupSambaForm(group) : undefined,
    formData,
    setFormData,
  )

  // Initialize form with default values when entering add mode
  const handleAddSambaGroup = () => {
    setFormData({
      sambaSID: `S-1-5-21-0-0-0-${group.gidNumber}`,
      sambaGroupType: '2',
      displayName: group.cn || '',
    })
    setIsAddingObjectClass(true)
  }

  const updateMutation = useMutation({
    mutationFn: () => {
      const updateData: Record<string, string | undefined> = {}
      if (formData.sambaSID !== (group?.sambaSID || '')) updateData.sambaSID = formData.sambaSID
      if (formData.sambaGroupType !== (group?.sambaGroupType || '')) updateData.sambaGroupType = formData.sambaGroupType
      if (formData.displayName !== (group?.displayName || '')) updateData.displayName = formData.displayName
      return api.groups.updateSamba(dn, updateData)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      queryClient.invalidateQueries({ queryKey: ['group', dn] })
      setIsAddingObjectClass(false)
      toast.success(isAddingObjectClass ? 'Samba group mapping added' : 'Samba settings updated')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    updateMutation.mutate()
  }

  // Show "Add ObjectClass" UI if group doesn't have sambaGroupMapping and not in add mode
  if (!hasObjectClass && !isAddingObjectClass) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Samba Group Mapping</CardTitle>
          <CardDescription>sambaGroupMapping attributes for Windows/Samba integration</CardDescription>
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
              This group does not have the <code className="text-xs bg-muted px-1 py-0.5 rounded">sambaGroupMapping</code> objectClass. Add it to enable Windows/Samba integration.
            </p>
            {canWrite ? (
              <Button
                onClick={handleAddSambaGroup}
                size="lg"
              >
                <Plus className="h-5 w-5 mr-2" />
                Add Samba Group Mapping
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
          <CardTitle>{isAddingObjectClass ? 'Add Samba Group Mapping' : 'Samba Group Mapping'}</CardTitle>
          <CardDescription>
            {isAddingObjectClass
              ? 'Configure Samba attributes to enable Windows/Samba integration for this group'
              : 'sambaGroupMapping attributes for Windows/Samba integration'}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {isAddingObjectClass && (
            <Callout variant="info">
              Fill in the Samba attributes below and click "Save Changes" to add the sambaGroupMapping objectClass to this group.
            </Callout>
          )}

          <div className="space-y-2">
            <Label htmlFor="sambaSID">SID</Label>
            <Input
              id="sambaSID"
              value={formData.sambaSID}
              onChange={(e) => setFormData({ ...formData, sambaSID: e.target.value })}
              disabled={!canWrite}
              placeholder="S-1-5-21-...-512"
            />
            <p className="text-xs text-muted-foreground">Samba Security Identifier for this group</p>
          </div>

          <FieldRow>
            <div className="space-y-2">
              <Label htmlFor="sambaGroupType">Group Type</Label>
              <Input
                id="sambaGroupType"
                value={formData.sambaGroupType}
                onChange={(e) => setFormData({ ...formData, sambaGroupType: e.target.value })}
                disabled={!canWrite}
                placeholder="2"
              />
              <p className="text-xs text-muted-foreground">2=Domain, 4=Local, 5=Builtin</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="displayName">Display Name</Label>
              <Input
                id="displayName"
                value={formData.displayName}
                onChange={(e) => setFormData({ ...formData, displayName: e.target.value })}
                disabled={!canWrite}
                placeholder="Domain Admins"
              />
              <p className="text-xs text-muted-foreground">Windows display name</p>
            </div>
          </FieldRow>

          {canWrite && (
            <div className="flex justify-end gap-2 pt-4">
              {/* In add mode the defaults are the change: nothing is stored yet to
                  compare them with, so Save stays available. */}
              <FormActions
                dirty={isDirty || isAddingObjectClass}
                pending={updateMutation.isPending}
                onDiscard={isAddingObjectClass ? undefined : () => setFormData(groupSambaForm(group))}
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

// Sudo Tab
function SudoTab({ group, dn, canWrite }: { group: NonNullable<ReturnType<typeof api.groups.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean }) {
  const queryClient = useQueryClient()
  const [addRoleOpen, setAddRoleOpen] = useState(false)
  const [selectedRole, setSelectedRole] = useState('')

  // Fetch group's sudo roles
  const { data: groupSudoRoles, isLoading: loadingGroupRoles } = useQuery({
    queryKey: ['group', dn, 'sudo-roles'],
    queryFn: ({ signal }) => api.groups.getSudoRoles(dn, signal),
  })

  // Candidate roles, matched by the directory. Headroom for the roles the group
  // already has, which are dropped below.
  const rolePicker = usePickerSearch(['sudo-roles', 'picker', dn], api.sudoRoles.list, {
    sort: 'cn',
    headroom: groupSudoRoles?.data.length ?? 0,
    enabled: addRoleOpen,
  })

  const availableRoles = useMemo(() => {
    const groupRoleDns = new Set(groupSudoRoles?.data.map(r => r.dn) ?? [])
    return rolePicker.items.filter(r => !groupRoleDns.has(r.dn))
  }, [rolePicker.items, groupSudoRoles?.data])

  const addToRoleMutation = useMutation({
    mutationFn: (roleDn: string) => api.sudoRoles.addGroup(roleDn, group.cn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['group', dn, 'sudo-roles'] })
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      setAddRoleOpen(false)
      setSelectedRole('')
      rolePicker.reset()
      toast.success('Group added to sudo role')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const removeFromRoleMutation = useMutation({
    mutationFn: (roleDn: string) => api.sudoRoles.removeGroup(roleDn, group.cn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['group', dn, 'sudo-roles'] })
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      toast.success('Group removed from sudo role')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const sudoRoles = groupSudoRoles?.data || []

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheck className="h-5 w-5" />
            Sudo Roles ({sudoRoles.length})
          </CardTitle>
          <CardDescription>sudoRole memberships for group sudo privileges (via %{group.cn})</CardDescription>
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
                  Select a sudo role to grant this group sudo privileges.
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
                          : 'This group is already in every sudo role.'}
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
        {(addToRoleMutation.error || removeFromRoleMutation.error) && (
          <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md mb-4">
            {(addToRoleMutation.error || removeFromRoleMutation.error)?.message}
          </div>
        )}

        {loadingGroupRoles ? (
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
                  <div className="flex items-center gap-2">
                    <ShieldCheck className="h-4 w-4 text-muted-foreground" />
                    <span className="font-medium">{role.cn}</span>
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
                {canWrite && (
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

// Security Tab
function SecurityTab({
  group,
  canDelete,
  deleteMutation,
}: {
  group: NonNullable<ReturnType<typeof api.groups.get> extends Promise<infer T> ? T : never>
  canDelete: boolean
  deleteMutation: ReturnType<typeof useMutation<unknown, Error, void>>
}) {
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)

  return (
    <div className="space-y-4">
      {canDelete && (
        <Card className="border-destructive/50">
          <CardHeader>
            <CardTitle className="text-destructive">Danger Zone</CardTitle>
            <CardDescription>Irreversible actions</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex items-center justify-between p-4 rounded-lg border border-destructive/30 bg-destructive/5">
              <div>
                <p className="font-medium">Delete this group</p>
                <p className="text-sm text-muted-foreground">
                  Permanently remove this group from the directory. This action cannot be undone.
                </p>
              </div>
              <ConfirmDialog
                open={deleteDialogOpen}
                onOpenChange={setDeleteDialogOpen}
                trigger={
                  <Button variant="destructive">
                    <Trash2 className="h-4 w-4 mr-1" />
                    Delete Group
                  </Button>
                }
                title="Delete Group"
                description={`Are you sure you want to delete group "${group.cn}"? Sudo rules granting to %${group.cn} and accounts using it as their primary group are left as they are; they will be listed once the group is gone. This action cannot be undone.`}
                confirmLabel="Delete Group"
                error={deleteMutation.error?.message}
                isPending={deleteMutation.isPending}
                onConfirm={() => deleteMutation.mutate()}
              />
            </div>
          </CardContent>
        </Card>
      )}

      {!canDelete && (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            You don't have permission to manage security settings for this group.
          </CardContent>
        </Card>
      )}
    </div>
  )
}
