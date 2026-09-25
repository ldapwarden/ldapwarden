import { DetailSkeleton } from '@/components/ui/skeleton'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, ConfigValue } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Callout } from '@/components/ui/callout'
import { Server, Database, Key, Clock, Mail, Building, Play, History, Users, KeyRound, CheckCircle, XCircle, Loader2, CalendarClock, ShieldCheck, ShieldAlert, ShieldQuestion, Lock } from 'lucide-react'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { useState, useId } from 'react'
import type { PolicySelfTest, TriggerTaskResponse } from '@/lib/api'
import { toast } from 'sonner'
import { showOutcome } from '@/lib/outcome'
import { taskCheckedLabel, taskLabel, taskNotifiedLabel } from '@/lib/scheduled-tasks'

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
  const id = useId()
  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Label htmlFor={id} className="text-sm font-medium">{label}</Label>
        <span className={`text-xs px-2 py-0.5 rounded-full ${
          config.source === 'env'
            ? 'bg-success/15 text-success'
            : 'bg-muted text-muted-foreground'
        }`}>
          {config.source === 'env' ? 'Environment' : 'Default'}
        </span>
      </div>
      {/* Read-only rather than disabled: a disabled field cannot be focused,
          selected or copied, and a config value is mostly there to be copied. */}
      <Input
        id={id}
        value={displayValue}
        readOnly
        className="font-mono text-sm bg-muted"
      />
      <div className="flex flex-wrap items-center justify-between gap-x-2 text-xs text-muted-foreground">
        {description && <span>{description}</span>}
        {config.envVar && <code className="break-all bg-muted px-1 py-0.5 rounded">{config.envVar}</code>}
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

  // All three toasts read their wording from the same table as the run list, so
  // a task can never be described one way when triggered and another when
  // listed. Each counts something different: "users checked" would misdescribe
  // the lockout scan, which only reads the accounts already locked.
  const reportRun = (taskName: string) => (result: TriggerTaskResponse) => {
    refetchTaskRuns()
    toast.success(
      `${taskLabel(taskName)} completed: ${taskCheckedLabel(taskName, result.usersChecked)}, ` +
        taskNotifiedLabel(result.notificationsSent),
    )
    showOutcome({
      title: `${taskLabel(taskName)} completed with errors`,
      description: 'The run finished, but these steps failed. The accounts they concern were not handled.',
      items: result.errors ?? [],
      tone: 'error',
    })
  }

  const triggerUsersMutation = useMutation({
    mutationFn: api.admin.scheduledTasks.triggerUsersExpiration,
    onSuccess: reportRun('users_expiration'),
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const triggerPasswordsMutation = useMutation({
    mutationFn: api.admin.scheduledTasks.triggerPasswordsExpiration,
    onSuccess: reportRun('passwords_expiration'),
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  const triggerLockoutScanMutation = useMutation({
    mutationFn: api.admin.scheduledTasks.triggerLockoutScan,
    onSuccess: reportRun('lockouts_scan'),
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })

  if (isLoading) {
    return <DetailSkeleton cards={4} />
  }

  if (error) {
    return (
      <Callout variant="error">
        Failed to load configuration: {error.message}
      </Callout>
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
            <div className="grid gap-4 sm:grid-cols-2">
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
            <div className="grid gap-4 sm:grid-cols-2">
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
            <ConfigField
              label="Default Policy DN"
              config={config!.ldap.defaultPolicyDn}
              description="The policy in olcPPolicyDefault, which applies to entries with no pwdPolicySubentry. Not readable from cn=config, so it has to be declared here — without it a ppolicy lockout cannot be told from one that has already expired."
            />
            <div className="grid gap-4 sm:grid-cols-2">
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
            <div className="grid gap-4 sm:grid-cols-2">
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

        <DirectoryEnforcementCard capabilities={config!.capabilities} />

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
            <div className="grid gap-4 sm:grid-cols-3">
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
            <div className="grid gap-4 sm:grid-cols-2">
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
            <CardDescription>Expiration notifications and lockout detection</CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="grid gap-4 md:grid-cols-2">
              {/* Users Expiration Task */}
              <div className="border rounded-lg p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Users className="h-4 w-4 text-muted-foreground" />
                    <span className="font-medium">{taskLabel('users_expiration')}</span>
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
                    <span className="font-medium">{taskLabel('passwords_expiration')}</span>
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

              {/* Lockout Scan */}
              <div className="border rounded-lg p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Lock className="h-4 w-4 text-muted-foreground" />
                    <span className="font-medium">{taskLabel('lockouts_scan')}</span>
                  </div>
                  <Badge variant={scheduledTasksConfig?.lockoutScan.value ? 'default' : 'secondary'}>
                    {scheduledTasksConfig?.lockoutScan.value || 'Disabled'}
                  </Badge>
                </div>
                <p className="text-sm text-muted-foreground">
                  Reports accounts the directory locked by itself after failed sign-ins. Runs
                  frequently on purpose: the directory erases the failed-attempt timestamps at the
                  account's next sign-in.
                </p>
                <Button
                  size="sm"
                  onClick={() => triggerLockoutScanMutation.mutate()}
                  disabled={triggerLockoutScanMutation.isPending || !scheduledTasksConfig?.lockoutScan.value}
                >
                  {triggerLockoutScanMutation.isPending ? (
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
                  <div key={run.id} className="flex flex-wrap items-center justify-between gap-2 text-sm border rounded p-2">
                    <div className="flex items-center gap-2">
                      {run.status === 'completed' ? (
                        <CheckCircle className="h-4 w-4 text-success" />
                      ) : run.status === 'failed' ? (
                        <XCircle className="h-4 w-4 text-destructive" />
                      ) : (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      )}
                      <span>{taskLabel(run.taskName)}</span>
                      <Badge variant="outline" className="text-xs">{run.triggeredBy}</Badge>
                    </div>
                    <div className="flex flex-wrap items-center gap-x-4 text-muted-foreground">
                      <span>{taskCheckedLabel(run.taskName, run.usersChecked)}</span>
                      <span>{taskNotifiedLabel(run.notificationsSent)}</span>
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

// What the app knows about whether the directory enforces a password policy, and
// the one way to find out for certain.
//
// The distinction is not academic: ppolicy enforces nothing unless a policy
// applies to the entry, yet pwdAccountLockedTime and pwdEndTime stay writable, so
// an unenforced directory happily reports accounts as locked or expired while
// letting them authenticate. Nothing readable with an ordinary bind identity
// distinguishes the two states, hence "unknown" and hence the self-test.
function DirectoryEnforcementCard({
  capabilities,
}: {
  capabilities: { scheduledExpirySupported: boolean; policyEnforcement: string } | undefined
}) {
  const queryClient = useQueryClient()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [result, setResult] = useState<PolicySelfTest | null>(null)

  const selfTest = useMutation({
    mutationFn: api.admin.policySelfTest,
    onSuccess: (res) => {
      setResult(res)
      setConfirmOpen(false)
      // The server now knows something it did not before; the state shown above
      // comes from the config query, so it has to be re-read.
      queryClient.invalidateQueries({ queryKey: ['admin', 'config'] })
      if (res.result === 'enforced') {
        toast.success('The directory enforces the password policy')
      } else if (res.result === 'not-enforced') {
        toast.warning('The directory does not enforce any password policy')
      } else {
        toast.warning('The self-test could not reach a conclusion')
      }
      // A probe left behind is reported in the result below, which stays on the
      // page: a toast was gone before anyone had copied the DN to delete.
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const enforcement = capabilities?.policyEnforcement ?? 'unknown'
  const state = {
    enforced: {
      icon: ShieldCheck,
      tone: 'text-success',
      label: 'Enforced',
      blurb: 'A password policy applies. Locking an account and expiring it will be honoured by the directory.',
    },
    'not-enforced': {
      icon: ShieldAlert,
      tone: 'text-destructive',
      label: 'Not enforced',
      blurb:
        'No password policy applies. Locking and account expiry will be written to the directory and ignored — accounts will keep authenticating. Set olcPPolicyDefault on the ppolicy overlay, or a pwdPolicySubentry on each entry.',
    },
    unknown: {
      icon: ShieldQuestion,
      tone: 'text-muted-foreground',
      label: 'Unknown',
      blurb:
        'This bind identity cannot read cn=config, which is normal. Every other signal is identical whether or not a policy applies, so the app will not guess. Run the self-test for a definitive answer.',
    },
  }[enforcement] ?? {
    icon: ShieldQuestion,
    tone: 'text-muted-foreground',
    label: enforcement,
    blurb: '',
  }
  const StateIcon = state.icon

  return (
    // Anchored: the dashboard links here when enforcement is unknown or absent.
    <Card id="directory-enforcement" className="scroll-mt-20">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5" />
          Directory enforcement
        </CardTitle>
        <CardDescription>What the connected directory will actually apply</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-1.5">
          <Label>Scheduled account expiry</Label>
          <div className="flex items-center gap-2 text-sm">
            {capabilities?.scheduledExpirySupported ? (
              <>
                <CheckCircle className="h-4 w-4 text-success" aria-hidden="true" />
                <span>Supported</span>
              </>
            ) : (
              <>
                <XCircle className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
                <span>Unavailable</span>
              </>
            )}
          </div>
          <p className="text-xs text-muted-foreground">
            Whether <code>pwdEndTime</code> exists, i.e. OpenLDAP 2.5 or later with the ppolicy
            overlay loaded. Says nothing about whether it will be enforced.
          </p>
        </div>

        <div className="space-y-1.5">
          <Label>Password policy</Label>
          <div className="flex items-center gap-2 text-sm">
            <StateIcon className={`h-4 w-4 ${state.tone}`} aria-hidden="true" />
            <span>{state.label}</span>
          </div>
          <p className="text-xs text-muted-foreground">{state.blurb}</p>
        </div>

        <div className="pt-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setConfirmOpen(true)}
            disabled={selfTest.isPending}
          >
            {selfTest.isPending ? (
              <Loader2 className="h-4 w-4 mr-1 animate-spin" />
            ) : (
              <Play className="h-4 w-4 mr-1" />
            )}
            {selfTest.isPending ? 'Testing...' : 'Run self-test'}
          </Button>
        </div>

        {result && (
          <Callout
            variant={
              result.result === 'enforced' ? 'success' : result.result === 'not-enforced' ? 'error' : 'warning'
            }
          >
            <p className="font-medium">
              {result.result === 'enforced'
                ? 'Enforced: a locked probe account was refused.'
                : result.result === 'not-enforced'
                  ? 'Not enforced: a locked probe account authenticated anyway.'
                  : 'Inconclusive.'}
            </p>
            {result.detail && <p className="mt-1 text-xs">{result.detail}</p>}
            {/* The steps are the evidence — a verdict about security should be
                checkable, not taken on faith. */}
            <ul className="mt-2 space-y-0.5 text-xs font-mono">
              {result.steps.map((step, i) => (
                <li key={i}>{step}</li>
              ))}
            </ul>
          </Callout>
        )}

        {result && !result.cleanedUp && (
          <Callout variant="error">
            <p className="font-medium">The probe entry could not be removed.</p>
            <p className="mt-1 text-xs">
              It is a locked account with a random password, so it grants nothing, but it is
              yours to delete by hand:
            </p>
            <p className="mt-1 text-xs font-mono break-all">{result.probeDn}</p>
          </Callout>
        )}
      </CardContent>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={(open) => { if (!open) setConfirmOpen(false) }}
        title="Run the policy self-test?"
        description={
          'This is the only way to know for certain, and it writes to the directory: it creates ' +
          'the entry uid=ldapwarden-policy-selftest in the user OU, gives it a random password, ' +
          'locks it, tries to authenticate as it, and removes it. The verdict covers newly created ' +
          'entries in that OU. If an entry with that name already exists, nothing is touched.'
        }
        confirmLabel="Run self-test"
        pendingLabel="Testing..."
        isPending={selfTest.isPending}
        onConfirm={() => selfTest.mutate()}
      />
    </Card>
  )
}
