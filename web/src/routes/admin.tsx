import { InlineSpinner } from '@/components/inline-spinner'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useQuery, useMutation } from '@tanstack/react-query'
import { api, ConfigValue } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Server, Database, Key, Clock, Mail, Building, Play, History, Users, KeyRound, CheckCircle, XCircle, Loader2, CalendarClock } from 'lucide-react'
import { toast } from 'sonner'

export const Route = createFileRoute('/admin')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
    if (context.auth.session?.roleName !== 'admin') {
      throw redirect({ to: '/' })
    }
  },
  component: AdminPage,
})

function ConfigField({ label, config, description }: { label: string; config: ConfigValue; description?: string }) {
  const displayValue = Array.isArray(config.value) ? config.value.join(', ') : String(config.value)
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <Label className="text-sm font-medium">{label}</Label>
        <span className={`text-xs px-2 py-0.5 rounded-full ${
          config.source === 'env'
            ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'
            : 'bg-muted text-muted-foreground'
        }`}>
          {config.source === 'env' ? 'Environment' : 'Default'}
        </span>
      </div>
      <Input
        value={displayValue}
        disabled
        className="font-mono text-sm bg-muted"
      />
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        {description && <span>{description}</span>}
        {config.envVar && <code className="bg-muted px-1 py-0.5 rounded">{config.envVar}</code>}
      </div>
    </div>
  )
}

