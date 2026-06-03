import { createFileRoute, redirect, useRouter, Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { decodeDN, encodeDN, formatLdapTimestamp, isLdapTimestampInFuture, ldapTimestampToDateString } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Avatar } from '@/components/ui/avatar'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Select } from '@/components/ui/select'
import { ArrowLeft, Save, Camera, Trash2, Lock, Unlock, Plus, X, Key, Users, Shield, User, Terminal, UserPlus, UserMinus, ShieldCheck, Mail, AlertTriangle, CalendarClock } from 'lucide-react'
import { DatePicker } from '@/components/ui/date-picker'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { useState, useRef, useMemo } from 'react'

export const Route = createFileRoute('/users/$dn')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  component: UserDetailPage,
})

function UserDetailPage() {
  const { dn: encodedDN } = Route.useParams()
  const dn = decodeDN(encodedDN)
  const router = useRouter()
  const { hasPermission } = useAuth()
  const canWrite = hasPermission('users:write')
  const canDelete = hasPermission('users:delete')

  const { data: user, isLoading, error } = useQuery({
    queryKey: ['user', dn],
    queryFn: ({ signal }) => api.users.get(dn, signal),
  })

  const { data: userGroups } = useQuery({
    queryKey: ['user', dn, 'groups'],
    queryFn: ({ signal }) => api.users.getGroups(dn, signal),
  })

  const { data: config } = useQuery({
    queryKey: ['admin', 'config'],
    queryFn: ({ signal }) => api.admin.getConfig(signal),
  })

  // Get enabled LDAP object classes from config
  const enabledObjects = useMemo(() => {
    const objects = config?.app.usersObjects.value
    if (Array.isArray(objects)) {
      return new Set(objects.map(m => m.toLowerCase()))
    }
    return new Set(['inetorgperson', 'posixaccount', 'ldappublickey'])
  }, [config])

  // Get enabled high-level modules from config
  const enabledModules = useMemo(() => {
    const modules = config?.app.modules.value
    if (Array.isArray(modules)) {
      return new Set(modules.map(m => m.toLowerCase()))
    }
    return new Set(['users', 'groups', 'sudo', 'policies'])
  }, [config])

  const showIdentityTab = enabledObjects.has('inetorgperson')
  const showPosixTab = enabledObjects.has('posixaccount')
  const showSSHKeysTab = enabledObjects.has('ldappublickey')
  const showSambaTab = enabledObjects.has('sambasamaccount')
  const showShadowTab = enabledObjects.has('shadowaccount')
  const showSudoTab = enabledModules.has('sudo')
  const showPoliciesModule = enabledModules.has('policies')

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

  const [userSelectedTab, setUserSelectedTab] = useState<string | null>(null)
  const activeTab = userSelectedTab ?? defaultTab

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
        Failed to load user: {error.message}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" aria-label="Back to users" onClick={() => router.navigate({ to: '/users' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <Avatar
          src={user?.jpegPhoto}
          fallback={user?.displayName || user?.cn}
          size="lg"
        />
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold">{user?.displayName || user?.cn}</h1>
            {user?.accountLocked && (
              <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-destructive/10 text-destructive">
                <Lock className="h-3 w-3 mr-1" />
                Locked
              </span>
            )}
          </div>
          <p className="text-sm text-muted-foreground">{user?.dn}</p>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setUserSelectedTab}>
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
            <ShadowTab user={user!} dn={dn} canWrite={canWrite} hasObjectClass={hasShadowAccount} />
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
          <SecurityTab user={user!} dn={dn} canWrite={canWrite} canDelete={canDelete} showPoliciesModule={showPoliciesModule} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// Identity Tab (inetOrgPerson)
