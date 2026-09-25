import { useState } from 'react'
import { useRouter } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { showOutcome } from '@/lib/outcome'
import { api } from '@/lib/api'
import { encodeDN } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Callout } from '@/components/ui/callout'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'

const CN_PATTERN = /^[a-zA-Z0-9._-]+$/

/**
 * Renaming a group, on its own control rather than in the edit form.
 *
 * The same shape as the user rename, and for the same reasons: it changes the
 * entry's DN, it migrates references the directory does not track, and it can
 * come back with things it could not reach. None of that belongs on a form
 * somebody opened to fix a description.
 *
 * What it says is deliberately narrower than the user version, because a group
 * rename really is less invasive: members are stored as plain uids and primary
 * members by gidNumber, so nobody's membership moves. Sudo is the exception —
 * it names a group as "%cn" — and that is the one thing worth naming here.
 */
export function GroupRenameCard({ dn, cn }: { dn: string; cn: string }) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [newCn, setNewCn] = useState('')
  const [confirm, setConfirm] = useState('')

  const valid = CN_PATTERN.test(newCn) && newCn !== cn

  const rename = useMutation({
    mutationFn: (name: string) => api.groups.changeCn(dn, name),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      toast.success(`Group renamed to ${res.group.cn}`)
      showOutcome({ title: `Group renamed to ${res.group.cn}: references to check`, items: res.warnings })
      close()
      // The DN changed with the name, so this page's address did too.
      router.navigate({ to: '/groups/$dn', params: { dn: encodeDN(res.group.dn) } })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const close = () => {
    setOpen(false)
    setNewCn('')
    setConfirm('')
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Change name</CardTitle>
        <CardDescription>Rename the group's cn — its directory identity</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-warning/30 bg-warning/5 p-4">
          <div>
            <p className="font-medium">Rename this group</p>
            <p className="text-sm text-muted-foreground">
              Renames the LDAP entry and rewrites the sudo rules that grant to it. Members are
              unaffected.
            </p>
          </div>
          <Dialog open={open} onOpenChange={(next) => (next ? setOpen(true) : close())}>
            <DialogTrigger asChild>
              <Button variant="outline">Change name</Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Change name</DialogTitle>
                <DialogDescription>
                  Renaming <span className="font-mono">{cn}</span> cannot be undone automatically.
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4">
                <Callout variant="warning">
                  <p className="font-medium">This will:</p>
                  <ul className="mt-1 list-disc space-y-0.5 pl-5">
                    <li>rename the directory entry (its DN changes)</li>
                    <li>
                      rewrite <code>%{cn}</code> to the new name in every sudo rule that grants to
                      this group
                    </li>
                  </ul>
                  <p className="mt-2">
                    Members are not affected: they are stored as usernames, and primary members by
                    GID number, so neither follows the name.
                  </p>
                  <p className="mt-2">
                    Anything outside the directory that names this group — host-side
                    <code> sudoers</code> files, scripts, ACLs — keeps pointing at the old name.
                  </p>
                </Callout>
                <div className="space-y-2">
                  <Label htmlFor="newCn">New name</Label>
                  <Input
                    id="newCn"
                    value={newCn}
                    onChange={(e) => setNewCn(e.target.value)}
                    placeholder="new-group-name"
                    autoComplete="off"
                  />
                  {newCn && !CN_PATTERN.test(newCn) && (
                    <p className="text-xs text-destructive">
                      Only letters, digits, '.', '_' or '-' are allowed.
                    </p>
                  )}
                  {newCn && newCn === cn && (
                    <p className="text-xs text-destructive">This is already the current name.</p>
                  )}
                </div>
                <div className="space-y-2">
                  <Label htmlFor="confirmCn">
                    Type the current name <span className="font-mono">{cn}</span> to confirm
                  </Label>
                  <Input
                    id="confirmCn"
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
                  disabled={!valid || confirm !== cn || rename.isPending}
                  onClick={() => rename.mutate(newCn)}
                >
                  {rename.isPending ? 'Renaming...' : 'Change name'}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </CardContent>
    </Card>
  )
}
