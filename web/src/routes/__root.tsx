import { createRootRouteWithContext, Outlet, Link, useRouter } from '@tanstack/react-router'
import { encodeDN } from '@/lib/utils'
import { lazy, Suspense } from 'react'
import type { QueryClient } from '@tanstack/react-query'
import { Toaster } from 'sonner'

// Only load devtools in development
const TanStackRouterDevtools = import.meta.env.DEV
  ? lazy(() =>
      import('@tanstack/router-devtools').then((m) => ({
        default: m.TanStackRouterDevtools,
      }))
    )
  : () => null
import { useMutation, useQuery } from '@tanstack/react-query'
import { useAuth } from '@/lib/auth'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Avatar } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Users, UsersRound, User, ScrollText, LogOut, Shield, ShieldCheck, Settings, ChevronDown, Key, KeyRound } from 'lucide-react'
import { useState, useMemo } from 'react'

interface RouterContext {
  auth: ReturnType<typeof useAuth>
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootComponent,
})

function RootComponent() {
  const { isAuthenticated, session, logout, isLoading } = useAuth()
  const router = useRouter()
  const [passwordDialogOpen, setPasswordDialogOpen] = useState(false)
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')

  // Fetch config to get enabled modules
  const { data: config } = useQuery({
    queryKey: ['config'],
    queryFn: () => api.admin.getConfig(),
    enabled: isAuthenticated && session?.roleName === 'admin',
    staleTime: 5 * 60 * 1000, // Cache for 5 minutes
  })

  // Get enabled modules from config (default to all if not available)
  const enabledModules = useMemo(() => {
    const modulesValue = config?.app?.modules?.value
    if (Array.isArray(modulesValue)) {
      return modulesValue as string[]
    }
    // Default modules if config not loaded (for non-admin users)
    return ['users', 'groups', 'sudo', 'policies']
  }, [config])

  const isModuleEnabled = (module: string) => enabledModules.includes(module)

  const changePasswordMutation = useMutation({
    mutationFn: async () => {
      return api.auth.changePassword(newPassword)
    },
    onSuccess: () => {
      setPasswordDialogOpen(false)
      setNewPassword('')
      setConfirmPassword('')
    },
  })

  const passwordStrength = useMemo(() => {
    if (!newPassword) return { score: 0, label: '', color: '' }

    let score = 0
    if (newPassword.length >= 8) score++
    if (newPassword.length >= 12) score++
    if (/[a-z]/.test(newPassword)) score++
    if (/[A-Z]/.test(newPassword)) score++
    if (/[0-9]/.test(newPassword)) score++
    if (/[^a-zA-Z0-9]/.test(newPassword)) score++

    if (score <= 2) return { score, label: 'Weak', color: 'bg-destructive' }
    if (score <= 4) return { score, label: 'Medium', color: 'bg-yellow-500' }
    return { score, label: 'Strong', color: 'bg-green-500' }
  }, [newPassword])

  const passwordsMatch = newPassword && confirmPassword && newPassword === confirmPassword
  const passwordsDontMatch = newPassword && confirmPassword && newPassword !== confirmPassword

  const handlePasswordChange = (e: React.FormEvent) => {
    e.preventDefault()
    if (newPassword && newPassword === confirmPassword) {
      changePasswordMutation.mutate()
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
      </div>
    )
  }

  if (!isAuthenticated) {
    return <Outlet />
  }

  const handleLogout = async () => {
    await logout()
    router.navigate({ to: '/login' })
  }

  return (
    <div className="min-h-screen bg-background">
      <nav className="border-b bg-card">
        <div className="container mx-auto px-4">
          <div className="flex h-14 items-center justify-between">
            <div className="flex items-center gap-6">
              <Link to="/" className="flex items-center gap-2 font-semibold">
                <Shield className="h-5 w-5" />
                LDAP Warden
              </Link>
              <div className="flex items-center gap-1">
                {isModuleEnabled('users') && (
                  <Link to="/users">
                    {({ isActive }) => (
                      <Button variant={isActive ? "secondary" : "ghost"} size="sm">
                        <Users className="h-4 w-4 mr-1" />
                        Users
                      </Button>
                    )}
                  </Link>
                )}
                {isModuleEnabled('groups') && (
                  <Link to="/groups">
                    {({ isActive }) => (
                      <Button variant={isActive ? "secondary" : "ghost"} size="sm">
                        <UsersRound className="h-4 w-4 mr-1" />
                        Groups
                      </Button>
                    )}
                  </Link>
                )}
                {isModuleEnabled('sudo') && (
                  <Link to="/sudo-roles">
                    {({ isActive }) => (
                      <Button variant={isActive ? "secondary" : "ghost"} size="sm">
                        <ShieldCheck className="h-4 w-4 mr-1" />
                        Sudo Roles
                      </Button>
                    )}
                  </Link>
                )}
                {isModuleEnabled('policies') && session?.permissions.includes('settings:read') && (
                  <Link to="/password-policies">
                    {({ isActive }) => (
                      <Button variant={isActive ? "secondary" : "ghost"} size="sm">
                        <KeyRound className="h-4 w-4 mr-1" />
                        Policies
                      </Button>
                    )}
                  </Link>
                )}
                {session?.permissions.includes('audit:read') && (
                  <Link to="/audit-logs">
                    {({ isActive }) => (
                      <Button variant={isActive ? "secondary" : "ghost"} size="sm">
                        <ScrollText className="h-4 w-4 mr-1" />
                        Audit Logs
                      </Button>
                    )}
                  </Link>
                )}
              </div>
            </div>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" className="flex items-center gap-2 px-2">
                  <Avatar
                    src={session?.jpegPhoto}
                    fallback={session?.displayName}
                    size="sm"
                  />
                  <div className="flex flex-col items-start text-left">
                    <span className="text-sm font-medium">{session?.displayName}</span>
                    <span className="text-xs text-muted-foreground">{session?.userUid}</span>
                  </div>
                  <ChevronDown className="h-4 w-4 text-muted-foreground" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-56">
                <DropdownMenuLabel className="font-normal">
                  <div className="flex flex-col space-y-1">
                    <p className="text-sm font-medium">{session?.displayName}</p>
                    <p className="text-xs text-muted-foreground">{session?.mail || session?.userUid}</p>
                    <p className="text-xs text-muted-foreground capitalize">Role: {session?.roleName}</p>
                  </div>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={() => setPasswordDialogOpen(true)}>
                  <Key className="mr-2 h-4 w-4" />
                  Change Password
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link to="/users/$dn" params={{ dn: encodeDN(session?.userDn ?? '') }} className="flex items-center cursor-pointer">
                    <User className="mr-2 h-4 w-4" />
                    My Profile
                  </Link>
                </DropdownMenuItem>
                {session?.roleName === 'admin' && (
                  <DropdownMenuItem asChild>
                    <Link to="/admin" className="flex items-center cursor-pointer">
                      <Settings className="mr-2 h-4 w-4" />
                      Administration
                    </Link>
                  </DropdownMenuItem>
                )}
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={handleLogout} className="text-destructive focus:text-destructive">
                  <LogOut className="mr-2 h-4 w-4" />
                  Logout
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>

            <Dialog open={passwordDialogOpen} onOpenChange={setPasswordDialogOpen}>
              <DialogContent className="sm:max-w-md">
                <DialogHeader>
                  <DialogTitle>Change Password</DialogTitle>
                  <DialogDescription>
                    Enter a new password for your account.
                  </DialogDescription>
                </DialogHeader>
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
                    <Label htmlFor="newPassword">New Password</Label>
                    <Input
                      id="newPassword"
                      type="password"
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      autoComplete="new-password"
                    />
                    {newPassword && (
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
                      autoComplete="new-password"
                    />
                    {passwordsDontMatch && (
                      <p className="text-xs text-destructive">Passwords do not match</p>
                    )}
                    {passwordsMatch && (
                      <p className="text-xs text-green-600">Passwords match</p>
                    )}
                  </div>

                  <div className="flex justify-end gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => {
                        setPasswordDialogOpen(false)
                        setNewPassword('')
                        setConfirmPassword('')
                        changePasswordMutation.reset()
                      }}
                    >
                      Cancel
                    </Button>
                    <Button
                      type="submit"
                      disabled={!passwordsMatch || changePasswordMutation.isPending}
                    >
                      <Key className="h-4 w-4 mr-1" />
                      {changePasswordMutation.isPending ? 'Changing...' : 'Change Password'}
                    </Button>
                  </div>
                </form>
              </DialogContent>
            </Dialog>
          </div>
        </div>
      </nav>
      <main className="container mx-auto px-4 py-6">
        <Outlet />
      </main>
      <Suspense>
        <TanStackRouterDevtools />
      </Suspense>
      <Toaster richColors position="top-right" />
    </div>
  )
}
