import { createFileRoute, Link, redirect } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { api, type AuditLog, type TaskHealth } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Callout } from '@/components/ui/callout'
import { Skeleton } from '@/components/ui/skeleton'
import { encodeDN } from '@/lib/utils'
import { resourceLink } from '@/lib/audit-links'
import { useServerInfo } from '@/lib/use-server-info'
import { actionTone, toneStyle, type Tone } from '@/lib/tones'
import { taskCheckedLabel, taskLabel, taskNotifiedLabel } from '@/lib/scheduled-tasks'
import {
  AlertTriangle,
  CalendarClock,
  CheckCircle2,
  CircleDashed,
  KeyRound,
  Lock,
  Play,
  Plus,
  ScrollText,
  ShieldCheck,
  Upload,
  Users,
  UsersRound,
  XCircle,
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

  const { data: taskHealth } = useQuery({
    queryKey: ['admin', 'scheduled-tasks', 'health'],
    queryFn: ({ signal }) => api.admin.scheduledTasks.getHealth(signal),
    enabled: canReadSettings,
  })

  const serverInfo = useServerInfo()
  const enforcement = serverInfo?.capabilities?.policyEnforcement ?? 'unknown'
  const isAdmin = session?.roleName === 'admin'

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

      {/* The locked and expired figures below describe attributes, not what the
          directory does with them: ppolicy honours neither unless a policy applies,
          and that is not detectable from an ordinary bind. Said here, next to the
          numbers, rather than only on the admin page. */}
      {serverInfo && enforcement === 'not-enforced' && (
        <Callout variant="error">
          <p className="font-medium">The directory does not enforce locks or expiry.</p>
          <p className="mt-1">
            The policy self-test showed a locked account authenticating anyway, so accounts
            counted as locked or expired here can still log in.
            {isAdmin && (
              <>
                {' '}
                <Link to="/admin" hash="directory-enforcement" className="font-medium underline">
                  See directory enforcement
                </Link>
              </>
            )}
          </p>
        </Callout>
      )}
      {/* Unknown is the default, and only an admin can settle it — so they are
          always told. Anyone else only when there are locked or expired accounts
          whose figure it makes uncertain. */}
      {serverInfo && enforcement === 'unknown' && (isAdmin || cannotLogIn > 0) && (
        <Callout variant="warning">
          <p className="font-medium">Whether locks and expiry are enforced is not known yet.</p>
          <p className="mt-1">
            Nothing readable from this bind tells a directory that honours them from one that
            ignores them, so accounts counted as locked or expired may still be able to log in.
            {isAdmin ? (
              <>
                {' '}
                <Link to="/admin" hash="directory-enforcement" className="font-medium underline">
                  Run the policy self-test
                </Link>{' '}
                to find out.
              </>
            ) : (
              ' An administrator can settle it with the policy self-test.'
            )}
          </p>
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
                        <ActivityTarget log={log} />
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
            {!taskHealth ? (
              <ListSkeleton rows={2} />
            ) : taskHealth.data.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                Every scheduled task is disabled: no expiration notice is sent and no lockout is
                recorded.
              </p>
            ) : (
              <>
                {/* One row per configured task, not the latest runs: a task that
                    stopped running would simply drop out of a list of runs. */}
                <ul className="divide-y">
                  {taskHealth.data.map((task) => (
                    <TaskHealthRow key={task.taskName} task={task} />
                  ))}
                </ul>
                <Link to="/admin" className="mt-3 inline-block">
                  <Button variant="outline" size="sm">
                    <Play className="h-4 w-4 mr-1" />
                    Run history and manual runs
                  </Button>
                </Link>
              </>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}

// The health of one task, in words: an icon alone said "failed" with a padlock
// and nothing at all for a task that had stopped running.
function TaskHealthRow({ task }: { task: TaskHealth }) {
  const run = task.lastRun
  const state = task.overdue
    ? {
        icon: AlertTriangle,
        tone: 'text-warning',
        label: task.expectedAt ? `Missed its run ${relativeTime(task.expectedAt)}` : 'Overdue',
      }
    : run?.status === 'failed'
      ? { icon: XCircle, tone: 'text-destructive', label: 'Last run failed' }
      : !run
        ? { icon: CircleDashed, tone: 'text-muted-foreground', label: 'Not run yet' }
        : { icon: CheckCircle2, tone: 'text-success', label: 'OK' }
  const Icon = state.icon

  return (
    <li className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm first:pt-0 last:pb-0">
      <span className="flex min-w-0 items-center gap-2">
        <Icon className={`h-4 w-4 shrink-0 ${state.tone}`} aria-hidden="true" />
        <span className="font-medium">{taskLabel(task.taskName)}</span>
        <span className={state.tone}>{state.label}</span>
      </span>
      <span className="flex flex-wrap items-center gap-x-4 text-muted-foreground">
        {run && (
          <>
            <span>{taskCheckedLabel(task.taskName, run.usersChecked)}</span>
            <span>{taskNotifiedLabel(run.notificationsSent)}</span>
            <span title={new Date(run.startedAt).toLocaleString()}>last {relativeTime(run.startedAt)}</span>
          </>
        )}
        <span title={task.schedule}>next {relativeTime(task.nextRun)}</span>
      </span>
    </li>
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


// ActivityTarget reads as "<actor> → <resource>": who did it and to what. The
// resource links to its page, as in the audit log, unless the entry records its
// deletion.
function ActivityTarget({ log }: { log: AuditLog }) {
  const name = log.details?.resourceName
  const target = typeof name === 'string' && name !== '' ? name : log.resourceDn
  const link = resourceLink(log)
  return (
    <span className="min-w-0 flex-1 truncate">
      {log.actorUid}
      {target && (
        <>
          {' → '}
          {link ? (
            <Link to={link.to} params={{ dn: link.dn }} className="hover:underline">
              {target}
            </Link>
          ) : (
            target
          )}
        </>
      )}
    </span>
  )
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
