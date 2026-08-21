import { Check, Circle, Dot, Info } from 'lucide-react'
import type { PasswordPolicy } from '@/lib/api'

interface Requirement {
  text: string
  /** Whether the typed value satisfies it, or undefined when this side cannot tell. */
  met?: boolean
}

/**
 * What the directory requires of a password.
 *
 * The strength meter next to it is this app's own heuristic and has nothing to
 * do with the policy: a password it calls "Strong" is refused if it repeats one
 * of the last `pwdInHistory` values, and one it calls "Weak" sails through a
 * directory with no policy at all. Since the app already reads the applicable
 * policy — to show and change it in a card below — it can state the rules rather
 * than let the operator discover them through a rejection.
 *
 * The footnote is not a hedge. ppolicy exempts a privileged bind identity from
 * password policy, and LDAP Warden writes with whatever identity LDAP_BIND_DN
 * names — the rootdn in the bundled stack and in the shipped .env.example. So a
 * password set from this form may well be accepted here and then refused the
 * next time the user changes it themselves. Which of the two happens depends on
 * a privilege the app cannot read, so it says so instead of guessing.
 *
 * Renders nothing when no policy is known or it sets none of these: naming the
 * wrong requirements would be worse than naming none.
 */
export function PasswordRequirements({
  policy,
  password,
}: {
  policy: PasswordPolicy | undefined
  password: string
}) {
  if (!policy) return null

  const requirements: Requirement[] = []
  if (policy.pwdMinLength) {
    requirements.push({
      text: `At least ${policy.pwdMinLength} characters`,
      met: password.length >= policy.pwdMinLength,
    })
  }
  if (policy.pwdInHistory) {
    requirements.push({
      text: `Not one of the last ${policy.pwdInHistory} passwords used on this account`,
    })
  }
  if (policy.pwdCheckQuality) {
    // What "quality" means is the directory's business (pwdCheckModule, or
    // slapd's own check), so describe the requirement instead of inventing it.
    requirements.push({ text: "Complex enough for the directory's quality check" })
  }

  if (requirements.length === 0) return null

  return (
    <div className="space-y-1.5 rounded-md border bg-muted/40 px-3 py-2">
      <p className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <Info className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
        Policy {policy.cn} requires
      </p>
      <ul className="space-y-0.5">
        {requirements.map((requirement) => (
          <li key={requirement.text} className="flex items-start gap-1.5 text-xs">
            {requirement.met === undefined ? (
              <Dot className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
            ) : requirement.met ? (
              <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-success" aria-hidden="true" />
            ) : (
              <Circle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
            )}
            <span className={requirement.met ? 'text-success' : 'text-muted-foreground'}>
              {requirement.text}
            </span>
          </li>
        ))}
      </ul>
      <p className="border-t pt-1.5 text-xs text-muted-foreground">
        A password set here can be accepted even if it breaks these rules — the directory exempts
        privileged accounts from its own policy, and LDAP Warden writes as the identity it binds
        with. The user will still hit these rules the next time they change it themselves.
      </p>
    </div>
  )
}
