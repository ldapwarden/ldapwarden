import { createRootRouteWithContext, Outlet, Link, useRouter, useRouterState } from '@tanstack/react-router'
import { encodeDN } from '@/lib/utils'
import { useSections, useIsAdmin } from '@/lib/sections'
import { pageTitle } from '@/lib/page-title'
import { BuildFooter } from '@/components/build-footer'
import { lazy, Suspense } from 'react'
import type { QueryClient } from '@tanstack/react-query'
import { Toaster, toast } from 'sonner'

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
import { useTheme } from '@/lib/theme'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { ThemeToggle } from '@/components/theme-toggle'
import { PasswordStrength } from '@/components/ui/password-strength'
import { PasswordInput } from '@/components/ui/password-input'
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
import { User, LogOut, Shield, Settings, ChevronDown, Key, LayoutDashboard, Menu, Search } from 'lucide-react'
import { CommandPalette } from '@/components/command-palette'
import { useCommandHotkey } from '@/lib/use-hotkey'
import { RouteError } from '@/components/route-error'
import { LoadingScreen } from '@/components/loading-screen'
import { SessionExpiryNotice } from '@/components/session-expiry-notice'
import { OutcomeHost } from '@/components/outcome-dialog'
import { useCallback, useState, useEffect } from 'react'

interface RouterContext {
  auth: ReturnType<typeof useAuth>
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootComponent,
  errorComponent: RouteError,
})

