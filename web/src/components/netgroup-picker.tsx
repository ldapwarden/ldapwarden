import { useState } from 'react'
import { Network, AlertTriangle, Plus } from 'lucide-react'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Callout } from '@/components/ui/callout'
import { PickerOverflow } from '@/components/ui/picker-overflow'
import { usePickerSearch } from '@/lib/use-picker-search'
import { useNetgroupsEnabled } from '@/lib/use-netgroups'
import { netgroupKind, netgroupSideFit, type NetgroupKind, type NetgroupSide } from '@/lib/netgroup'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'

/**
 * Picks a netgroup to name in a sudo rule, as "+name".
 *
 * It exists because the string is the same on both sides — "+ops" in sudoUser
 * and in sudoHost are indistinguishable — while the meaning is not: sudo reads
 * only the user slots of a netgroup named in Users, and only the host slots of
 * one named in Hosts. A netgroup put on the wrong side matches nothing at all,
 * and neither the directory nor sudo says a word about it. Typing "+ops" by hand
 * into a free-text field gave no chance to catch that; this does, before the
 * click.
 */
export function NetgroupPicker({
  side,
  existing,
  onPick,
  disabled,
}: {
  /** Which sudo attribute this will be added to. */
  side: 'sudoUser' | 'sudoHost'
  /** Values already on the rule, so a netgroup is not offered twice. */
  existing: string[]
  onPick: (value: string) => void
  disabled?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)

  // Only offer the button where netgroups are actually managed: without
  // nis.schema the endpoint answers 501, and with the module off the operator
  // has said this is not where they are managed.
  const supported = useNetgroupsEnabled()

  const picker = usePickerSearch(['netgroups', 'picker', side], api.netgroups.list, {
    sort: 'cn',
    enabled: open && supported,
  })

  if (!supported) return null

  const wanted: NetgroupSide = side === 'sudoUser' ? 'users' : 'hosts'
  const candidates = picker.items.filter((n) => !existing.includes('+' + n.cn))
  const selectedNetgroup = candidates.find((n) => n.cn === selected)
  const selectedKind = selectedNetgroup ? netgroupKind(selectedNetgroup) : null
  const fit = selectedKind === null ? 'ok' : netgroupSideFit(selectedKind, wanted)

  const close = () => {
    setOpen(false)
    setSelected(null)
    picker.reset()
  }

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? setOpen(true) : close())}>
      <DialogTrigger asChild>
        <Button type="button" variant="outline" disabled={disabled}>
          <Network className="h-4 w-4 mr-1" />
          Netgroup
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            Add a netgroup to {side === 'sudoUser' ? 'Users' : 'Hosts'}
          </DialogTitle>
          <DialogDescription>
            It will be stored as “+name”, which is how sudo names a netgroup.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <Input
            placeholder="Search netgroups..."
            value={picker.input}
            onChange={(e) => picker.setInput(e.target.value)}
          />

          <div className="max-h-64 space-y-1 overflow-y-auto">
            {candidates.length > 0 ? (
              candidates.map((netgroup) => {
                const kind = netgroupKind(netgroup)
                const isSelected = selected === netgroup.cn
                const wrongSide = netgroupSideFit(kind, wanted) === 'ignored'
                return (
                  <button
                    key={netgroup.dn}
                    type="button"
                    onClick={() => setSelected(netgroup.cn)}
                    className={`flex w-full items-start gap-2 rounded-md p-2 text-left transition-colors ${
                      isSelected ? 'bg-primary text-primary-foreground' : 'hover:bg-muted'
                    }`}
                  >
                    <Network className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
                    <span className="min-w-0 flex-1">
                      <span className="block font-medium">{netgroup.cn}</span>
                      <span
                        className={`block text-xs ${
                          isSelected ? 'text-primary-foreground/70' : 'text-muted-foreground'
                        }`}
                      >
                        {kindSummary(kind)}
                        {netgroup.description ? ` — ${netgroup.description}` : ''}
                      </span>
                    </span>
                    {wrongSide && !isSelected && (
                      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                    )}
                  </button>
                )
              })
            ) : (
              <p className="py-4 text-center text-sm text-muted-foreground">
                {picker.isFetching
                  ? 'Searching...'
                  : picker.input
                    ? 'No netgroup matches this search.'
                    : 'Every netgroup is already on this rule.'}
              </p>
            )}
          </div>
          <PickerOverflow count={picker.overflow} />

          {fit === 'ignored' && (
            <Callout variant="warning">
              <p className="font-medium">
                This netgroup names {selectedKind === 'users' ? 'users' : 'hosts'}, and you are
                adding it to {side === 'sudoUser' ? 'Users' : 'Hosts'}.
              </p>
              <p className="mt-1">
                Sudo would read the {side === 'sudoUser' ? 'user' : 'host'} slots of its members,
                which are empty — the rule would match nobody, without any error. You can add it
                anyway if that is deliberate.
              </p>
            </Callout>
          )}

          {fit === 'partial' && (
            <Callout variant="info">
              This netgroup names both users and hosts. Added here, sudo reads only its{' '}
              {side === 'sudoUser' ? 'user' : 'host'} slots — the rest is ignored, which is fine if
              that is what you meant.
            </Callout>
          )}

          {selectedKind === 'empty' && (
            <Callout variant="info">
              This netgroup names nothing of its own; whatever it holds comes from the netgroups it
              nests. Open it to see what it resolves to.
            </Callout>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={close}>
            Cancel
          </Button>
          <Button
            disabled={!selected}
            onClick={() => {
              if (selected) onPick('+' + selected)
              close()
            }}
          >
            <Plus className="h-4 w-4 mr-1" />
            Add
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function kindSummary(kind: NetgroupKind): string {
  switch (kind) {
    case 'users':
      return 'names users'
    case 'hosts':
      return 'names hosts'
    case 'mixed':
      return 'names both users and hosts'
    case 'empty':
      return 'nested only'
  }
}

/**
 * The warning that has to sit next to any "+netgroup" grant.
 *
 * Writing the grant is necessary and not sufficient: sudo resolves a netgroup on
 * the client, through NSS, ldap.conf's netgroup_base or SSSD, none of which this
 * app can see. On a host where none of that is configured the rule is inert, and
 * nothing in the directory would ever say so — the same shape as a ppolicy that
 * is loaded but applies to nothing, which this project already refuses to paper
 * over.
 */
export function NetgroupResolutionNotice({ className }: { className?: string }) {
  return (
    <Callout variant="info" className={className}>
      A “+netgroup” grant only takes effect on hosts that can resolve netgroups —
      through NSS, <code>netgroup_base</code> in <code>ldap.conf</code>, or SSSD. That is decided on
      each machine and cannot be checked from here. Where it is not configured, the rule is written
      and silently does nothing; <code>getent netgroup &lt;name&gt;</code> on a host is what settles it.
    </Callout>
  )
}
