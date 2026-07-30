import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { runBulk, bulkSummary } from '@/lib/bulk'
import { BulkResultDialog, type BulkOutcome } from '@/components/bulk-result-dialog'
import { BulkActionBar } from '@/components/bulk-action-bar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { PickerOverflow } from '@/components/ui/picker-overflow'
import { usePickerSearch } from '@/lib/use-picker-search'
import { Lock, Unlock, Trash2, UserPlus, UserMinus, CalendarClock } from 'lucide-react'

export interface BulkUser {
  dn: string
  uid: string
  cn: string
  displayName?: string
}

interface GroupOption {
  dn: string
  cn: string
}

/**
 * Selection toolbar + dialogs for bulk actions on the users list. Each action
 * loops the existing per-item API endpoints via runBulk (bounded concurrency),
 * reporting a summary toast. On completion it invalidates the affected queries
 * and clears the selection via onDone.
 */
export function UsersBulkActions({
  selected,
  onClear,
}: {
  selected: BulkUser[]
  onClear: () => void
}) {
  const queryClient = useQueryClient()
  const [running, setRunning] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [groupMode, setGroupMode] = useState<'add' | 'remove' | null>(null)
  const [expirationOpen, setExpirationOpen] = useState(false)
  // Set when a run had failures, so they can be listed rather than summarised.
  const [outcome, setOutcome] = useState<BulkOutcome<BulkUser> | null>(null)

  const invalidateUsers = () => queryClient.invalidateQueries({ queryKey: ['users'] })
  const invalidateGroups = () => queryClient.invalidateQueries({ queryKey: ['groups'] })

  // Run a bulk action, report it, refresh and clear the selection.
  async function run(
    verb: string,
    worker: (u: BulkUser) => Promise<unknown>,
    opts: { touchesGroups?: boolean } = {},
  ) {
    setRunning(true)
    try {
      const result = await runBulk(selected, worker)
      invalidateUsers()
      if (opts.touchesGroups) invalidateGroups()
      const summary = bulkSummary(verb, 'user', result)
      if (result.failed.length === 0) {
        toast.success(summary)
      } else {
        // The toast reports the shape; the dialog says which items failed and why.
        toast.error(summary)
        setOutcome({ verb, noun: 'user', succeeded: result.succeeded, failed: result.failed })
      }
      onClear()
    } finally {
      setRunning(false)
    }
  }

  return (
    <>
      <BulkResultDialog
        outcome={outcome}
        label={(user) => `${user.displayName || user.cn} (${user.uid})`}
        onClose={() => setOutcome(null)}
      />
      <BulkActionBar count={selected.length} onClear={onClear}>
        <Button variant="outline" size="sm" disabled={running} onClick={() => run('Locked', (u) => api.users.lock(u.dn))}>
          <Lock className="h-4 w-4 mr-1" />
          Lock
        </Button>
        <Button variant="outline" size="sm" disabled={running} onClick={() => run('Unlocked', (u) => api.users.unlock(u.dn))}>
          <Unlock className="h-4 w-4 mr-1" />
          Unlock
        </Button>
        <Button variant="outline" size="sm" disabled={running} onClick={() => setGroupMode('add')}>
          <UserPlus className="h-4 w-4 mr-1" />
          Add to group
        </Button>
        <Button variant="outline" size="sm" disabled={running} onClick={() => setGroupMode('remove')}>
          <UserMinus className="h-4 w-4 mr-1" />
          Remove from group
        </Button>
        <Button variant="outline" size="sm" disabled={running} onClick={() => setExpirationOpen(true)}>
          <CalendarClock className="h-4 w-4 mr-1" />
          Set expiration
        </Button>
        <Button variant="destructive" size="sm" disabled={running} onClick={() => setDeleteOpen(true)}>
          <Trash2 className="h-4 w-4 mr-1" />
          Delete
        </Button>
      </BulkActionBar>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={(open) => { if (!open) setDeleteOpen(false) }}
        title="Delete users"
        description={`Delete ${selected.length} selected user${selected.length === 1 ? '' : 's'}? This permanently removes them from the directory and cannot be undone.`}
        confirmLabel={`Delete ${selected.length}`}
        pendingLabel="Deleting..."
        isPending={running}
        onConfirm={async () => {
          await run('Deleted', (u) => api.users.delete(u.dn), { touchesGroups: true })
          setDeleteOpen(false)
        }}
      />

      <GroupPickerDialog
        mode={groupMode}
        count={selected.length}
        running={running}
        onClose={() => setGroupMode(null)}
        onConfirm={async (group) => {
          const verb = groupMode === 'add' ? 'Added to group' : 'Removed from group'
          const worker =
            groupMode === 'add'
              ? (u: BulkUser) => api.groups.addMember(group.dn, u.uid)
              : (u: BulkUser) => api.groups.removeMember(group.dn, u.uid)
          await run(verb, worker, { touchesGroups: true })
          setGroupMode(null)
        }}
      />

      <ExpirationDialog
        open={expirationOpen}
        count={selected.length}
        running={running}
        onClose={() => setExpirationOpen(false)}
        onConfirm={async (date) => {
          const verb = date ? 'Set expiration for' : 'Cleared expiration for'
          await run(verb, (u) => api.users.setExpiration(u.dn, date || null))
          setExpirationOpen(false)
        }}
      />
    </>
  )
}