function AdminPage() {
  const { hasPermission } = useAuth()

  const { data: config, isLoading, error } = useQuery({
    queryKey: ['admin', 'config'],
    queryFn: ({ signal }) => api.admin.getConfig(signal),
    enabled: hasPermission('settings:read'),
  })

  const { data: scheduledTasksConfig } = useQuery({
    queryKey: ['admin', 'scheduled-tasks', 'config'],
    queryFn: ({ signal }) => api.admin.scheduledTasks.getConfig(signal),
    enabled: hasPermission('settings:read'),
  })

  const { data: taskRuns, refetch: refetchTaskRuns } = useQuery({
    queryKey: ['admin', 'scheduled-tasks', 'runs'],
    queryFn: ({ signal }) => api.admin.scheduledTasks.getRuns(undefined, signal),
    enabled: hasPermission('settings:read'),
  })

  const triggerUsersMutation = useMutation({
    mutationFn: api.admin.scheduledTasks.triggerUsersExpiration,
    onSuccess: (result) => {
      refetchTaskRuns()
      toast.success(`Account expiration check completed: ${result.usersChecked} users checked, ${result.notificationsSent} notifications sent`)
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const triggerPasswordsMutation = useMutation({
    mutationFn: api.admin.scheduledTasks.triggerPasswordsExpiration,
    onSuccess: (result) => {
      refetchTaskRuns()
      toast.success(`Password expiration check completed: ${result.usersChecked} users checked, ${result.notificationsSent} notifications sent`)
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  if (isLoading) {
    return (
      <InlineSpinner />
    )
  }

  if (error) {
    return (
      <div className="p-4 text-destructive bg-destructive/10 rounded-md">
        Failed to load configuration: {error.message}
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Administration</h1>
        <p className="text-muted-foreground">System configuration (read-only)</p>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              Server
            </CardTitle>
            <CardDescription>HTTP server configuration</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <ConfigField
              label="Host"
              config={config!.server.host}
              description="Bind address for the HTTP server"
            />
            <ConfigField
              label="Port"
              config={config!.server.port}
              description="Port number for the HTTP server"
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Database className="h-5 w-5" />
              Database
            </CardTitle>
            <CardDescription>PostgreSQL connection settings</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <ConfigField
              label="Connection URL"
              config={config!.database.url}
              description="PostgreSQL connection string (password masked)"
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Database className="h-5 w-5" />
              Redis
            </CardTitle>
            <CardDescription>Redis connection settings</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <ConfigField
              label="Connection URL"
              config={config!.redis.url}
              description="Redis connection string"
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Key className="h-5 w-5" />
              LDAP
            </CardTitle>
            <CardDescription>LDAP directory connection settings</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <ConfigField
              label="Server URL"
              config={config!.ldap.url}
              description="LDAP server URL"
            />
            <ConfigField
              label="Base DN"
              config={config!.ldap.baseDn}
              description="Base Distinguished Name"
            />
            <ConfigField
              label="Bind DN"
              config={config!.ldap.bindDn}
              description="DN used for binding to LDAP"
            />
            <div className="grid grid-cols-2 gap-4">
              <ConfigField
                label="Users OU"
                config={config!.ldap.userOu}
                description="Organizational Unit for users"
              />
              <ConfigField
                label="Groups OU"
                config={config!.ldap.groupOu}
                description="Organizational Unit for groups"
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <ConfigField
                label="Sudoers OU"
                config={config!.ldap.sudoersOu}
                description="Organizational Unit for sudo roles"
              />
              <ConfigField
                label="Password Policies OU"
                config={config!.ldap.ppolicyOu}
                description="Organizational Unit for password policies"
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <ConfigField
                label="Min UID"
                config={config!.ldap.minUid}
                description="Minimum UID for new users"
              />
              <ConfigField
                label="Min GID"
                config={config!.ldap.minGid}
                description="Minimum GID for new groups"
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <ConfigField
                label="TLS Mode"
                config={config!.ldap.tlsMode}
                description="none, ssl, or starttls"
              />
              <ConfigField
                label="TLS Skip Verify"
                config={config!.ldap.tlsSkipVerify}
                description="Skip certificate verification"
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Clock className="h-5 w-5" />
              Session
            </CardTitle>
            <CardDescription>Session management settings</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <ConfigField
              label="Session TTL"
              config={config!.session.ttl}
              description="Session time-to-live duration"
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Building className="h-5 w-5" />
              Application
            </CardTitle>
            <CardDescription>Application-specific settings</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <ConfigField
              label="Admin Group"
              config={config!.app.adminGroup}
              description="LDAP group that grants admin privileges"
            />
            <ConfigField
              label="Organization"
              config={config!.app.organization}
              description="Organization name for emails"
            />
            <ConfigField
              label="Public URL"
              config={config!.app.publicUrl}
              description="Public URL for password reset links"
            />
            <ConfigField
              label="Modules"
              config={config!.app.modules}
              description="Enabled high-level modules (tabs)"
            />
            <ConfigField
              label="Users Objects"
              config={config!.app.usersObjects}
              description="LDAP objectClasses for users"
            />
            <ConfigField
              label="Groups Objects"
              config={config!.app.groupsObjects}
              description="LDAP objectClasses for groups"
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Mail className="h-5 w-5" />
              Mail
            </CardTitle>
            <CardDescription>SMTP server configuration</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-3 gap-4">
              <ConfigField
                label="Host"
                config={config!.mail.host}
                description="SMTP server hostname"
              />
              <ConfigField
                label="Port"
                config={config!.mail.port}
                description="SMTP server port"
              />
              <ConfigField
                label="SSL Mode"
                config={config!.mail.ssl}
                description="none, starttls, or ssl"
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <ConfigField
                label="User"
                config={config!.mail.user}
                description="SMTP username"
              />
              <ConfigField
                label="Password"
                config={config!.mail.password}
                description="SMTP password (masked)"
              />
            </div>
            <ConfigField
              label="From Address"
              config={config!.mail.from}
              description="Sender email address"
            />
          </CardContent>
        </Card>

        <Card className="md:col-span-2">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <CalendarClock className="h-5 w-5" />
              Scheduled Tasks
            </CardTitle>
            <CardDescription>Expiration notification tasks</CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="grid gap-4 md:grid-cols-2">
              {/* Users Expiration Task */}
              <div className="border rounded-lg p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Users className="h-4 w-4 text-muted-foreground" />
                    <span className="font-medium">Account Expiration</span>
                  </div>
                  <Badge variant={scheduledTasksConfig?.usersExpiration.value ? 'default' : 'secondary'}>
                    {scheduledTasksConfig?.usersExpiration.value || 'Disabled'}
                  </Badge>
                </div>
                <p className="text-sm text-muted-foreground">
                  Notifies admins when user accounts are about to expire
                </p>
                <Button
                  size="sm"
                  onClick={() => triggerUsersMutation.mutate()}
                  disabled={triggerUsersMutation.isPending || !scheduledTasksConfig?.usersExpiration.value}
                >
                  {triggerUsersMutation.isPending ? (
                    <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  ) : (
                    <Play className="h-4 w-4 mr-2" />
                  )}
                  Run Now
                </Button>
              </div>

              {/* Passwords Expiration Task */}
              <div className="border rounded-lg p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <KeyRound className="h-4 w-4 text-muted-foreground" />
                    <span className="font-medium">Password Expiration</span>
                  </div>
                  <Badge variant={scheduledTasksConfig?.passwordsExpiration.value ? 'default' : 'secondary'}>
                    {scheduledTasksConfig?.passwordsExpiration.value || 'Disabled'}
                  </Badge>
                </div>
                <p className="text-sm text-muted-foreground">
                  Notifies users when their passwords are about to expire
                </p>
                <Button
                  size="sm"
                  onClick={() => triggerPasswordsMutation.mutate()}
                  disabled={triggerPasswordsMutation.isPending || !scheduledTasksConfig?.passwordsExpiration.value}
                >
                  {triggerPasswordsMutation.isPending ? (
                    <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  ) : (
                    <Play className="h-4 w-4 mr-2" />
                  )}
                  Run Now
                </Button>
              </div>
            </div>

            {/* Recent Task Runs */}
            <div>
              <h4 className="text-sm font-medium mb-3 flex items-center gap-2">
                <History className="h-4 w-4" />
                Recent Runs
              </h4>
              <div className="space-y-2">
                {taskRuns?.data.slice(0, 5).map((run) => (
                  <div key={run.id} className="flex items-center justify-between text-sm border rounded p-2">
                    <div className="flex items-center gap-2">
                      {run.status === 'completed' ? (
                        <CheckCircle className="h-4 w-4 text-green-500" />
                      ) : run.status === 'failed' ? (
                        <XCircle className="h-4 w-4 text-red-500" />
                      ) : (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      )}
                      <span>{run.taskName === 'users_expiration' ? 'Account' : 'Password'} Expiration</span>
                      <Badge variant="outline" className="text-xs">{run.triggeredBy}</Badge>
                    </div>
                    <div className="flex items-center gap-4 text-muted-foreground">
                      <span>{run.usersChecked} checked</span>
                      <span>{run.notificationsSent} sent</span>
                      <span>{new Date(run.startedAt).toLocaleString()}</span>
                    </div>
                  </div>
                ))}
                {(!taskRuns || taskRuns.data.length === 0) && (
                  <p className="text-sm text-muted-foreground text-center py-4">No task runs yet</p>
                )}
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
