import { createFileRoute, Link, redirect, useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ArrowLeft, Save, Trash2 } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useState } from 'react'
import { decodeDN } from '@/lib/utils'
import { toast } from 'sonner'

export const Route = createFileRoute('/password-policies/$dn')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  component: PasswordPolicyDetailPage,
})

function PasswordPolicyDetailPage() {
  const { dn: encodedDn } = Route.useParams()
  const dn = decodeDN(encodedDn)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { hasPermission } = useAuth()
  const canWrite = hasPermission('settings:write')

  const [showDeleteDialog, setShowDeleteDialog] = useState(false)
  const { data: policy, isLoading, error } = useQuery({
    queryKey: ['password-policy', dn],
    queryFn: ({ signal }) => api.passwordPolicies.get(dn, signal),
  })

  const [formData, setFormData] = useState({
    description: policy?.description || '',
    pwdAttribute: policy?.pwdAttribute || 'userPassword',
    pwdMinAge: policy?.pwdMinAge || 0,
    pwdMaxAge: policy?.pwdMaxAge || 0,
    pwdInHistory: policy?.pwdInHistory || 0,
    pwdCheckQuality: policy?.pwdCheckQuality || 0,
    pwdMinLength: policy?.pwdMinLength || 0,
    pwdExpireWarning: policy?.pwdExpireWarning || 0,
    pwdGraceAuthNLimit: policy?.pwdGraceAuthNLimit || 0,
    pwdLockout: policy?.pwdLockout || false,
    pwdLockoutDuration: policy?.pwdLockoutDuration || 0,
    pwdMaxFailure: policy?.pwdMaxFailure || 0,
    pwdFailureCountInterval: policy?.pwdFailureCountInterval || 0,
    pwdMustChange: policy?.pwdMustChange || false,
    pwdAllowUserChange: policy?.pwdAllowUserChange ?? true,
    pwdSafeModify: policy?.pwdSafeModify || false,
    pwdCheckModule: policy?.pwdCheckModule || '',
  })

  const updateMutation = useMutation({
    mutationFn: () => api.passwordPolicies.update(dn, formData),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['password-policy', dn] })
      queryClient.invalidateQueries({ queryKey: ['password-policies'] })
      toast.success('Password policy updated')
    },
    onError: (error) => {
      toast.error(error.message)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => api.passwordPolicies.delete(dn),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['password-policies'] })
      toast.success('Password policy deleted')
      navigate({ to: '/password-policies' })
    },
    onError: (error) => {
      toast.error(error.message)
    },
  })

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
      </div>
    )
  }

  if (error || !policy) {
    return (
      <div className="p-4 text-destructive bg-destructive/10 rounded-md">
        Failed to load password policy: {error?.message || 'Not found'}
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <Link to="/password-policies">
            <Button variant="ghost" size="icon" aria-label="Back to password policies">
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div>
            <h1 className="text-2xl font-bold">{policy.cn}</h1>
            <p className="text-sm text-muted-foreground font-mono">{policy.dn}</p>
          </div>
        </div>
        <div className="flex gap-2">
          {canWrite && (
            <>
              <Button
                variant="destructive"
                onClick={() => setShowDeleteDialog(true)}
              >
                <Trash2 className="h-4 w-4 mr-1" />
                Delete
              </Button>
              <Button
                onClick={() => updateMutation.mutate()}
                disabled={updateMutation.isPending}
              >
                <Save className="h-4 w-4 mr-1" />
                {updateMutation.isPending ? 'Saving...' : 'Save Changes'}
              </Button>
            </>
          )}
        </div>
      </div>

      <div className="grid gap-6">
        <Card>
          <CardHeader>
            <CardTitle>General Settings</CardTitle>
            <CardDescription>Basic policy configuration</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Textarea
                id="description"
                value={formData.description}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                disabled={!canWrite}
                rows={2}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pwdAttribute">Password Attribute</Label>
              <Input
                id="pwdAttribute"
                value={formData.pwdAttribute}
                onChange={(e) => setFormData({ ...formData, pwdAttribute: e.target.value })}
                disabled={!canWrite}
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
                  disabled={!canWrite}
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
                  disabled={!canWrite}
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
                disabled={!canWrite}
                placeholder="e.g., check_password.so"
              />
              <p className="text-xs text-muted-foreground">
                External module for password quality checking
              </p>
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
                  disabled={!canWrite}
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
                  disabled={!canWrite}
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
                  disabled={!canWrite}
                />
                <p className="text-xs text-muted-foreground">
                  Time before expiration to start warning
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="pwdGraceAuthNLimit">Grace Logins</Label>
                <Input
                  id="pwdGraceAuthNLimit"
                  type="number"
                  min="0"
                  value={formData.pwdGraceAuthNLimit}
                  onChange={(e) => setFormData({ ...formData, pwdGraceAuthNLimit: parseInt(e.target.value) || 0 })}
                  disabled={!canWrite}
                />
                <p className="text-xs text-muted-foreground">
                  Allowed logins after password expires
                </p>
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
                disabled={!canWrite}
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
                disabled={!canWrite}
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
                    disabled={!canWrite}
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
                    disabled={!canWrite}
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
                    disabled={!canWrite}
                  />
                  <p className="text-xs text-muted-foreground">
                    Time to reset failure count
                  </p>
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
                disabled={!canWrite}
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
                disabled={!canWrite}
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
                disabled={!canWrite}
              />
            </div>
          </CardContent>
        </Card>
      </div>

      <Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Password Policy</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete "{policy.cn}"?
              This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDeleteDialog(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteMutation.mutate()}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
