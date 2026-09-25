import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { showOutcome } from '@/lib/outcome'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Callout } from '@/components/ui/callout'
import { FieldRow } from '@/components/ui/field-row'
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
 * Changing a user's POSIX numbers, behind its own confirmation.
 *
 * Separate from the form these fields sit in, for a reason the rename does not
 * have: renaming migrates every reference it can find, while this cannot follow
 * the numbers where they actually matter. A uidNumber is what the filesystem
 * writes into every inode the account owns, on every host, and nothing in the
 * directory knows where those are — so the change succeeds instantly and the
 * consequences land somewhere this app cannot see.
 *
 * Which is why the dialog spends most of its space on what it will *not* do.
 */
export function PosixIdsChange({
  dn,
  uid,
  uidNumber,
  gidNumber,
}: {
  dn: string
  uid: string
  uidNumber?: number
  gidNumber?: number
}) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [newUid, setNewUid] = useState('')
  const [newGid, setNewGid] = useState('')
  const [confirm, setConfirm] = useState('')

  const parsed = (value: string) => (value.trim() === '' ? undefined : Number(value))
  const uidValue = parsed(newUid)
  const gidValue = parsed(newGid)

  const uidBad = newUid.trim() !== '' && (!Number.isInteger(uidValue) || (uidValue ?? 0) < 0)
  const gidBad = newGid.trim() !== '' && (!Number.isInteger(gidValue) || (gidValue ?? 0) < 0)
  const changesSomething =
    (uidValue !== undefined && uidValue !== uidNumber) ||
    (gidValue !== undefined && gidValue !== gidNumber)

  const change = useMutation({
    mutationFn: () =>
      api.users.changePosixIds(dn, {
        ...(uidValue !== undefined && uidValue !== uidNumber ? { uidNumber: uidValue } : {}),
        ...(gidValue !== undefined && gidValue !== gidNumber ? { gidNumber: gidValue } : {}),
      }),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ['user', dn] })
      queryClient.invalidateQueries({ queryKey: ['users'] })
      toast.success('POSIX numbers changed')
      // Not a toast: this is the list of things still to do on the hosts, and a
      // toast is gone before anyone has written them down.
      showOutcome({
        title: 'POSIX numbers changed: finish on the hosts',
        description:
          'The directory is updated. Files on disk still carry the old numbers, and nothing here can reach them.',
        items: res.warnings,
      })
      close()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const close = () => {
    setOpen(false)
    setNewUid('')
    setNewGid('')
    setConfirm('')
  }

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-warning/30 bg-warning/5 p-4">
      <div>
        <p className="font-medium">Change the POSIX numbers</p>
        <p className="text-sm text-muted-foreground">
          The directory accepts them at once. Files already on disk do not follow.
        </p>
      </div>
      <Dialog open={open} onOpenChange={(next) => (next ? setOpen(true) : close())}>
        <DialogTrigger asChild>
          <Button variant="outline">Change UID / GID number</Button>
        </DialogTrigger>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Change UID / GID number</DialogTitle>
            <DialogDescription>
              For <span className="font-mono">{uid}</span>, currently uid{' '}
              <span className="font-mono">{uidNumber ?? '—'}</span> and gid{' '}
              <span className="font-mono">{gidNumber ?? '—'}</span>.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <Callout variant="warning">
              <p className="font-medium">This app cannot finish this change.</p>
              <p className="mt-1">
                A uid number is what the filesystem stores in every file the account owns. Changing
                it here does not touch those files: on every host, they stay owned by the old
                number. The account loses access to its own data until someone reassigns them:
              </p>
              <pre className="mt-2 overflow-x-auto rounded bg-muted p-2 text-xs">
                find / -xdev -uid {uidNumber ?? '<old>'} -exec chown {newUid || '<new>'} {'{}'} +
              </pre>
              <p className="mt-2">
                And if the old number is later issued to someone else, they inherit everything left
                behind. The same applies to the gid number and group-owned files.
              </p>
              <p className="mt-2">
                A <code>sambaSID</code> whose RID came from the old number is realigned here; one
                allocated by your domain is left alone and reported.
              </p>
            </Callout>

            <FieldRow>
              <div className="space-y-2">
                <Label htmlFor="newUidNumber">New UID number</Label>
                <Input
                  id="newUidNumber"
                  inputMode="numeric"
                  value={newUid}
                  onChange={(e) => setNewUid(e.target.value)}
                  placeholder={String(uidNumber ?? '')}
                  autoComplete="off"
                />
                {uidBad && <p className="text-xs text-destructive">Must be a whole number.</p>}
              </div>
              <div className="space-y-2">
                <Label htmlFor="newGidNumber">New GID number</Label>
                <Input
                  id="newGidNumber"
                  inputMode="numeric"
                  value={newGid}
                  onChange={(e) => setNewGid(e.target.value)}
                  placeholder={String(gidNumber ?? '')}
                  autoComplete="off"
                />
                {gidBad && <p className="text-xs text-destructive">Must be a whole number.</p>}
              </div>
            </FieldRow>
            <p className="text-xs text-muted-foreground">
              Leave a field empty to keep the number it has.
            </p>

            <div className="space-y-2">
              <Label htmlFor="confirmPosix">
                Type the username <span className="font-mono">{uid}</span> to confirm
              </Label>
              <Input
                id="confirmPosix"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                autoComplete="off"
              />
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={close}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={
                uidBad || gidBad || !changesSomething || confirm !== uid || change.isPending
              }
              onClick={() => change.mutate()}
            >
              {change.isPending ? 'Changing...' : 'Change numbers'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