function RootComponent() {
  const { isAuthenticated, session, logout, isLoading, sessionExpired } = useAuth()
  const { theme } = useTheme()
  const router = useRouter()
  const [passwordDialogOpen, setPasswordDialogOpen] = useState(false)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')

  // Fetch the current user's avatar. It is served separately from the session
  // (which no longer carries the base64 photo) and cached so it is loaded once
  // per app session rather than on every request.
  const { data: avatar } = useQuery({
    queryKey: ['me-avatar'],
    queryFn: ({ signal }) => api.auth.avatar(signal),
    enabled: isAuthenticated,
    staleTime: 5 * 60 * 1000,
  })

  // One list for the bar, the mobile menu and the command palette, with one
  // set of visibility rules: see lib/sections.ts.
  const sections = useSections()
  const isAdmin = useIsAdmin()

  const changePasswordMutation = useMutation({
    mutationFn: async () => {
      return api.auth.changePassword(currentPassword, newPassword)
    },
    onSuccess: () => {
      closePasswordDialog()
      toast.success('Password changed')
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  // Public routes render without a session; everything else is protected.
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const isPublicRoute = pathname === '/login' || pathname.startsWith('/reset-password')

  // When the session ends mid-visit (expiry or server-side revocation), a 401
  // clears the auth state via onAuthError. Route beforeLoad guards only run on
  // navigation, so without this a user sitting on a protected page would be
  // stranded on a chrome-less error screen instead of the login page. Redirect
  // them explicitly, preserving where they were so login can send them back.
  useEffect(() => {
    if (!isLoading && !isAuthenticated && !isPublicRoute) {
      // Carry the reason across: landing on a bare sign-in form gave no clue
      // whether the session had ended, a click had gone wrong, or the app had
      // broken — and the work in the abandoned form was gone either way.
      router.navigate({
        to: '/login',
        search: { redirect: pathname, ...(sessionExpired ? { reason: 'expired' as const } : {}) },
      })
    }
  }, [isLoading, isAuthenticated, isPublicRoute, pathname, router, sessionExpired])

  // Keep the browser tab title in sync with the current section. Centralised
  // here (keyed off the path) so individual route files don't each have to
  // manage document.title.
  useEffect(() => {
    document.title = pageTitle(pathname)
  }, [pathname])

  // Every way out of the dialog clears it: the current password must not sit
  // in state for the next time it opens.
  const closePasswordDialog = () => {
    setPasswordDialogOpen(false)
    setCurrentPassword('')
    setNewPassword('')
    setConfirmPassword('')
    changePasswordMutation.reset()
  }

  const passwordsMatch = newPassword && confirmPassword && newPassword === confirmPassword
  const passwordsDontMatch = newPassword && confirmPassword && newPassword !== confirmPassword

  useCommandHotkey(useCallback(() => setPaletteOpen((open) => !open), []))

  const handlePasswordChange = (e: React.FormEvent) => {
    e.preventDefault()
    if (currentPassword && newPassword && newPassword === confirmPassword) {
      changePasswordMutation.mutate()
    }
  }

  // isLoading is never true here: main.tsx holds the router back until the
  // session probe answers, so guards and this component always see a settled
  // auth state.
  if (!isAuthenticated) {
    // Public routes (login, password reset) render normally. On a protected
    // route the effect above is navigating to /login — show the spinner rather
    // than briefly rendering the now-unauthenticated page's error state.
    if (!isPublicRoute) {
      return <LoadingScreen />
    }
    return <Outlet />
  }

  const handleLogout = async () => {
    await logout()
    router.navigate({ to: '/login' })
  }

  return (
    <div className="min-h-screen bg-background">
      {/* Hidden until focused. Without it the keyboard traverses the whole nav —
          up to five section links, the search button, the theme menu and the
          account menu — before reaching the table, on every page. */}
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2 focus:text-sm focus:font-medium focus:ring-2 focus:ring-ring"
      >
        Skip to content
      </a>
      <nav className="border-b bg-card">
        <div className="container mx-auto px-4">
          <div className="flex h-14 items-center justify-between">
            <div className="flex min-w-0 items-center gap-2 md:gap-6">
              {/* Below md the five section links do not fit next to the account
                  menu, so they move into this dropdown. Without it the nav row
                  pushed the document to ~900px on a phone: the account menu was
                  unreachable and every page scrolled sideways. */}
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon" className="md:hidden" aria-label="Main menu">
                    <Menu className="h-5 w-5" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="w-56">
                  {/* Search first: below sm the header has no room for its
                      button, and a phone has no ⌘K, so this is the only way in. */}
                  <DropdownMenuItem onClick={() => setPaletteOpen(true)} className="cursor-pointer sm:hidden">
                    <Search className="mr-2 h-4 w-4" />
                    Search
                  </DropdownMenuItem>
                  <DropdownMenuSeparator className="sm:hidden" />
                  {[{ to: '/', label: 'Dashboard', icon: LayoutDashboard }, ...sections].map(({ to, label, icon: Icon }) => (
                    <DropdownMenuItem key={to} asChild>
                      <Link
                        to={to}
                        activeOptions={{ exact: to === '/' }}
                        className="flex cursor-pointer items-center data-[status=active]:bg-accent data-[status=active]:font-medium"
                      >
                        {({ isActive }) => (
                          <>
                            <Icon className="mr-2 h-4 w-4" />
                            {label}
                            {isActive && <span className="sr-only"> (current page)</span>}
                          </>
                        )}
                      </Link>
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>

              <Link to="/" className="flex shrink-0 items-center gap-2 font-semibold">
                <Shield className="h-5 w-5" />
                <span className="hidden sm:inline">LDAP Warden</span>
              </Link>

              <div className="hidden items-center gap-1 md:flex">
                {sections.map(({ to, label, shortLabel, icon: Icon }) => (
                  <Link key={to} to={to}>
                    {({ isActive }) => (
                      <Button variant={isActive ? 'secondary' : 'ghost'} size="sm" title={shortLabel ? label : undefined}>
                        <Icon className="h-4 w-4 mr-1" />
                        {shortLabel ?? label}
                      </Button>
                    )}
                  </Link>
                ))}
              </div>
            </div>
            <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              className="hidden items-center gap-2 text-muted-foreground sm:flex"
              onClick={() => setPaletteOpen(true)}
            >
              <Search className="h-4 w-4" />
              <span className="hidden lg:inline">Search</span>
              <kbd className="hidden rounded border px-1 text-xs font-sans lg:inline">⌘K</kbd>
            </Button>
            <ThemeToggle />
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" aria-label="Account menu" className="flex items-center gap-2 px-2">
                  <Avatar
                    src={avatar?.jpegPhoto}
                    fallback={session?.displayName}
                    size="sm"
                  />
                  <div className="hidden flex-col items-start text-left sm:flex">
                    <span className="text-sm font-medium">{session?.displayName}</span>
                    <span className="text-xs text-muted-foreground">{session?.userUid}</span>
                  </div>
                  <ChevronDown className="hidden h-4 w-4 text-muted-foreground sm:block" />
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
                {isAdmin && (
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
            </div>

            <Dialog open={passwordDialogOpen} onOpenChange={(open) => (open ? setPasswordDialogOpen(true) : closePasswordDialog())}>
              <DialogContent className="sm:max-w-md">
                <DialogHeader>
                  <DialogTitle>Change Password</DialogTitle>
                  <DialogDescription>
                    Enter your current password, then a new one. Your other sessions
                    will be signed out.
                  </DialogDescription>
                </DialogHeader>
                <form onSubmit={handlePasswordChange} className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="currentPassword">Current Password</Label>
                    <PasswordInput
                      id="currentPassword"
                      value={currentPassword}
                      onValueChange={setCurrentPassword}
                      autoComplete="current-password"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="newPassword">New Password</Label>
                    <PasswordInput
                      id="newPassword"
                      value={newPassword}
                      onValueChange={setNewPassword}
                      autoComplete="new-password"
                    />
                    <PasswordStrength password={newPassword} />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="confirmPassword">Confirm Password</Label>
                    <PasswordInput
                      id="confirmPassword"
                      value={confirmPassword}
                      onValueChange={setConfirmPassword}
                      autoComplete="new-password"
                    />
                    {passwordsDontMatch && (
                      <p className="text-xs text-destructive">Passwords do not match</p>
                    )}
                    {passwordsMatch && (
                      <p className="text-xs text-success">Passwords match</p>
                    )}
                  </div>

                  <div className="flex justify-end gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={closePasswordDialog}
                    >
                      Cancel
                    </Button>
                    <Button
                      type="submit"
                      disabled={!currentPassword || !passwordsMatch || changePasswordMutation.isPending}
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
      <main id="main" tabIndex={-1} className="container mx-auto px-4 py-6">
        <SessionExpiryNotice />
        <Outlet />
      </main>
      <BuildFooter />
      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
      <Suspense>
        <TanStackRouterDevtools />
      </Suspense>
      <Toaster richColors position="top-right" theme={theme} />
      <OutcomeHost />
    </div>
  )
}
