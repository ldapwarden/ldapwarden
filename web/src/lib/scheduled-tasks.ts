/**
 * How a scheduled task is named and counted in the UI.
 *
 * The dashboard and the admin page both list task runs, and both used to derive
 * the label with `taskName === 'users_expiration' ? 'Account' : 'Password'`.
 * That is a two-way test over three task names, so every `lockouts_scan` run —
 * the only record the app keeps of a directory-side lockout, since ppolicy
 * erases pwdFailureTime at the account's next bind — was displayed as "Password
 * expiration" on the very screen an operator opens to check that the scan ran.
 *
 * Keyed off the names the scheduler writes (internal/scheduler/*.go), with a
 * fallback so a task added later shows its raw name rather than another task's.
 */
interface TaskDisplay {
  label: string
  /** What `usersChecked` counted for this task: the two are not the same thing. */
  checkedLabel: (count: number) => string
}

const TASKS: Record<string, TaskDisplay> = {
  users_expiration: {
    label: 'Account expiration',
    checkedLabel: (n) => `${n} account${n === 1 ? '' : 's'} checked`,
  },
  passwords_expiration: {
    label: 'Password expiration',
    checkedLabel: (n) => `${n} password${n === 1 ? '' : 's'} checked`,
  },
  lockouts_scan: {
    label: 'Lockout scan',
    // The scan only reads the locked accounts, so "checked" would overstate
    // what it looked at — and 0 is the healthy outcome, not a broken run.
    checkedLabel: (n) => `${n} locked account${n === 1 ? '' : 's'} seen`,
  },
}

export function taskLabel(taskName: string): string {
  return TASKS[taskName]?.label ?? taskName
}

export function taskCheckedLabel(taskName: string, count: number): string {
  return TASKS[taskName]?.checkedLabel(count) ?? `${count} checked`
}

export function taskNotifiedLabel(count: number): string {
  return `${count} notification${count === 1 ? '' : 's'} sent`
}
