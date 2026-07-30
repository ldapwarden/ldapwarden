import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function encodeDN(dn: string): string {
  return btoa(dn).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function decodeDN(encoded: string): string {
  let base64 = encoded.replace(/-/g, '+').replace(/_/g, '/')
  while (base64.length % 4) {
    base64 += '='
  }
  return atob(base64)
}

// Helper function to parse LDAP generalized time to Date object
export function parseLdapTimestamp(timestamp: string): Date | null {
  if (!timestamp) return null
  const match = timestamp.match(/^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})/)
  if (!match) return null
  const [, year, month, day, hour, minute, second] = match
  return new Date(Date.UTC(
    parseInt(year), parseInt(month) - 1, parseInt(day),
    parseInt(hour), parseInt(minute), parseInt(second)
  ))
}

// Helper function to check if LDAP timestamp is in the future
export function isLdapTimestampInFuture(timestamp: string): boolean {
  const date = parseLdapTimestamp(timestamp)
  if (!date) return false
  return date > new Date()
}

// Helper function to format LDAP generalized time (e.g., "20240115143022Z")
export function formatLdapTimestamp(timestamp: string): string {
  const date = parseLdapTimestamp(timestamp)
  if (!date) return timestamp || 'N/A'
  return date.toLocaleString()
}

// Helper function to convert LDAP timestamp to date string (YYYY-MM-DD) for date picker
export function ldapTimestampToDateString(timestamp: string): string {
  const date = parseLdapTimestamp(timestamp)
  if (!date) return ''
  return date.toISOString().split('T')[0]
}

/**
 * Whether two DNs designate the same entry, for deciding whether the UI is
 * looking at the signed-in account.
 *
 * Case- and whitespace-insensitive, which covers the ways the same DN gets
 * written ("uid=a, ou=People" vs "uid=a,ou=People"). This is a display decision
 * only: the API compares DNs properly (see sameDN in internal/api/dn.go) and
 * refuses the dangerous operations regardless of what the UI chose to show.
 */
export function sameDn(a: string | undefined, b: string | undefined): boolean {
  if (!a || !b) return false
  const normalise = (dn: string) =>
    dn
      .split(',')
      .map((part) => part.trim().replace(/\s*=\s*/, '='))
      .join(',')
      .toLowerCase()
  return normalise(a) === normalise(b)
}

/**
 * An account expiry, rendered as the calendar date it actually falls on.
 *
 * `pwdEndTime` for a date-granularity expiry is stored as the last second of that
 * day in UTC, so rendering it with toLocaleString (which formatLdapTimestamp does,
 * correctly, for real instants like pwdChangedTime) shifts it to the next day
 * anywhere east of UTC: pick 20 August in the date field, save, and the page said
 * 21 August while the field still said the 20th. The date is a date — format it in
 * the zone it was written in and the two agree.
 *
 * A value that is not end-of-day came from somewhere other than this UI, so the
 * time is shown, labelled, rather than rounded away.
 */
export function formatExpiryDate(timestamp: string): string {
  const date = parseLdapTimestamp(timestamp)
  if (!date) return timestamp || 'N/A'

  const day = new Intl.DateTimeFormat(undefined, { dateStyle: 'long', timeZone: 'UTC' }).format(date)
  const endOfDay =
    date.getUTCHours() === 23 && date.getUTCMinutes() === 59 && date.getUTCSeconds() === 59
  if (endOfDay) return day

  const time = new Intl.DateTimeFormat(undefined, { timeStyle: 'short', timeZone: 'UTC' }).format(date)
  return `${day}, ${time} UTC`
}

/**
 * The exact instant an expiry takes effect, for a tooltip: the rendered date hides
 * that "the 20th" ends at 23:59:59 UTC, which is the 21st in local time for anyone
 * east of Greenwich. Available on hover rather than in the sentence, because the
 * date is what the operator chose and the instant is the footnote.
 */
export function expiryInstantTitle(timestamp: string): string {
  const date = parseLdapTimestamp(timestamp)
  if (!date) return ''
  return `${timestamp} — ${date.toLocaleString()} in your local time`
}

/**
 * The value of a DN's first component — "jdoe" out of
 * "uid=jdoe,ou=People,dc=example,dc=org".
 *
 * A label of last resort, for showing a referenced entry the current view has
 * not fetched (a manager picked before the picker's search results changed). It
 * is the uid in practice, so it reads as a name rather than as a DN. Falls back
 * to the whole string if there is no `=`.
 */
export function rdnValue(dn: string): string {
  const first = dn.split(',')[0]
  const eq = first.indexOf('=')
  return eq === -1 ? dn : first.slice(eq + 1).trim()
}
