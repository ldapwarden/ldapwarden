import { createFileRoute, Link, redirect, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ArrowLeft, Save } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { encodeDN } from '@/lib/utils'

export const Route = createFileRoute('/password-policies/new')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  component: NewPasswordPolicyPage,
})

function NewPasswordPolicyPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [formData, setFormData] = useState({
    cn: '',
    description: '',
    pwdAttribute: 'userPassword',
    pwdMinAge: 0,
    pwdMaxAge: 0,
    pwdInHistory: 0,
    pwdCheckQuality: 0,
    pwdMinLength: 8,
    pwdExpireWarning: 0,
    pwdGraceAuthNLimit: 0,
    pwdLockout: false,
    pwdLockoutDuration: 0,
    pwdMaxFailure: 5,
    pwdFailureCountInterval: 0,
    pwdMustChange: false,
    pwdAllowUserChange: true,
    pwdSafeModify: false,
    pwdCheckModule: '',
  })

  const createMutation = useMutation({
    mutationFn: () => api.passwordPolicies.create(formData),
    onSuccess: (policy) => {
      queryClient.invalidateQueries({ queryKey: ['password-policies'] })
      toast.success('Password policy created')
      navigate({ to: '/password-policies/$dn', params: { dn: encodeDN(policy.dn) } })
    },
    onError: (error) => {
      toast.error(error.message)
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!formData.cn.trim()) {
      toast.error('Policy name is required')
      return
    }
    createMutation.mutate()
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link to="/password-policies">
          <Button variant="ghost" size="icon" aria-label="Back to password policies">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <h1 className="text-2xl font-bold">New Password Policy</h1>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle>General Settings</CardTitle>
            <CardDescription>Basic policy configuration</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="cn">Policy Name *</Label>
              <Input
                id="cn"
                value={formData.cn}
                onChange={(e) => setFormData({ ...formData, cn: e.target.value })}
                placeholder="e.g., default-policy"
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Textarea
                id="description"
                value={formData.description}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                rows={2}
                placeholder="Description of this policy"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pwdAttribute">Password Attribute</Label>
              <Input
                id="pwdAttribute"
                value={formData.pwdAttribute}
                onChange={(e) => setFormData({ ...formData, pwdAttribute: e.target.value })}
                placeholder="userPassword"
              />
              <p className="text-xs text-muted-foreground">
                The LDAP attribute this policy applies to (usually userPassword)
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Password Quality</CardTitle>
            <CardDescription>Requirements for password strength</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="pwdMinLength">Minimum Length</Label>
                <Input
                  id="pwdMinLength"
                  type="number"
                  min="0"
                  value={formData.pwdMinLength}
                  onChange={(e) => setFormData({ ...formData, pwdMinLength: parseInt(e.target.value) || 0 })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="pwdCheckQuality">Check Quality</Label>
                <Input
                  id="pwdCheckQuality"
                  type="number"
                  min="0"
                  max="2"
                  value={formData.pwdCheckQuality}
                  onChange={(e) => setFormData({ ...formData, pwdCheckQuality: parseInt(e.target.value) || 0 })}
                />
                <p className="text-xs text-muted-foreground">
                  0=disabled, 1=check if module, 2=always check
                </p>
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="pwdCheckModule">Check Module</Label>
              <Input
                id="pwdCheckModule"
                value={formData.pwdCheckModule}
                onChange={(e) => setFormData({ ...formData, pwdCheckModule: e.target.value })}
                placeholder="e.g., check_password.so"
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Password Expiration</CardTitle>
            <CardDescription>Settings for password aging (values in seconds)</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="pwdMinAge">Minimum Age (seconds)</Label>
                <Input
                  id="pwdMinAge"
                  type="number"
                  min="0"
                  value={formData.pwdMinAge}
                  onChange={(e) => setFormData({ ...formData, pwdMinAge: parseInt(e.target.value) || 0 })}
                />
                <p className="text-xs text-muted-foreground">
                  Time before password can be changed (0 = no minimum)
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="pwdMaxAge">Maximum Age (seconds)</Label>
                <Input
                  id="pwdMaxAge"
                  type="number"
                  min="0"
                  value={formData.pwdMaxAge}
                  onChange={(e) => setFormData({ ...formData, pwdMaxAge: parseInt(e.target.value) || 0 })}
                />
                <p className="text-xs text-muted-foreground">
                  Password expiration time (0 = never expires)
                </p>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="pwdExpireWarning">Expire Warning (seconds)</Label>
                <Input
                  id="pwdExpireWarning"
                  type="number"
                  min="0"
                  value={formData.pwdExpireWarning}
                  onChange={(e) => setFormData({ ...formData, pwdExpireWarning: parseInt(e.target.value) || 0 })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="pwdGraceAuthNLimit">Grace Logins</Label>
                <Input
                  id="pwdGraceAuthNLimit"
                  type="number"
                  min="0"
                  value={formData.pwdGraceAuthNLimit}
                  onChange={(e) => setFormData({ ...formData, pwdGraceAuthNLimit: parseInt(e.target.value) || 0 })}
                />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Password History</CardTitle>
            <CardDescription>Prevent password reuse</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              <Label htmlFor="pwdInHistory">Passwords in History</Label>
              <Input
                id="pwdInHistory"
                type="number"
                min="0"
                value={formData.pwdInHistory}
                onChange={(e) => setFormData({ ...formData, pwdInHistory: parseInt(e.target.value) || 0 })}
              />
              <p className="text-xs text-muted-foreground">
                Number of previous passwords to remember (0 = disabled)
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Account Lockout</CardTitle>
            <CardDescription>Lock accounts after failed login attempts</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label>Enable Lockout</Label>
                <p className="text-xs text-muted-foreground">
                  Lock account after too many failed attempts
                </p>
              </div>
              <Switch
                checked={formData.pwdLockout}
                onCheckedChange={(checked) => setFormData({ ...formData, pwdLockout: checked })}
              />
            </div>
            {formData.pwdLockout && (
              <div className="grid grid-cols-3 gap-4 pt-4 border-t">
                <div className="space-y-2">
                  <Label htmlFor="pwdMaxFailure">Max Failures</Label>
                  <Input
                    id="pwdMaxFailure"
                    type="number"
                    min="0"
                    value={formData.pwdMaxFailure}
                    onChange={(e) => setFormData({ ...formData, pwdMaxFailure: parseInt(e.target.value) || 0 })}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="pwdLockoutDuration">Lockout Duration (s)</Label>
                  <Input
                    id="pwdLockoutDuration"
                    type="number"
                    min="0"
                    value={formData.pwdLockoutDuration}
                    onChange={(e) => setFormData({ ...formData, pwdLockoutDuration: parseInt(e.target.value) || 0 })}
                  />
                  <p className="text-xs text-muted-foreground">
                    0 = permanent lock
                  </p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="pwdFailureCountInterval">Failure Reset (s)</Label>
                  <Input
                    id="pwdFailureCountInterval"
                    type="number"
                    min="0"
                    value={formData.pwdFailureCountInterval}
                    onChange={(e) => setFormData({ ...formData, pwdFailureCountInterval: parseInt(e.target.value) || 0 })}
                  />
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>User Options</CardTitle>
            <CardDescription>Control user password change behavior</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label>Must Change on Reset</Label>
                <p className="text-xs text-muted-foreground">
                  Require password change after admin reset
                </p>
              </div>
              <Switch
                checked={formData.pwdMustChange}
                onCheckedChange={(checked) => setFormData({ ...formData, pwdMustChange: checked })}
              />
            </div>
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label>Allow User Change</Label>
                <p className="text-xs text-muted-foreground">
                  Allow users to change their own password
                </p>
              </div>
              <Switch
                checked={formData.pwdAllowUserChange}
                onCheckedChange={(checked) => setFormData({ ...formData, pwdAllowUserChange: checked })}
              />
            </div>
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label>Safe Modify</Label>
                <p className="text-xs text-muted-foreground">
                  Require current password when changing
                </p>
              </div>
              <Switch
                checked={formData.pwdSafeModify}
                onCheckedChange={(checked) => setFormData({ ...formData, pwdSafeModify: checked })}
              />
            </div>
          </CardContent>
        </Card>

        <div className="flex justify-end">
          <Button type="submit" disabled={createMutation.isPending}>
            <Save className="h-4 w-4 mr-1" />
            {createMutation.isPending ? 'Creating...' : 'Create Policy'}
          </Button>
        </div>
      </form>
    </div>
  )
}
