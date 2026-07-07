import { createFileRoute, useRouter } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import { useAuth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader } from '@/components/ui/card'

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): { redirect?: string } => ({
    redirect: typeof search.redirect === 'string' ? search.redirect : undefined,
  }),
  component: LoginPage,
})

// safeRedirect guards against open-redirect: only same-origin absolute paths
// ("/users", "/groups/…") are honoured, never "//evil.com" or "https://…".
function safeRedirect(target: string | undefined): string {
  if (target && target.startsWith('/') && !target.startsWith('//')) {
    return target
  }
  return '/'
}

function LoginPage() {
  const { login, isAuthenticated } = useAuth()
  const router = useRouter()
  const { redirect } = Route.useSearch()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(false)

  useEffect(() => {
    if (isAuthenticated) {
      router.navigate({ to: safeRedirect(redirect) })
    }
  }, [isAuthenticated, redirect, router])

  if (isAuthenticated) {
    return null
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setIsLoading(true)

    try {
      await login(username, password)
      // Navigation is handled by the useEffect when isAuthenticated changes
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="relative flex items-center justify-center min-h-screen overflow-hidden bg-[#0a0a1a]">
      {/* Aurora background layers */}
      <div className="absolute inset-0">
        {/* Base gradient */}
        <div className="absolute inset-0 bg-linear-to-b from-[#0a0a1a] via-[#0d1033] to-[#0a0a1a]" />

        {/* Aurora wave 1 */}
        <div
          className="absolute inset-0 opacity-60"
          style={{
            background: 'radial-gradient(ellipse 80% 50% at 50% 20%, #1247db 0%, transparent 50%)',
            animation: 'aurora1 8s ease-in-out infinite',
          }}
        />

        {/* Aurora wave 2 */}
        <div
          className="absolute inset-0 opacity-50"
          style={{
            background: 'radial-gradient(ellipse 60% 40% at 30% 30%, #36aaf5 0%, transparent 50%)',
            animation: 'aurora2 10s ease-in-out infinite',
          }}
        />

        {/* Aurora wave 3 */}
        <div
          className="absolute inset-0 opacity-40"
          style={{
            background: 'radial-gradient(ellipse 70% 35% at 70% 25%, #0ea5e9 0%, transparent 50%)',
            animation: 'aurora3 12s ease-in-out infinite',
          }}
        />

        {/* Aurora wave 4 - accent */}
        <div
          className="absolute inset-0 opacity-30"
          style={{
            background: 'radial-gradient(ellipse 50% 30% at 60% 40%, #6366f1 0%, transparent 50%)',
            animation: 'aurora4 9s ease-in-out infinite',
          }}
        />

        {/* Subtle stars/particles */}
        <div className="absolute inset-0 opacity-40" style={{
          backgroundImage: `radial-gradient(1px 1px at 20px 30px, white, transparent),
                           radial-gradient(1px 1px at 40px 70px, rgba(255,255,255,0.8), transparent),
                           radial-gradient(1px 1px at 50px 160px, rgba(255,255,255,0.6), transparent),
                           radial-gradient(1px 1px at 90px 40px, white, transparent),
                           radial-gradient(1px 1px at 130px 80px, rgba(255,255,255,0.7), transparent),
                           radial-gradient(1px 1px at 160px 120px, white, transparent)`,
          backgroundSize: '200px 200px',
        }} />
      </div>

      {/* CSS Animations */}
      <style>{`
        @keyframes aurora1 {
          0%, 100% { transform: translateY(0) translateX(0) scale(1); opacity: 0.6; }
          25% { transform: translateY(-5%) translateX(5%) scale(1.1); opacity: 0.7; }
          50% { transform: translateY(-10%) translateX(-5%) scale(1); opacity: 0.5; }
          75% { transform: translateY(-5%) translateX(-10%) scale(1.05); opacity: 0.65; }
        }
        @keyframes aurora2 {
          0%, 100% { transform: translateY(0) translateX(0) scale(1); opacity: 0.5; }
          33% { transform: translateY(-8%) translateX(10%) scale(1.15); opacity: 0.6; }
          66% { transform: translateY(-3%) translateX(-8%) scale(0.95); opacity: 0.45; }
        }
        @keyframes aurora3 {
          0%, 100% { transform: translateY(0) translateX(0) scale(1); opacity: 0.4; }
          50% { transform: translateY(-12%) translateX(-15%) scale(1.2); opacity: 0.55; }
        }
        @keyframes aurora4 {
          0%, 100% { transform: translateY(0) translateX(0) scale(1); opacity: 0.3; }
          40% { transform: translateY(-6%) translateX(12%) scale(1.1); opacity: 0.4; }
          80% { transform: translateY(-3%) translateX(-6%) scale(0.9); opacity: 0.25; }
        }
      `}</style>

      {/* Login card */}
      {/* The card is intentionally always white (it sits on a fixed dark
          gradient, theme-independent). Force dark text so inputs, labels and
          description stay readable in dark mode — otherwise they inherit the
          dark-theme light foreground and vanish on the white surface. */}
      <Card className="relative z-10 w-full max-w-md mx-4 bg-white/95 text-gray-900 backdrop-blur-xs shadow-2xl">
        <CardHeader className="text-center">
          <div className="flex justify-center mb-4">
            <img src="/ldapwarden-logo.svg" alt="LDAP Warden" className="h-24 w-auto" />
          </div>
          <CardDescription className="text-gray-600">Sign in with your LDAP credentials</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
                {error}
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="username">Username</Label>
              <Input
                id="username"
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="Enter your username"
                className="placeholder:text-gray-400"
                required
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter your password"
                className="placeholder:text-gray-400"
                required
              />
            </div>
            <Button type="submit" className="w-full" disabled={isLoading}>
              {isLoading ? 'Signing in...' : 'Sign In'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
