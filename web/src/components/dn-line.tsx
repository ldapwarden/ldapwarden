import { Copy } from 'lucide-react'
import { toast } from 'sonner'

/**
 * An entry's DN under its title, with a copy button.
 *
 * The DN is what an LDAP admin pastes into ldapsearch, ldapmodify or a ticket,
 * and a long one is awkward to select by hand; it is also the one thing on the
 * page that reads the same in every other tool, so it stays verbatim.
 */
export function DNLine({ dn }: { dn: string | undefined }) {
  if (!dn) return null

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(dn)
      toast.success('DN copied')
    } catch {
      toast.error('Could not copy: the browser refused clipboard access')
    }
  }

  return (
    <p className="flex items-start gap-1 text-sm text-muted-foreground">
      <span className="break-all font-mono text-xs leading-5">{dn}</span>
      <button
        type="button"
        onClick={copy}
        aria-label="Copy DN"
        title="Copy DN"
        className="shrink-0 rounded p-0.5 hover:text-foreground focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Copy className="h-3.5 w-3.5" aria-hidden="true" />
      </button>
    </p>
  )
}
