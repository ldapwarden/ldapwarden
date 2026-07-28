import { CalendarClock, CheckCircle2, Lock } from 'lucide-react'
import { accountExpiration, type AccountStatusFields } from '@/lib/account'

interface Status {
  icon: typeof Lock
  className: string
  label: string
}

function statusOf(user: AccountStatusFields, now: Date): Status {
  const expiresAt = accountExpiration(user)

  // A locked account and a lapsed expiration are the same underlying state (a
  // past pwdAccountLockedTime blocks login), so both read as locked; the label
  // keeps the nuance where the two can be told apart.
  if (user.accountLocked || (expiresAt && expiresAt < now)) {
    return {
      icon: Lock,
      className: 'text-destructive',
      label: user.accountLocked ? 'Account locked' : 'Account expired',
    }
  }

  if (expiresAt) {
    const days = Math.ceil((expiresAt.getTime() - now.getTime()) / (1000 * 60 * 60 * 24))
    return {
      icon: CalendarClock,
      className: days <= 7 ? 'text-destructive' : days <= 30 ? 'text-warning' : 'text-success',
      label: `Expires in ${days} day${days === 1 ? '' : 's'}`,
    }
  }

  return { icon: CheckCircle2, className: 'text-success', label: 'Active' }
}

/**
 * Account status for a list row.
 *
 * The state used to be carried by the icon's colour alone, with the wording
 * only in a `title` attribute — invisible to a screen reader and to anyone who
 * cannot separate red from green. The label is now real text (visually hidden,
 * so the column stays compact) and the glyph is decorative.
 */
export function AccountStatus({ user }: { user: AccountStatusFields }) {
  const now = new Date()
  const { icon: Icon, className, label } = statusOf(user, now)

  return (
    <span className="inline-flex items-center" title={label}>
      <Icon className={`h-4 w-4 ${className}`} aria-hidden="true" />
      <span className="sr-only">{label}</span>
    </span>
  )
}
