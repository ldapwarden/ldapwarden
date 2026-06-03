import { createFileRoute, redirect, useRouter } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { DatePicker } from '@/components/ui/date-picker'
import { Select } from '@/components/ui/select'
import { ArrowLeft, Save, Wand2, Users, X, CalendarClock } from 'lucide-react'
import { useState, useMemo } from 'react'

export const Route = createFileRoute('/users/new')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
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

  const { data: allGroups } = useQuery({
    queryKey: ['groups'],
    queryFn: ({ signal }) => api.groups.list(signal),
  })

  const { data: config } = useQuery({
    queryKey: ['admin', 'config'],
    queryFn: ({ signal }) => api.admin.getConfig(signal),
  })

  // Check if policies module is enabled
  const enabledModules = useMemo(() => {
    const modules = config?.app?.modules?.value
    if (Array.isArray(modules)) {
      return new Set(modules)
    }
    return new Set<string>()
  }, [config])

  const showPoliciesModule = enabledModules.has('policies')

  const [primaryGroup, setPrimaryGroup] = useState('')
  const [selectedGroups, setSelectedGroups] = useState<string[]>([])
  const [groupSearch, setGroupSearch] = useState('')

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

  // Resolve gidNumber from primary group selection or next available
  const resolvedGidNumber = useMemo(() => {
    if (primaryGroup) {
      const group = allGroups?.data?.find(g => g.cn === primaryGroup)
      if (group) return group.gidNumber
    }
    return nextIds?.nextGid ?? 0
  }, [primaryGroup, allGroups?.data, nextIds])

  // Filter and sort available groups
  const availableGroups = useMemo(() => {
    const groups = allGroups?.data ?? []
    const filtered = groupSearch
      ? groups.filter(g => g.cn.toLowerCase().includes(groupSearch.toLowerCase()))
      : groups
    return [...filtered].sort((a, b) => a.cn.localeCompare(b.cn))
  }, [allGroups?.data, groupSearch])

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
      uidNumber: parseInt(formData.uidNumber || nextIds?.nextUid?.toString() || '0'),
      gidNumber: resolvedGidNumber,
      homeDirectory: formData.homeDirectory || `/home/${formData.uid}`,
      loginShell: formData.loginShell,
      description: formData.description || undefined,
      groups: selectedGroups.length > 0 ? selectedGroups : undefined,
      createPrimaryGroup: !primaryGroup || undefined,
      expirationDate: formData.expirationDate || undefined,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      toast.success('User created successfully')
      router.navigate({ to: '/users' })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createMutation.mutate()
  }

  if (!hasPermission('users:write')) {
    return null
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" aria-label="Back to users" onClick={() => router.navigate({ to: '/users' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <h1 className="text-2xl font-bold">Create New User</h1>
      </div>

      <form onSubmit={handleSubmit}>
        <Card>
          <CardHeader>
            <CardTitle>User Information</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {createMutation.error && (
              <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
                {createMutation.error.message}
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="uid">Username (UID) *</Label>
              <Input
                id="uid"
                value={formData.uid}
                onChange={(e) => setFormData({ ...formData, uid: e.target.value })}
                required
              />
            </div>

            <div className="grid grid-cols-2 gap-4">
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
            </div>

            <div className="grid grid-cols-2 gap-4">
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
            </div>

            <div className="grid grid-cols-2 gap-4">
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
                    aria-label="Auto-generate next available UID"
                    title="Auto-generate next available UID"
                    onClick={() => nextIds && setFormData({ ...formData, uidNumber: nextIds.nextUid.toString() })}
                    disabled={!nextIds}
                  >
                    <Wand2 className="h-4 w-4" />
                  </Button>
                </div>
                {nextIds && <p className="text-xs text-muted-foreground">Min: {nextIds.minUid}, Next available: {nextIds.nextUid}</p>}
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
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
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
            </div>

            <div className="grid grid-cols-4 gap-4">
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
            </div>

            <div className="grid grid-cols-2 gap-4">
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
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Input
                id="description"
                value={formData.description}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
              />
            </div>

            {showPoliciesModule && (
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
                  Optional. If set, the account will be locked on this date.
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
                        className="hover:bg-primary-foreground/20 rounded"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
                </div>
              )}
              <div className="relative">
                <Input
                  placeholder="Search groups to add..."
                  value={groupSearch}
                  onChange={(e) => setGroupSearch(e.target.value)}
                />
              </div>
              {groupSearch && (
                <div className="max-h-40 overflow-y-auto border rounded-md bg-background">
                  {availableGroups.filter(g => !selectedGroups.includes(g.cn)).length > 0 ? (
                    availableGroups
                      .filter(g => !selectedGroups.includes(g.cn))
                      .slice(0, 10)
                      .map((group) => (
                        <button
                          key={group.dn}
                          type="button"
                          onClick={() => {
                            setSelectedGroups([...selectedGroups, group.cn])
                            setGroupSearch('')
                          }}
                          className="w-full flex items-center gap-2 p-2 text-left hover:bg-muted transition-colors"
                        >
                          <Users className="h-4 w-4 text-muted-foreground" />
                          <span className="font-medium">{group.cn}</span>
                        </button>
                      ))
                  ) : (
                    <p className="text-sm text-muted-foreground text-center py-2">
                      No matching groups
                    </p>
                  )}
                </div>
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
