import { createFileRoute, redirect, useRouter } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { decodeDN } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
import { ArrowLeft, Save, UserPlus, UserMinus, Users, Search, Info, Shield, Trash2, ShieldCheck, Plus, X } from 'lucide-react'
import { useState, useEffect, useMemo } from 'react'

export const Route = createFileRoute('/groups/$dn')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
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

  const [activeTab, setActiveTab] = useState('information')
  const [description, setDescription] = useState('')
  const [newMember, setNewMember] = useState('')
  const [addMemberOpen, setAddMemberOpen] = useState(false)
  const [memberSearch, setMemberSearch] = useState('')

  const { data: group, isLoading, error } = useQuery({
    queryKey: ['group', dn],
    queryFn: ({ signal }) => api.groups.get(dn, signal),
  })

  const { data: usersData } = useQuery({
    queryKey: ['users'],
    queryFn: ({ signal }) => api.users.list(signal),
  })

  const { data: config } = useQuery({
    queryKey: ['admin', 'config'],
    queryFn: ({ signal }) => api.admin.getConfig(signal),
  })

  // Get enabled LDAP object classes from config
  const enabledObjects = useMemo(() => {
    const objects = config?.app.groupsObjects.value
    if (Array.isArray(objects)) {
      return new Set(objects.map(m => m.toLowerCase()))
    }
    return new Set(['posixgroup'])
  }, [config])

  // Get enabled high-level modules from config
  const enabledModules = useMemo(() => {
    const modules = config?.app.modules.value
    if (Array.isArray(modules)) {
      return new Set(modules.map(m => m.toLowerCase()))
    }
    return new Set(['users', 'groups', 'sudo', 'policies'])
  }, [config])

  const showPosixGroupTabs = enabledObjects.has('posixgroup')
  const showSambaTab = enabledObjects.has('sambagroupmapping')
  const showSudoTab = enabledModules.has('sudo')

  // Check which objectClasses the group actually has
  const groupObjectClasses = useMemo(() => {
    const classes = new Set((group?.objectClasses || []).map(oc => oc.toLowerCase()))
    return classes
  }, [group?.objectClasses])

  const hasSambaGroupMapping = groupObjectClasses.has('sambagroupmapping')

  // Set default active tab to first available tab
  useEffect(() => {
    if (showPosixGroupTabs) {
      setActiveTab('information')
    } else if (showSambaTab) {
      setActiveTab('samba')
    } else if (showSudoTab) {
      setActiveTab('sudo')
    } else {
      setActiveTab('security')
    }
  }, [showPosixGroupTabs, showSambaTab, showSudoTab])

  useEffect(() => {
    if (group) {
      setDescription(group.description || '')
    }
  }, [group])

  const updateMutation = useMutation({
    mutationFn: () => api.groups.update(dn, { description }),
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
      setNewMember('')
      setMemberSearch('')
      setAddMemberOpen(false)
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      toast.success('Group deleted')
      router.navigate({ to: '/groups' })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    updateMutation.mutate()
  }

  const availableUsers = useMemo(() => {
    const filtered = usersData?.data.filter(
      (user) => !group?.memberUid?.includes(user.uid)
    ) ?? []

    // Filter by search term
    const searched = memberSearch
      ? filtered.filter((user) => {
          const searchLower = memberSearch.toLowerCase()
          return (
            user.uid.toLowerCase().includes(searchLower) ||
            (user.displayName || user.cn).toLowerCase().includes(searchLower)
          )
        })
      : filtered

    // Sort by display name
    return [...searched].sort((a, b) => {
      const aName = (a.displayName || a.cn).toLowerCase()
      const bName = (b.displayName || b.cn).toLowerCase()
      return aName.localeCompare(bName)
    })
  }, [usersData?.data, group?.memberUid, memberSearch])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-4 text-destructive bg-destructive/10 rounded-md">
        Failed to load group: {error.message}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => router.navigate({ to: '/groups' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-2xl font-bold">{group?.cn}</h1>
          <p className="text-sm text-muted-foreground">{group?.dn}</p>
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
              description={description}
              setDescription={setDescription}
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
              usersData={usersData}
              availableUsers={availableUsers}
              canWrite={canWrite}
              addMemberOpen={addMemberOpen}
              setAddMemberOpen={setAddMemberOpen}
              newMember={newMember}
              setNewMember={setNewMember}
              memberSearch={memberSearch}
              setMemberSearch={setMemberSearch}
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
  description,
  setDescription,
  canWrite,
  updateMutation,
  onSubmit,
}: {
  group: NonNullable<ReturnType<typeof api.groups.get> extends Promise<infer T> ? T : never>
  description: string
  setDescription: (value: string) => void
  canWrite: boolean
  updateMutation: ReturnType<typeof useMutation<unknown, Error, void>>
  onSubmit: (e: React.FormEvent) => void
}) {
  return (
    <form onSubmit={onSubmit}>
      <Card>
        <CardHeader>
          <CardTitle>Group Information</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {updateMutation.error && (
            <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
              {updateMutation.error.message}
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="cn">Name (CN)</Label>
            <Input id="cn" value={group?.cn || ''} disabled />
          </div>

          <div className="space-y-2">
            <Label htmlFor="gidNumber">GID Number</Label>
            <Input id="gidNumber" value={group?.gidNumber || ''} disabled />
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
            <div className="flex justify-end pt-4">
              <Button type="submit" disabled={updateMutation.isPending}>
                <Save className="h-4 w-4 mr-1" />
                {updateMutation.isPending ? 'Saving...' : 'Save Changes'}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </form>
  )
}

// Members Tab
function MembersTab({
  group,
  usersData,
  availableUsers,
  canWrite,
  addMemberOpen,
  setAddMemberOpen,
  newMember,
  setNewMember,
  memberSearch,
  setMemberSearch,
  addMemberMutation,
  removeMemberMutation,
}: {
  group: NonNullable<ReturnType<typeof api.groups.get> extends Promise<infer T> ? T : never>
  usersData: { data: Array<{ uid: string; cn: string; displayName?: string; jpegPhoto?: string }> } | undefined
  availableUsers: Array<{ uid: string; cn: string; displayName?: string; jpegPhoto?: string }>
  canWrite: boolean
  addMemberOpen: boolean
  setAddMemberOpen: (open: boolean) => void
  newMember: string
  setNewMember: (uid: string) => void
  memberSearch: string
  setMemberSearch: (search: string) => void
  addMemberMutation: ReturnType<typeof useMutation<unknown, Error, string>>
  removeMemberMutation: ReturnType<typeof useMutation<unknown, Error, string>>
}) {
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
                    value={memberSearch}
                    onChange={(e) => setMemberSearch(e.target.value)}
                    className="pl-8"
                  />
                </div>
                <div className="max-h-64 overflow-y-auto space-y-1">
                  {availableUsers.length > 0 ? (
                    availableUsers.map((user) => (
                      <button
                        key={user.uid}
                        type="button"
                        onClick={() => setNewMember(user.uid)}
                        className={`w-full flex items-center gap-3 p-2 rounded-md text-left transition-colors ${
                          newMember === user.uid
                            ? 'bg-primary text-primary-foreground'
                            : 'hover:bg-muted'
                        }`}
                      >
                        <Avatar
                          src={user.jpegPhoto}
                          fallback={user.displayName || user.cn}
                          size="sm"
                        />
                        <div className="flex-1 min-w-0">
                          <div className="font-medium truncate">
                            {user.displayName || user.cn}
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
                      No users available
                    </p>
                  )}
                </div>
              </div>
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => {
                  setAddMemberOpen(false)
                  setNewMember('')
                  setMemberSearch('')
                }}>
                  Cancel
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
            {[...group.memberUid].sort((a, b) => a.localeCompare(b)).map((uid) => {
              const user = usersData?.data.find((u) => u.uid === uid)
              return (
                <li
                  key={uid}
                  className="flex items-center justify-between p-2 rounded-md bg-muted"
                >
                  <div className="flex items-center gap-3">
                    <Avatar
                      src={user?.jpegPhoto}
                      fallback={user?.displayName || user?.cn || uid}
                      size="sm"
                    />
                    <div>
                      <span className="font-medium">{uid}</span>
                      {user && (
                        <span className="text-sm text-muted-foreground ml-2">
                          ({user.displayName || user.cn})
                        </span>
                      )}
                    </div>
                  </div>
                  {canWrite && (
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => removeMemberMutation.mutate(uid)}
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
    </Card>
  )
}

// Samba Tab (sambaGroupMapping)
function SambaTab({ group, dn, canWrite, hasObjectClass }: { group: NonNullable<ReturnType<typeof api.groups.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean, hasObjectClass: boolean }) {
  const queryClient = useQueryClient()
  const [isAddingObjectClass, setIsAddingObjectClass] = useState(false)

  const [formData, setFormData] = useState({
    sambaSID: '',
    sambaGroupType: '',
    displayName: '',
  })

  useEffect(() => {
    if (group) {
      setFormData({
        sambaSID: group.sambaSID || '',
        sambaGroupType: group.sambaGroupType || '',
        displayName: group.displayName || '',
      })
    }
  }, [group])

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
          {updateMutation.error && (
            <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
              {updateMutation.error.message}
            </div>
          )}

          {isAddingObjectClass && (
            <div className="p-3 text-sm text-blue-700 bg-blue-100 rounded-md">
              Fill in the Samba attributes below and click "Save Changes" to add the sambaGroupMapping objectClass to this group.
            </div>
          )}

          {updateMutation.isSuccess && !isAddingObjectClass && (
            <div className="p-3 text-sm text-green-700 bg-green-100 rounded-md">
              Samba attributes updated successfully
            </div>
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

          <div className="grid grid-cols-2 gap-4">
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
          </div>

          {canWrite && (
            <div className="flex justify-end gap-2 pt-4">
              {isAddingObjectClass && (
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setIsAddingObjectClass(false)}
                >
                  Cancel
                </Button>
              )}
              <Button type="submit" disabled={updateMutation.isPending}>
                <Save className="h-4 w-4 mr-1" />
                {updateMutation.isPending ? 'Saving...' : 'Save Changes'}
              </Button>
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
  const [roleSearch, setRoleSearch] = useState('')

  // Fetch group's sudo roles
  const { data: groupSudoRoles, isLoading: loadingGroupRoles } = useQuery({
    queryKey: ['group', dn, 'sudo-roles'],
    queryFn: ({ signal }) => api.groups.getSudoRoles(dn, signal),
  })

  // Fetch all sudo roles
  const { data: allSudoRoles } = useQuery({
    queryKey: ['sudo-roles'],
    queryFn: ({ signal }) => api.sudoRoles.list(signal),
  })

  // Sudo roles the group is NOT a member of
  const availableRoles = useMemo(() => {
    const groupRoleDns = new Set(groupSudoRoles?.data.map(r => r.dn) ?? [])
    const filtered = allSudoRoles?.data.filter(r => !groupRoleDns.has(r.dn)) ?? []

    // Filter by search
    const searched = roleSearch
      ? filtered.filter(r => r.cn.toLowerCase().includes(roleSearch.toLowerCase()))
      : filtered

    // Sort by name
    return [...searched].sort((a, b) => a.cn.localeCompare(b.cn))
  }, [allSudoRoles?.data, groupSudoRoles?.data, roleSearch])

  const addToRoleMutation = useMutation({
    mutationFn: (roleDn: string) => api.sudoRoles.addGroup(roleDn, group.cn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['group', dn, 'sudo-roles'] })
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      setAddRoleOpen(false)
      setSelectedRole('')
      setRoleSearch('')
    },
  })

  const removeFromRoleMutation = useMutation({
    mutationFn: (roleDn: string) => api.sudoRoles.removeGroup(roleDn, group.cn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['group', dn, 'sudo-roles'] })
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
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
                    value={roleSearch}
                    onChange={(e) => setRoleSearch(e.target.value)}
                  />
                </div>
                <div className="max-h-64 overflow-y-auto space-y-1">
                  {availableRoles.length > 0 ? (
                    availableRoles.map((role) => (
                      <button
                        key={role.dn}
                        type="button"
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
                      No sudo roles available
                    </p>
                  )}
                </div>
              </div>
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => {
                  setAddRoleOpen(false)
                  setSelectedRole('')
                  setRoleSearch('')
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
              <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
                <DialogTrigger asChild>
                  <Button variant="destructive">
                    <Trash2 className="h-4 w-4 mr-1" />
                    Delete Group
                  </Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Delete Group</DialogTitle>
                    <DialogDescription>
                      Are you sure you want to delete group "{group.cn}"?
                      This action cannot be undone.
                    </DialogDescription>
                  </DialogHeader>
                  {deleteMutation.error && (
                    <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
                      {deleteMutation.error.message}
                    </div>
                  )}
                  <div className="flex justify-end gap-2">
                    <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
                      Cancel
                    </Button>
                    <Button
                      variant="destructive"
                      onClick={() => deleteMutation.mutate()}
                      disabled={deleteMutation.isPending}
                    >
                      {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
                    </Button>
                  </div>
                </DialogContent>
              </Dialog>
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
