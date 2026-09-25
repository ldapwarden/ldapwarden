import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { api, type AuditLog } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Callout } from '@/components/ui/callout'
import { Skeleton } from '@/components/ui/skeleton'
import { encodeDN } from '@/lib/utils'
import { actionTone, toneStyle, type Tone } from '@/lib/tones'
import { taskCheckedLabel, taskLabel, taskNotifiedLabel } from '@/lib/scheduled-tasks'
import {
  CalendarClock,
  CheckCircle2,
  KeyRound,
  Lock,
  Play,
  Plus,
  ScrollText,
  ShieldCheck,
  Upload,
  Users,
  UsersRound,
} from 'lucide-react'

export const Route = createFileRoute('/')({
  beforeLoad: ({ context }) => {
    if (!context.auth.isAuthenticated) {
      throw redirect({ to: '/login' })
    }
  },
  component: DashboardPage,
})

/**
 * The landing page.
 *
 * It used to redirect straight to /users, which meant the answers an operator
 * opens the tool for — how many accounts are locked, what expires this month,
 * what changed recently, did last night's expiration job run — each took a
 * separate hunt. Everything here is data the app already had.
 */
function DashboardPage() {
  const { session, hasPermission } = useAuth()

  const { data: overview, isLoading, error } = useQuery({
    queryKey: ['overview'],
    queryFn: ({ signal }) => api.overview.get(signal),
  })

  // Both panels below are permission-gated: a read-only operator has neither.
  const canReadAudit = hasPermission('audit:read')
  const canReadSettings = hasPermission('settings:read')
  const canWrite = hasPermission('users:write')

  const { data: recentAudit } = useQuery({
    queryKey: ['audit-logs', 'recent'],
    queryFn: ({ signal }) => api.auditLogs.list({ limit: 6 }, signal),
    enabled: canReadAudit,
  })

  const { data: taskRuns } = useQuery({
    queryKey: ['admin', 'scheduled-tasks', 'runs'],
    queryFn: ({ signal }) => api.admin.scheduledTasks.getRuns(undefined, signal),
    enabled: canReadSettings,
  })

  if (error) {
    return <Callout variant="error">Failed to load the dashboard: {error.message}</Callout>
  }

  const users = overview?.users
  // total - active counts each account that cannot log in once, however many
  // reasons it has; locked + expired would charge a locked-and-expired account
  // twice and overstate the headline.
  const cannotLogIn = users ? users.total - users.active : 0
  const needsAttention = cannotLogIn + (users?.expiring.length ?? 0)

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-2xl font-bold">
            {session?.displayName ? `Welcome back, ${session.displayName.split(' ')[0]}` : 'Overview'}
          </h1>
          <p className="text-muted-foreground">
            {needsAttention > 0
              ? needsAttention === 1
                ? '1 account needs attention'
                : `${needsAttention} accounts need attention`
              : 'Nothing needs attention right now'}
          </p>
        </div>
        {canWrite && (
          <div className="flex flex-wrap items-center gap-2">
            <Link to="/users/import">
              <Button variant="outline">
                <Upload className="h-4 w-4 mr-1" />
                Import CSV
              </Button>
            </Link>
            <Link to="/users/new">
              <Button>
                <Plus className="h-4 w-4 mr-1" />
                New User
              </Button>
            </Link>
          </div>
        )}
      </div>

      {/* Only shown when the *directory* stopped a count early — a size limit on
          the bind identity. It used to fire on the app's own LDAP_SEARCH_SIZE_LIMIT,
          which meant every directory over 500 entries carried a permanent warning
          about a knob the operator had no reason to touch. */}
      {overview?.truncated && (
        <Callout variant="warning">
          The directory stopped short of counting every entry, so these figures are lower bounds.
          Raise the search size limit for the account LDAP Warden binds with
          (<code>olcSizeLimit</code> on OpenLDAP) to see exact counts.
        </Callout>
      )}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          label="Users"
          value={users?.total}
          loading={isLoading}
          icon={Users}
          tone="blue"
          to="/users"
          hint={users ? `${users.active} active` : undefined}
        />
        {/* total - active, not locked + expired: an account can be both, and the
            sum would count it twice. The breakdown below can therefore add up to
            more than the headline, which is the honest way round. */}
        <StatCard
          label="Locked or expired"
          value={users ? cannotLogIn : undefined}
          loading={isLoading}
          icon={Lock}
          tone={cannotLogIn > 0 ? 'rose' : 'slate'}
          to="/users"
          search={{ status: 'inactive' }}
          hint={users ? `${users.locked} locked, ${users.expired} expired` : undefined}
        />
        <StatCard
          label="Groups"
          value={overview?.groups}
          loading={isLoading}
          icon={UsersRound}
          tone="emerald"
          to="/groups"
        />
        <StatCard
          label="Sudo roles"
          value={overview?.sudoRoles}
          loading={isLoading}
          icon={ShieldCheck}
          tone="violet"
          to="/sudo-roles"
          hint={canReadSettings ? `${overview?.policies ?? 0} password policies` : undefined}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <CalendarClock className="h-5 w-5" />
              Expiring within 30 days
            </CardTitle>
            <CardDescription>
              Accounts that will stop working soon — the same ones the expiration job emails about
            </CardDescription>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <ListSkeleton rows={3} />
            ) : users && users.expiring.length > 0 ? (
              <ul className="divide-y">
                {users.expiring.map((account) => {
                  const days = daysUntil(account.expiresAt)
                  return (
                    <li key={account.dn} className="flex items-center justify-between gap-2 py-2 first:pt-0 last:pb-0">
                      <Link
                        to="/users/$dn"
                        params={{ dn: encodeDN(account.dn) }}
                        search={{ tab: 'security' as const }}
                        className="min-w-0 hover:underline"
                      >
                        <span className="font-medium">{account.displayName || account.uid}</span>
                        <span className="ml-2 text-sm text-muted-foreground">{account.uid}</span>
                      </Link>
                      <span
                        className={`shrink-0 text-sm ${days <= 7 ? 'text-destructive' : 'text-warning'}`}
                      >
                        {days <= 0 ? 'today' : `in ${days} day${days === 1 ? '' : 's'}`}
                      </span>
                    </li>
                  )
                })}
              </ul>
            ) : (
              <p className="flex items-center gap-2 py-2 text-sm text-muted-foreground">
                <CheckCircle2 className="h-4 w-4 text-success" aria-hidden="true" />
                No account expires in the next 30 days.
              </p>
            )}
          </CardContent>
        </Card>

        {canReadAudit && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <ScrollText className="h-5 w-5" />
                Recent activity
              </CardTitle>
              <CardDescription>The last changes recorded in the audit log</CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {!recentAudit ? (
                <ListSkeleton rows={4} />
              ) : recentAudit.data.length === 0 ? (
                <p className="py-2 text-sm text-muted-foreground">Nothing recorded yet.</p>
              ) : (
                <>
                  <ul className="divide-y">
                    {recentAudit.data.map((log) => (
                      <li key={log.id} className="flex items-baseline gap-2 py-2 text-sm first:pt-0">
                        <span
                          className="tone shrink-0 rounded-full border px-2 py-0.5 text-xs font-medium"
                          style={toneStyle(actionTone(log.action))}
                        >
                          {log.action}
                        </span>
                        <span className="min-w-0 flex-1 truncate">{describeTarget(log)}</span>
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {relativeTime(log.createdAt)}
                        </span>
                      </li>
                    ))}
                  </ul>
                  <Link to="/audit-logs">
                    <Button variant="outline" size="sm">
                      View the audit log
                    </Button>
                  </Link>
                </>
              )}
            </CardContent>
          </Card>
        )}
      </div>

      {canReadSettings && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <KeyRound className="h-5 w-5" />
              Scheduled tasks
            </CardTitle>
            <CardDescription>
              Expiration notifications and lockout detection — a task that stops running is a
              silent failure
            </CardDescription>
          </CardHeader>
          <CardContent>
            {!taskRuns ? (
              <ListSkeleton rows={2} />
            ) : taskRuns.data.length === 0 ? (
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-sm text-muted-foreground">
                  No run recorded yet. The tasks run on their cron schedule, or on demand.
                </p>
                <Link to="/admin">
                  <Button variant="outline" size="sm">
                    <Play className="h-4 w-4 mr-1" />
                    Administration
                  </Button>
                </Link>
              </div>
            ) : (
              <ul className="divide-y">
                {taskRuns.data.slice(0, 3).map((run) => (
                  <li key={run.id} className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm first:pt-0 last:pb-0">
                    <span className="flex items-center gap-2">
                      {run.status === 'failed' ? (
                        <Lock className="h-4 w-4 text-destructive" aria-hidden="true" />
                      ) : (
                        <CheckCircle2 className="h-4 w-4 text-success" aria-hidden="true" />
                      )}
                      {taskLabel(run.taskName)}
                    </span>
                    <span className="flex flex-wrap items-center gap-x-4 text-muted-foreground">
                      <span>{taskCheckedLabel(run.taskName, run.usersChecked)}</span>
                      <span>{taskNotifiedLabel(run.notificationsSent)}</span>
                      <span>{relativeTime(run.startedAt)}</span>
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}

function StatCard({
  label,
  value,
  hint,
  loading,
  icon: Icon,
  tone,
  to,
  search,
}: {
  label: string
  value: number | undefined
  hint?: string
  loading: boolean
  icon: typeof Users
  tone: Tone
  to: string
  search?: Record<string, unknown>
}) {
  return (
    <Link to={to} search={search} className="block">
      <Card className="h-full transition-colors hover:border-primary/40">
        <CardContent className="flex items-center gap-4 p-6">
          <span className="tone rounded-full border p-3" style={toneStyle(tone)}>
            <Icon className="h-5 w-5" aria-hidden="true" />
          </span>
          <div className="min-w-0">
            {loading ? (
              <Skeleton className="h-7 w-12" />
            ) : (
              <p className="text-2xl font-bold leading-tight">{value ?? '—'}</p>
            )}
            <p className="text-sm font-medium">{label}</p>
            {hint && <p className="truncate text-xs text-muted-foreground">{hint}</p>}
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}

function ListSkeleton({ rows }: { rows: number }) {
  return (
    <div className="space-y-3">
      <span className="sr-only" role="status">Loading…</span>
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-5" style={{ width: `${[85, 65, 75, 55][i % 4]}%` }} />
      ))}
    </div>
  )
}


// describeTarget reads as "<actor> → <resource>": who did it and to what.
function describeTarget(log: AuditLog): string {
  const name = log.details?.resourceName
  const target = typeof name === 'string' && name !== '' ? name : log.resourceDn
  return target ? `${log.actorUid} → ${target}` : log.actorUid
}

function daysUntil(iso: string): number {
  return Math.ceil((new Date(iso).getTime() - Date.now()) / (1000 * 60 * 60 * 24))
}

// relativeTime keeps the activity list scannable: exact timestamps are one click
// away in the audit log.
function relativeTime(iso: string): string {
  const seconds = Math.round((Date.now() - new Date(iso).getTime()) / 1000)
  const units: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ['second', 60],
    ['minute', 60],
    ['hour', 24],
    ['day', 30],
    ['month', 12],
    ['year', Number.POSITIVE_INFINITY],
  ]
  let value = seconds
  for (const [unit, size] of units) {
    if (Math.abs(value) < size) {
      return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' }).format(-Math.round(value), unit)
    }
    value /= size
  }
  return new Date(iso).toLocaleDateString()
}
