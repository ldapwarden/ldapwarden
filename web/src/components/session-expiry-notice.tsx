import { useEffect, useState } from 'react'
import { Callout } from '@/components/ui/callout'
import { useAuth } from '@/lib/auth'

/** How long before the deadline to start warning. */
const WARN_WITHIN_MS = 5 * 60 * 1000
const TICK_MS = 30 * 1000

/**
 * Warns before the session runs out.
 *
 * The TTL is absolute: the backend stamps `expiresAt` at login and never extends
 * it, so no amount of activity keeps a session alive. Without a warning, ten
 * minutes of work in a create form ends in a redirect to the login screen with
 * the form gone — and there is no way back to it. This cannot offer to extend
 * the session (there is no endpoint that would, by design), so it does the one
 * useful thing instead: says when the deadline is, early enough to save first.
 */
export function SessionExpiryNotice() {
  const { session } = useAuth()
  const [now, setNow] = useState(() => Date.now())

  const expiresAt = session ? new Date(session.expiresAt).getTime() : null
  const remaining = expiresAt === null ? null : expiresAt - now
  const warning = remaining !== null && remaining <= WARN_WITHIN_MS

  useEffect(() => {
    if (expiresAt === null) return
    // Only tick once inside the warning window; before that, one check per
    // render of a mounted app is plenty.
    const id = setInterval(() => setNow(Date.now()), TICK_MS)
    return () => clearInterval(id)
  }, [expiresAt])

  if (!warning || remaining === null) return null

  const minutes = Math.max(0, Math.round(remaining / 60000))

  return (
    <Callout variant="warning" className="mb-4">
      {remaining <= 0 ? (
        <>Your session has ended. Save nothing further here — the next action will send you to the sign-in page.</>
      ) : (
        <>
          Your session ends at{' '}
          <strong>
            {new Date(expiresAt!).toLocaleTimeString(undefined, {
              hour: '2-digit',
              minute: '2-digit',
            })}
          </strong>{' '}
          ({minutes === 0 ? 'less than a minute' : `about ${minutes} minute${minutes === 1 ? '' : 's'}`}{' '}
          from now). Save anything in progress: signing in again starts a fresh session and cannot
          restore an unsaved form.
        </>
      )}
    </Callout>
  )
}
