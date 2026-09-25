import { useState } from 'react'
import { Plus, X, AlertTriangle } from 'lucide-react'
import { api, type NetgroupTriple } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { usePickerSearch } from '@/lib/use-picker-search'
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
  const [host, setHost] = useState('')

  // The user slot searches the directory, so a triple cannot quietly name an
  // account that does not exist. The host slot stays free text: there is no
  // host inventory to search, and inventing one is a separate piece of work.
  // Nothing forces a match either — a netgroup may legitimately name a user
  // this directory does not hold.
  const userPicker = usePickerSearch(['users', 'picker', 'netgroup-triple'], api.users.list, {
    sort: 'uid',
  })
  const user = userPicker.input

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
    userPicker.reset()
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
                  className="-m-1 shrink-0 rounded p-1 text-muted-foreground hover:text-destructive"
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
                onChange={(e) => userPicker.setInput(e.target.value)}
                placeholder="Leave empty for any user"
                onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), add())}
                autoComplete="off"
              />
              {user.trim() !== '' && user !== NETGROUP_NO_MATCH && (
                <UserSuggestions
                  matches={userPicker.items}
                  typed={user}
                  onPick={(uid) => userPicker.setInput(uid)}
                />
              )}
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

/**
 * Directory matches for what is being typed in the user slot.
 *
 * Suggestions, not a constraint: a netgroup can legitimately name a user this
 * directory does not hold — a local account on the hosts, or one from another
 * tree. So an unmatched value is flagged rather than refused, which is the same
 * stance the rest of the app takes towards references it cannot resolve.
 */
function UserSuggestions({
  matches,
  typed,
  onPick,
}: {
  matches: Array<{ dn: string; uid: string; cn: string; displayName?: string }>
  typed: string
  onPick: (uid: string) => void
}) {
  const exact = matches.some((u) => u.uid === typed)
  const shown = matches.filter((u) => u.uid !== typed).slice(0, 5)

  if (exact) return null

  return (
    <div className="space-y-1 pt-1">
      {shown.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {shown.map((match) => (
            <button
              key={match.dn}
              type="button"
              onClick={() => onPick(match.uid)}
              className="rounded-md bg-muted px-2 py-0.5 text-xs hover:bg-accent"
            >
              {match.uid}
              <span className="ml-1 text-muted-foreground">
                {match.displayName || match.cn}
              </span>
            </button>
          ))}
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">
          No user in this directory is called “{typed}”. That is allowed — the netgroup can
          name an account held elsewhere — but check the spelling.
        </p>
      )}
    </div>
  )
}
