import { useState } from 'react'
import { Plus, X, AlertTriangle } from 'lucide-react'
import type { NetgroupTriple } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { describeTriple, formatTriple, NETGROUP_NO_MATCH } from '@/lib/netgroup'

/**
 * Edits the triples of a netgroup.
 *
 * The whole design turns on one thing: a slot left empty means *any*, not none.
 * A blank input would read as "nothing here", so every empty slot is rendered as
 * the sentence it stands for — "jdoe on any host" — and the form says what
 * leaving a field blank will do before it is submitted.
 */
export function TripleEditor({
  triples,
  onChange,
  canWrite,
}: {
  triples: NetgroupTriple[]
  onChange: (triples: NetgroupTriple[]) => void
  canWrite: boolean
}) {
  const [user, setUser] = useState('')
  const [host, setHost] = useState('')

  const add = () => {
    const next: NetgroupTriple = {
      user: user.trim(),
      host: host.trim(),
      domain: '',
    }
    // Every slot open grants everything to everyone; the API refuses it too.
    if (!next.user && !next.host) return
    if (triples.some((t) => t.user === next.user && t.host === next.host && t.domain === next.domain)) {
      return
    }
    onChange([...triples, next])
    setUser('')
    setHost('')
  }

  const remove = (index: number) => {
    onChange(triples.filter((_, i) => i !== index))
  }

  return (
    <div className="space-y-3">
      {triples.length === 0 ? (
        <p className="text-sm text-muted-foreground">No members yet</p>
      ) : (
        <ul className="space-y-1">
          {triples.map((triple, index) => (
            <li
              key={`${formatTriple(triple)}-${index}`}
              className="flex items-center justify-between gap-2 rounded-md bg-muted px-2 py-1.5"
            >
              <span className="min-w-0">
                <span className="text-sm">{describeTriple(triple)}</span>
                <span className="ml-2 font-mono text-xs text-muted-foreground">
                  {formatTriple(triple)}
                </span>
              </span>
              {canWrite && (
                <button
                  type="button"
                  onClick={() => remove(index)}
                  aria-label={`Remove ${formatTriple(triple)}`}
                  className="shrink-0 text-muted-foreground hover:text-destructive"
                >
                  <X className="h-4 w-4" aria-hidden="true" />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}

      {canWrite && (
        <div className="space-y-2 rounded-md border p-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="triple-user" className="text-xs">
                User
              </Label>
              <Input
                id="triple-user"
                value={user}
                onChange={(e) => setUser(e.target.value)}
                placeholder="Leave empty for any user"
                onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), add())}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="triple-host" className="text-xs">
                Host
              </Label>
              <Input
                id="triple-host"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                placeholder="Leave empty for any host"
                onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), add())}
              />
            </div>
          </div>

          <p className="text-xs text-muted-foreground">
            An empty field means <strong>any</strong>, not none — “{describeTriple({
              user: user.trim(),
              host: host.trim(),
              domain: '',
            })}”. Use <code className="font-mono">{NETGROUP_NO_MATCH}</code> to match nothing.
          </p>

          {!user.trim() && !host.trim() && (
            <p className="flex items-start gap-1.5 text-xs text-warning">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
              Fill in at least one field: leaving both empty would match every user on every host.
            </p>
          )}

          <div className="flex justify-end">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={add}
              disabled={!user.trim() && !host.trim()}
            >
              <Plus className="h-4 w-4 mr-1" />
              Add member
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
