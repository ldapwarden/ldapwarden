import { useServerInfo } from '@/lib/use-server-info'

/**
 * Which build is answering.
 *
 * The first question on any bug report, and the one hardest to get a straight
 * answer to from a running deployment. It comes from the server rather than
 * from a constant compiled into this bundle, so a browser holding a stale
 * bundle still reports what it is actually talking to — the value of the answer
 * depends entirely on it being the running one.
 */
export function BuildFooter() {
  const info = useServerInfo()
  if (!info) return null

  const { version, revision, modified } = info.version

  return (
    <footer className="container mx-auto px-4 pb-6 text-xs text-muted-foreground">
      <p>
        LDAP Warden <span className="tabular-nums">{version}</span>
        {revision && (
          <>
            {' · '}
            <span className="font-mono">{revision}</span>
          </>
        )}
        {modified && (
          // Not the commit it names. Saying only the revision would describe
          // code that was never built.
          <>
            {' '}
            <span className="text-warning">(modified)</span>
          </>
        )}
      </p>
    </footer>
  )
}
