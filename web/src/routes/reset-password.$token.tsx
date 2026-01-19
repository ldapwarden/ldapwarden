import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery, useMutation } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Shield, Key, CheckCircle2, AlertCircle } from 'lucide-react'
import { useState, useMemo } from 'react'

export const Route = createFileRoute('/reset-password/$token')({
  component: ResetPasswordPage,
})

function ResetPasswordPage() {
  const { token } = Route.useParams()
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')

  const { data: info, isLoading, error } = useQuery({
    queryKey: ['password-reset', token],
    queryFn: () => api.passwordReset.getInfo(token),
  })

  const resetMutation = useMutation({
    mutationFn: () => api.passwordReset.confirm(token, password),
  })

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

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (password && password === confirmPassword) {
      resetMutation.mutate()
    }
  }

  if (isLoading) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center p-4">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardHeader className="text-center">
            <div className="mx-auto w-12 h-12 rounded-full bg-destructive/10 flex items-center justify-center mb-4">
              <AlertCircle className="h-6 w-6 text-destructive" />
            </div>
            <CardTitle>Invalid or Expired Link</CardTitle>
            <CardDescription>
              This password reset link is invalid or has expired. Please request a new password reset link.
            </CardDescription>
          </CardHeader>
          <CardContent className="text-center">
            <Link to="/login">
              <Button variant="outline">Go to Login</Button>
            </Link>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (resetMutation.isSuccess) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardHeader className="text-center">
            <div className="mx-auto w-12 h-12 rounded-full bg-green-100 flex items-center justify-center mb-4">
              <CheckCircle2 className="h-6 w-6 text-green-600" />
            </div>
            <CardTitle>Password Changed</CardTitle>
            <CardDescription>
              Your password has been successfully changed. You can now log in with your new password.
            </CardDescription>
          </CardHeader>
          <CardContent className="text-center">
            <Link to="/login">
              <Button>Go to Login</Button>
            </Link>
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <div className="mx-auto w-12 h-12 rounded-full bg-primary/10 flex items-center justify-center mb-4">
            <Shield className="h-6 w-6 text-primary" />
          </div>
          <CardTitle>{info?.organization}</CardTitle>
          <CardDescription>
            Reset password for <span className="font-medium">{info?.displayName}</span> ({info?.uid})
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            {resetMutation.error && (
              <div className="p-3 text-sm text-destructive bg-destructive/10 rounded-md">
                {resetMutation.error.message}
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="password">New Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="new-password"
                autoFocus
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
                autoComplete="new-password"
              />
              {passwordsDontMatch && (
                <p className="text-xs text-destructive">Passwords do not match</p>
              )}
              {passwordsMatch && (
                <p className="text-xs text-green-600">Passwords match</p>
              )}
            </div>

            <Button
              type="submit"
              className="w-full"
              disabled={!passwordsMatch || resetMutation.isPending}
            >
              <Key className="h-4 w-4 mr-1" />
              {resetMutation.isPending ? 'Resetting...' : 'Reset Password'}
            </Button>
          </form>

          <div className="mt-4 text-center">
            <Link to="/login" className="text-sm text-muted-foreground hover:underline">
              Back to login
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