function IdentityTab({ user, dn, canWrite, groups }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean, groups: Array<{ dn: string; cn: string; gidNumber: number }> }) {
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [addGroupOpen, setAddGroupOpen] = useState(false)
  const [selectedGroups, setSelectedGroups] = useState<string[]>([])
  const [groupSearch, setGroupSearch] = useState('')
  const [managerDropdownOpen, setManagerDropdownOpen] = useState(false)
  const [managerSearch, setManagerSearch] = useState('')

  // Fetch all groups to find available ones
  const { data: allGroups } = useQuery({
    queryKey: ['groups'],
    queryFn: ({ signal }) => api.groups.list(signal),
  })

  // Fetch all users for manager dropdown
  const { data: allUsers } = useQuery({
    queryKey: ['users'],
    queryFn: ({ signal }) => api.users.list(signal),
  })

  // Filter users for manager dropdown (exclude current user)
  const availableManagers = useMemo(() => {
    const filtered = allUsers?.data.filter(u => u.dn !== dn) ?? []

    // Filter by search
    const searched = managerSearch
      ? filtered.filter(u => {
          const searchLower = managerSearch.toLowerCase()
          return (
            u.uid.toLowerCase().includes(searchLower) ||
            (u.displayName || u.cn).toLowerCase().includes(searchLower)
          )
        })
      : filtered

    // Sort by display name
    return [...searched].sort((a, b) => {
      const aName = (a.displayName || a.cn).toLowerCase()
      const bName = (b.displayName || b.cn).toLowerCase()
      return aName.localeCompare(bName)
    })
  }, [allUsers?.data, dn, managerSearch])

  // Groups the user is NOT a member of
  const availableGroups = useMemo(() => {
    const memberGroupDns = new Set(groups.map(g => g.dn))
    const filtered = allGroups?.data.filter(g => !memberGroupDns.has(g.dn)) ?? []

    // Filter by search
    const searched = groupSearch
      ? filtered.filter(g => g.cn.toLowerCase().includes(groupSearch.toLowerCase()))
      : filtered

    // Sort by name
    return [...searched].sort((a, b) => a.cn.localeCompare(b.cn))
  }, [allGroups?.data, groups, groupSearch])

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
      setGroupSearch('')
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

  const [formData, setFormData] = useState({
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
    jpegPhoto: user.jpegPhoto || '',
  })
  const [photoChanged, setPhotoChanged] = useState(false)

  const handlePhotoUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

    const reader = new FileReader()
    reader.onload = () => {
      const base64 = (reader.result as string).split(',')[1]
      setFormData({ ...formData, jpegPhoto: base64 })
      setPhotoChanged(true)
    }
    reader.readAsDataURL(file)
  }

  const handlePhotoRemove = () => {
    setFormData({ ...formData, jpegPhoto: '' })
    setPhotoChanged(true)
  }

  const updateMutation = useMutation({
    mutationFn: () => {
      const updateData: Record<string, string | undefined> = {}
      if (formData.givenName !== user?.givenName) updateData.givenName = formData.givenName
      if (formData.sn !== user?.sn) updateData.sn = formData.sn
      if (formData.cn !== user?.cn) updateData.cn = formData.cn
      if (formData.displayName !== user?.displayName) updateData.displayName = formData.displayName
      if (formData.mail !== user?.mail) updateData.mail = formData.mail
      if (formData.telephoneNumber !== user?.telephoneNumber) updateData.telephoneNumber = formData.telephoneNumber
      if (formData.title !== user?.title) updateData.title = formData.title
      if (formData.departmentNumber !== user?.departmentNumber) updateData.departmentNumber = formData.departmentNumber
      if (formData.o !== (user?.o || '')) updateData.o = formData.o
      if (formData.employeeNumber !== (user?.employeeNumber || '')) updateData.employeeNumber = formData.employeeNumber
      if (formData.employeeType !== (user?.employeeType || '')) updateData.employeeType = formData.employeeType
      if (formData.initials !== (user?.initials || '')) updateData.initials = formData.initials
      if (formData.manager !== (user?.manager || '')) updateData.manager = formData.manager
      if (formData.description !== user?.description) updateData.description = formData.description
      if (photoChanged) updateData.jpegPhoto = formData.jpegPhoto
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
      <form onSubmit={handleSubmit}>
        <Card>
          <CardHeader>
            <CardTitle>Personal Information</CardTitle>
            <CardDescription>inetOrgPerson attributes</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {updateMutation.error && (
              <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
                {updateMutation.error.message}
              </div>
            )}

            {canWrite && (
              <div className="space-y-2">
                <Label>Photo</Label>
                <div className="flex items-center gap-4">
                  <Avatar
                    src={formData.jpegPhoto}
                    fallback={user?.displayName || user?.cn}
                    size="lg"
                  />
                  <div className="flex gap-2">
                    <input
                      ref={fileInputRef}
                      type="file"
                      accept="image/jpeg,image/png,image/gif"
                      onChange={handlePhotoUpload}
                      className="hidden"
                    />
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => fileInputRef.current?.click()}
                    >
                      <Camera className="h-4 w-4 mr-1" />
                      Upload
                    </Button>
                    {formData.jpegPhoto && (
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

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="uid">Username (UID)</Label>
                <Input id="uid" value={user?.uid || ''} disabled />
                <p className="text-xs text-muted-foreground">The username cannot be changed after creation.</p>
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
            </div>

            <div className="grid grid-cols-2 gap-4">
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
            </div>

            <div className="space-y-2">
              <Label htmlFor="displayName">Display Name</Label>
              <Input
                id="displayName"
                value={formData.displayName}
                onChange={(e) => setFormData({ ...formData, displayName: e.target.value })}
                disabled={!canWrite}
              />
            </div>

            <div className="grid grid-cols-2 gap-4">
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
            </div>

            <div className="grid grid-cols-2 gap-4">
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
            </div>

            <div className="space-y-2">
              <Label htmlFor="o">Organization</Label>
              <Input
                id="o"
                value={formData.o}
                onChange={(e) => setFormData({ ...formData, o: e.target.value })}
                disabled={!canWrite}
              />
            </div>

            <div className="grid grid-cols-2 gap-4">
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
            </div>

            <div className="grid grid-cols-2 gap-4">
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
                          const mgr = allUsers?.data.find(u => u.dn === formData.manager)
                          return mgr ? `${mgr.displayName || mgr.cn} (${mgr.uid})` : formData.manager
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
                        value={managerSearch}
                        onChange={(e) => setManagerSearch(e.target.value)}
                      />
                      <div className="max-h-64 overflow-y-auto space-y-1">
                        {formData.manager && (
                          <button
                            type="button"
                            onClick={() => {
                              setFormData({ ...formData, manager: '' })
                              setManagerDropdownOpen(false)
                              setManagerSearch('')
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
                              onClick={() => {
                                setFormData({ ...formData, manager: mgr.dn })
                                setManagerDropdownOpen(false)
                                setManagerSearch('')
                              }}
                              className={`w-full flex items-center gap-3 p-2 rounded-md text-left transition-colors ${
                                formData.manager === mgr.dn
                                  ? 'bg-primary text-primary-foreground'
                                  : 'hover:bg-muted'
                              }`}
                            >
                              <Avatar
                                src={mgr.jpegPhoto}
                                fallback={mgr.displayName || mgr.cn}
                                size="sm"
                              />
                              <div className="flex-1 min-w-0">
                                <div className="font-medium truncate">
                                  {mgr.displayName || mgr.cn}
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
                            No users found
                          </p>
                        )}
                      </div>
                    </div>
                  </DialogContent>
                </Dialog>
              </div>
            </div>

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
                      value={groupSearch}
                      onChange={(e) => setGroupSearch(e.target.value)}
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
                        No groups available
                      </p>
                    )}
                  </div>
                </div>
                <div className="flex justify-end gap-2">
                  <Button variant="outline" onClick={() => {
                    setAddGroupOpen(false)
                    setSelectedGroups([])
                    setGroupSearch('')
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
                      onClick={() => removeFromGroupMutation.mutate(group.dn)}
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
      </Card>
    </div>
  )
}

// POSIX Tab (posixAccount)
function PosixTab({ user, dn, canWrite }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean }) {
  const queryClient = useQueryClient()

  const [formData, setFormData] = useState({
    homeDirectory: user.homeDirectory || '',
    loginShell: user.loginShell || '',
    gecos: user.gecos || '',
  })

  const updateMutation = useMutation({
    mutationFn: () => {
      const updateData: Record<string, string | undefined> = {}
      if (formData.homeDirectory !== (user?.homeDirectory || '')) updateData.homeDirectory = formData.homeDirectory
      if (formData.loginShell !== (user?.loginShell || '')) updateData.loginShell = formData.loginShell
      if (formData.gecos !== (user?.gecos || '')) updateData.gecos = formData.gecos
      return api.users.update(dn, updateData)
    },
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
      <Card>
        <CardHeader>
          <CardTitle>POSIX Account</CardTitle>
          <CardDescription>posixAccount attributes for Unix/Linux systems</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {updateMutation.error && (
            <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
              {updateMutation.error.message}
            </div>
          )}

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="uidNumber">UID Number</Label>
              <Input
                id="uidNumber"
                value={user?.uidNumber ?? ''}
                disabled
                className={!user?.uidNumber ? 'text-muted-foreground italic' : ''}
                placeholder="Not set"
              />
              <p className="text-xs text-muted-foreground">Unique numeric user ID (read-only)</p>
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
              <p className="text-xs text-muted-foreground">Primary group ID (read-only)</p>
            </div>
          </div>

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
        {(addKeyMutation.error || removeKeyMutation.error) && (
          <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
            {(addKeyMutation.error || removeKeyMutation.error)?.message}
          </div>
        )}

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

  const [formData, setFormData] = useState({
    sambaSID: user.sambaSID || '',
    sambaPrimaryGroupSID: user.sambaPrimaryGroupSID || '',
    sambaAcctFlags: user.sambaAcctFlags || '',
    sambaHomePath: user.sambaHomePath || '',
    sambaHomeDrive: user.sambaHomeDrive || '',
    sambaLogonScript: user.sambaLogonScript || '',
    sambaProfilePath: user.sambaProfilePath || '',
    sambaDomainName: user.sambaDomainName || '',
  })

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
    mutationFn: () => {
      const updateData: Record<string, string | undefined> = {}
      if (formData.sambaSID !== (user?.sambaSID || '')) updateData.sambaSID = formData.sambaSID
      if (formData.sambaPrimaryGroupSID !== (user?.sambaPrimaryGroupSID || '')) updateData.sambaPrimaryGroupSID = formData.sambaPrimaryGroupSID
      if (formData.sambaAcctFlags !== (user?.sambaAcctFlags || '')) updateData.sambaAcctFlags = formData.sambaAcctFlags
      if (formData.sambaHomePath !== (user?.sambaHomePath || '')) updateData.sambaHomePath = formData.sambaHomePath
      if (formData.sambaHomeDrive !== (user?.sambaHomeDrive || '')) updateData.sambaHomeDrive = formData.sambaHomeDrive
      if (formData.sambaLogonScript !== (user?.sambaLogonScript || '')) updateData.sambaLogonScript = formData.sambaLogonScript
      if (formData.sambaProfilePath !== (user?.sambaProfilePath || '')) updateData.sambaProfilePath = formData.sambaProfilePath
      if (formData.sambaDomainName !== (user?.sambaDomainName || '')) updateData.sambaDomainName = formData.sambaDomainName
      return api.users.updateSamba(dn, updateData)
    },
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
          {updateMutation.error && (
            <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
              {updateMutation.error.message}
            </div>
          )}

          {isAddingObjectClass && (
            <div className="p-3 text-sm text-blue-700 bg-blue-100 rounded-md">
              Fill in the Samba attributes below and click "Save Changes" to add the sambaSamAccount objectClass to this user.
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
              placeholder="S-1-5-21-..."
            />
            <p className="text-xs text-muted-foreground">Samba Security Identifier (required for Samba)</p>
          </div>

          <div className="grid grid-cols-2 gap-4">
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
          </div>

          <div className="grid grid-cols-2 gap-4">
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
          </div>

          <div className="grid grid-cols-2 gap-4">
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
          </div>

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

// Shadow Tab (shadowAccount)
function ShadowTab({ user, dn, canWrite, hasObjectClass }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean, hasObjectClass: boolean }) {
  const queryClient = useQueryClient()
  const [isAddingObjectClass, setIsAddingObjectClass] = useState(false)

  const [formData, setFormData] = useState({
    shadowLastChange: user.shadowLastChange?.toString() || '',
    shadowMin: user.shadowMin?.toString() || '',
    shadowMax: user.shadowMax?.toString() || '',
    shadowWarning: user.shadowWarning?.toString() || '',
    shadowInactive: user.shadowInactive?.toString() || '',
    shadowExpire: user.shadowExpire?.toString() || '',
    shadowFlag: user.shadowFlag?.toString() || '',
  })

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
      const updateData: Record<string, number | undefined> = {}
      const parseIntOrUndefined = (val: string) => val !== '' ? parseInt(val, 10) : undefined

      if (formData.shadowLastChange !== (user?.shadowLastChange?.toString() || ''))
        updateData.shadowLastChange = parseIntOrUndefined(formData.shadowLastChange)
      if (formData.shadowMin !== (user?.shadowMin?.toString() || ''))
        updateData.shadowMin = parseIntOrUndefined(formData.shadowMin)
      if (formData.shadowMax !== (user?.shadowMax?.toString() || ''))
        updateData.shadowMax = parseIntOrUndefined(formData.shadowMax)
      if (formData.shadowWarning !== (user?.shadowWarning?.toString() || ''))
        updateData.shadowWarning = parseIntOrUndefined(formData.shadowWarning)
      if (formData.shadowInactive !== (user?.shadowInactive?.toString() || ''))
        updateData.shadowInactive = parseIntOrUndefined(formData.shadowInactive)
      if (formData.shadowExpire !== (user?.shadowExpire?.toString() || ''))
        updateData.shadowExpire = parseIntOrUndefined(formData.shadowExpire)
      if (formData.shadowFlag !== (user?.shadowFlag?.toString() || ''))
        updateData.shadowFlag = parseIntOrUndefined(formData.shadowFlag)
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
          {updateMutation.error && (
            <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
              {updateMutation.error.message}
            </div>
          )}

          {isAddingObjectClass && (
            <div className="p-3 text-sm text-blue-700 bg-blue-100 rounded-md">
              Fill in the shadow attributes below and click "Save Changes" to add the shadowAccount objectClass to this user.
            </div>
          )}

          {updateMutation.isSuccess && !isAddingObjectClass && (
            <div className="p-3 text-sm text-green-700 bg-green-100 rounded-md">
              Shadow attributes updated successfully
            </div>
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

          <div className="grid grid-cols-2 gap-4">
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
          </div>

          <div className="grid grid-cols-2 gap-4">
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
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="shadowExpire">Account Expiration</Label>
              <Input
                id="shadowExpire"
                type="number"
                value={formData.shadowExpire}
                onChange={(e) => setFormData({ ...formData, shadowExpire: e.target.value })}
                disabled={!canWrite}
                placeholder="Not set"
              />
              <p className="text-xs text-muted-foreground">
                Days since epoch when account expires
                {formData.shadowExpire && ` (${daysToDate(parseInt(formData.shadowExpire))})`}
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

// Sudo Tab (sudoRole)
function SudoTab({ user, dn, canWrite }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean }) {
  const queryClient = useQueryClient()
  const [addRoleOpen, setAddRoleOpen] = useState(false)
  const [selectedRole, setSelectedRole] = useState('')
  const [roleSearch, setRoleSearch] = useState('')

  // Fetch user's sudo roles
  const { data: userSudoRoles, isLoading: loadingUserRoles } = useQuery({
    queryKey: ['user', dn, 'sudo-roles'],
    queryFn: ({ signal }) => api.users.getSudoRoles(dn, signal),
  })

  // Fetch all sudo roles
  const { data: allSudoRoles } = useQuery({
    queryKey: ['sudo-roles'],
    queryFn: ({ signal }) => api.sudoRoles.list(signal),
  })

  // Sudo roles the user is NOT a member of
  const availableRoles = useMemo(() => {
    const userRoleDns = new Set(userSudoRoles?.data.map(r => r.dn) ?? [])
    const filtered = allSudoRoles?.data.filter(r => !userRoleDns.has(r.dn)) ?? []

    // Filter by search
    const searched = roleSearch
      ? filtered.filter(r => r.cn.toLowerCase().includes(roleSearch.toLowerCase()))
      : filtered

    // Sort by name
    return [...searched].sort((a, b) => a.cn.localeCompare(b.cn))
  }, [allSudoRoles?.data, userSudoRoles?.data, roleSearch])

  const addToRoleMutation = useMutation({
    mutationFn: (roleDn: string) => api.sudoRoles.addUser(roleDn, user.uid),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn, 'sudo-roles'] })
      queryClient.invalidateQueries({ queryKey: ['sudo-roles'] })
      setAddRoleOpen(false)
      setSelectedRole('')
      setRoleSearch('')
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

// Helper function to extract CN from a DN
function extractCN(dn: string): string {
  const match = dn.match(/^cn=([^,]+)/i)
  return match ? match[1] : dn
}

// Security Tab (password, lock/unlock, delete)
function SecurityTab({ user, dn, canWrite, canDelete, showPoliciesModule }: { user: NonNullable<ReturnType<typeof api.users.get> extends Promise<infer T> ? T : never>, dn: string, canWrite: boolean, canDelete: boolean, showPoliciesModule: boolean }) {
  const queryClient = useQueryClient()
  const router = useRouter()
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [removePasswordDialogOpen, setRemovePasswordDialogOpen] = useState(false)
  const [editingExpiration, setEditingExpiration] = useState(false)

  // Determine if the account has a future expiration date
  const hasFutureExpiration = user?.pwdAccountLockedTime && isLdapTimestampInFuture(user.pwdAccountLockedTime)
  const hasPastLockTime = user?.pwdAccountLockedTime && !isLdapTimestampInFuture(user.pwdAccountLockedTime)

  const [expirationDate, setExpirationDate] = useState(
    user?.pwdAccountLockedTime && hasFutureExpiration
      ? ldapTimestampToDateString(user.pwdAccountLockedTime)
      : ''
  )

  const { data: passwordPolicies } = useQuery({
    queryKey: ['password-policies'],
    queryFn: ({ signal }) => api.passwordPolicies.list(signal),
    enabled: showPoliciesModule,
  })

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

  const lockMutation = useMutation({
    mutationFn: () => api.users.lock(dn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      queryClient.invalidateQueries({ queryKey: ['users'] })
      toast.success('Account locked')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const unlockMutation = useMutation({
    mutationFn: () => api.users.unlock(dn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      queryClient.invalidateQueries({ queryKey: ['users'] })
      toast.success('Account unlocked')
    },
    onError: (error: Error) => {
      toast.error(error.message)
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      toast.success('User deleted')
      router.navigate({ to: '/users' })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
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

  // Password complexity check
  const passwordStrength = useMemo(() => {
    if (!password) return { score: 0, label: '', color: '' }

    let score = 0
    if (password.length >= 8) score++
    if (password.length >= 12) score++
    if (/[a-z]/.test(password)) score++
    if (/[A-Z]/.test(password)) score++
    if (/[0-9]/.test(password)) score++
    if (/[^a-zA-Z0-9]/.test(password)) score++

    if (score <= 2) return { score, label: 'Weak', color: 'bg-destructive' }
    if (score <= 4) return { score, label: 'Medium', color: 'bg-yellow-500' }
    return { score, label: 'Strong', color: 'bg-green-500' }
  }, [password])

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
            {changePasswordMutation.error && (
              <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
                {changePasswordMutation.error.message}
              </div>
            )}

            {changePasswordMutation.isSuccess && (
              <div className="p-3 text-sm text-green-700 bg-green-100 rounded-md">
                Password changed successfully
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="password">New Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={!canWrite}
              />
              {password && (
                <div className="space-y-1">
                  <div className="flex gap-1">
                    {[1, 2, 3, 4, 5, 6].map((i) => (
                      <div
                        key={i}
                        className={`h-1 flex-1 rounded ${
                          i <= passwordStrength.score ? passwordStrength.color : 'bg-muted'
                        }`}
                      />
                    ))}
                  </div>
                  <p className={`text-xs ${
                    passwordStrength.score <= 2 ? 'text-destructive' :
                    passwordStrength.score <= 4 ? 'text-yellow-600' : 'text-green-600'
                  }`}>
                    Password strength: {passwordStrength.label}
                  </p>
                </div>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="confirmPassword">Confirm Password</Label>
              <Input
                id="confirmPassword"
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                disabled={!canWrite}
              />
              {passwordsDontMatch && (
                <p className="text-xs text-destructive">Passwords do not match</p>
              )}
              {passwordsMatch && (
                <p className="text-xs text-green-600">Passwords match</p>
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
                {user.hasPassword ? (
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
                      {removePasswordMutation.error && (
                        <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
                          {removePasswordMutation.error.message}
                        </div>
                      )}
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
          {sendPasswordResetMutation.error && (
            <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
              {sendPasswordResetMutation.error.message}
            </div>
          )}

          {sendPasswordResetMutation.isSuccess && (
            <div className="p-3 text-sm text-green-700 bg-green-100 rounded-md">
              Password reset email sent successfully
            </div>
          )}

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
          {(lockMutation.error || unlockMutation.error || setExpirationMutation.error) && (
            <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
              {(lockMutation.error || unlockMutation.error || setExpirationMutation.error)?.message}
            </div>
          )}

          <div className="flex items-center gap-4 p-4 rounded-lg border">
            <div className={`p-3 rounded-full ${user?.accountLocked ? 'bg-destructive/10' : hasFutureExpiration ? 'bg-yellow-100' : 'bg-green-100'}`}>
              {user?.accountLocked ? (
                <Lock className="h-6 w-6 text-destructive" />
              ) : hasFutureExpiration ? (
                <CalendarClock className="h-6 w-6 text-yellow-600" />
              ) : (
                <Unlock className="h-6 w-6 text-green-600" />
              )}
            </div>
            <div className="flex-1">
              <p className="font-medium">
                {user?.accountLocked ? 'Account Locked' : hasFutureExpiration ? 'Account Active (Expires Soon)' : 'Account Active'}
              </p>
              <p className="text-sm text-muted-foreground">
                {user?.accountLocked
                  ? 'User cannot log in until the account is unlocked'
                  : hasFutureExpiration
                    ? 'Account will be locked on the expiration date'
                    : 'User can log in normally'}
              </p>
              {user?.accountLocked && hasPastLockTime && (
                <p className="text-xs text-muted-foreground mt-1">
                  Locked on: {formatLdapTimestamp(user.pwdAccountLockedTime!)}
                </p>
              )}
              {hasFutureExpiration && (
                <p className="text-xs text-yellow-600 mt-1">
                  Expires on: {formatLdapTimestamp(user.pwdAccountLockedTime!)}
                </p>
              )}
            </div>
          </div>

          {canWrite && (
            <div className="flex gap-2 flex-wrap">
              {user?.accountLocked ? (
                <Button
                  variant="default"
                  onClick={() => unlockMutation.mutate()}
                  disabled={unlockMutation.isPending}
                >
                  <Unlock className="h-4 w-4 mr-1" />
                  {unlockMutation.isPending ? 'Unlocking...' : 'Unlock Account'}
                </Button>
              ) : (
                <Button
                  variant="destructive"
                  onClick={() => lockMutation.mutate()}
                  disabled={lockMutation.isPending}
                >
                  <Lock className="h-4 w-4 mr-1" />
                  {lockMutation.isPending ? 'Locking...' : 'Lock Account'}
                </Button>
              )}
            </div>
          )}

          {/* Expiration Date Management - only show when policies module is enabled */}
          {showPoliciesModule && canWrite && !user?.accountLocked && (
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
                        if (user?.pwdAccountLockedTime && hasFutureExpiration) {
                          setExpirationDate(ldapTimestampToDateString(user.pwdAccountLockedTime))
                        }
                      }}
                    >
                      Cancel
                    </Button>
                  )}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">
                  This account is scheduled to expire on {formatLdapTimestamp(user.pwdAccountLockedTime!)}.
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
              <div className="flex items-center gap-3 p-3 rounded-lg bg-yellow-500/10 border border-yellow-500/30">
                <CalendarClock className="h-5 w-5 text-yellow-600 shrink-0" />
                <div>
                  <p className="font-medium text-yellow-700">Scheduled Expiration</p>
                  <p className="text-sm text-muted-foreground">
                    Account expires on: {formatLdapTimestamp(user.pwdAccountLockedTime!)}
                  </p>
                </div>
              </div>
            )}

            {hasPastLockTime && (
              <div className="flex items-center gap-3 p-3 rounded-lg bg-destructive/10 border border-destructive/30">
                <Lock className="h-5 w-5 text-destructive shrink-0" />
                <div>
                  <p className="font-medium text-destructive">Policy Locked</p>
                  <p className="text-sm text-muted-foreground">
                    Locked at: {formatLdapTimestamp(user.pwdAccountLockedTime!)}
                  </p>
                </div>
              </div>
            )}

            {user?.pwdReset && (
              <div className="flex items-center gap-3 p-3 rounded-lg bg-yellow-500/10 border border-yellow-500/30">
                <AlertTriangle className="h-5 w-5 text-yellow-600 shrink-0" />
                <div>
                  <p className="font-medium text-yellow-700">Password Reset Required</p>
                  <p className="text-sm text-muted-foreground">
                    User must change password on next login
                  </p>
                </div>
              </div>
            )}

            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <p className="text-muted-foreground">Password Changed</p>
                <p className="font-medium">
                  {user?.pwdChangedTime ? formatLdapTimestamp(user.pwdChangedTime) : 'N/A'}
                </p>
              </div>
              <div>
                <Label className="text-muted-foreground">Applied Policy</Label>
                {canWrite ? (
                  <Select
                    value={user?.pwdPolicySubentry || ''}
                    onChange={(e) => {
                      const newPolicy = e.target.value
                      updatePolicyMutation.mutate(newPolicy || undefined)
                    }}
                    options={[
                      { value: '', label: 'Default (none)' },
                      ...(passwordPolicies?.data || []).map((policy) => ({
                        value: policy.dn,
                        label: policy.cn + (policy.description ? ` - ${policy.description}` : ''),
                      })),
                    ]}
                    className="mt-1"
                    disabled={updatePolicyMutation.isPending}
                  />
                ) : (
                  <p className="font-medium font-mono text-xs mt-1">
                    {user?.pwdPolicySubentry ? extractCN(user.pwdPolicySubentry) : 'Default'}
                  </p>
                )}
              </div>
            </div>

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

      {canDelete && (
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
                description={`Are you sure you want to delete user "${user.displayName || user.uid}"? This action cannot be undone.`}
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