function GroupPickerDialog({
  mode,
  count,
  running,
  onClose,
  onConfirm,
}: {
  mode: 'add' | 'remove' | null
  count: number
  running: boolean
  onClose: () => void
  onConfirm: (group: GroupOption) => void
}) {
  // The chosen group is held whole rather than by DN: the candidate list is one
  // search's worth, so a later keystroke can drop the choice out of it, and the
  // confirm button must not go dead because of that.
  const [selected, setSelected] = useState<GroupOption | null>(null)
  const picker = usePickerSearch(['groups', 'picker', 'bulk'], api.groups.list, {
    sort: 'cn',
    enabled: mode !== null,
  })

  return (
    <Dialog
      open={mode !== null}
      onOpenChange={(open) => {
        if (!open) {
          picker.reset()
          setSelected(null)
          onClose()
        }
      }}
    >
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{mode === 'remove' ? 'Remove from group' : 'Add to group'}</DialogTitle>
          <DialogDescription>
            {mode === 'remove'
              ? `Remove the ${count} selected user${count === 1 ? '' : 's'} from a group.`
              : `Add the ${count} selected user${count === 1 ? '' : 's'} to a group.`}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Input
            placeholder="Search groups..."
            value={picker.input}
            onChange={(e) => picker.setInput(e.target.value)}
          />
          <div className="max-h-64 overflow-y-auto space-y-1">
            {picker.items.length > 0 ? (
              picker.items.map((group) => {
                const isSelected = group.dn === selected?.dn
                return (
                  <button
                    key={group.dn}
                    type="button"
                    onClick={() => setSelected({ dn: group.dn, cn: group.cn })}
                    className={`w-full flex items-center gap-3 p-2 rounded-md text-left transition-colors ${
                      isSelected ? 'bg-primary text-primary-foreground' : 'hover:bg-muted'
                    }`}
                  >
                    <span className="font-medium">{group.cn}</span>
                  </button>
                )
              })
            ) : (
              <p className="text-sm text-muted-foreground text-center py-4">
                {picker.isFetching ? 'Searching...' : 'No groups found'}
              </p>
            )}
          </div>
          <PickerOverflow count={picker.overflow} />
          {selected && !picker.items.some((g) => g.dn === selected.dn) && (
            <p className="text-xs text-muted-foreground">
              Selected: <span className="font-medium">{selected.cn}</span>
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant={mode === 'remove' ? 'destructive' : 'default'}
            disabled={!selected || running}
            onClick={() => selected && onConfirm(selected)}
          >
            {running ? 'Working...' : mode === 'remove' ? 'Remove' : 'Add'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ExpirationDialog({
  open,
  count,
  running,
  onClose,
  onConfirm,
}: {
  open: boolean
  count: number
  running: boolean
  onClose: () => void
  onConfirm: (date: string) => void
}) {
  const [date, setDate] = useState('')

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) {
          setDate('')
          onClose()
        }
      }}
    >
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Set expiration</DialogTitle>
          <DialogDescription>
            Set the account expiration date for the {count} selected user{count === 1 ? '' : 's'}. Leave
            empty to clear any existing expiration.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="bulk-expiration">Expiration date</Label>
          <Input id="bulk-expiration" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={running} onClick={() => onConfirm(date)}>
            {running ? 'Working...' : date ? 'Set expiration' : 'Clear expiration'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
