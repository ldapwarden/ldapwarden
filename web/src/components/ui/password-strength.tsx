import { useMemo } from 'react'

/**
 * A password strength meter (6 segments + label) shared by every form that
 * collects a new password. Renders nothing when `password` is empty.
 */
export function PasswordStrength({ password }: { password: string }) {
  const strength = useMemo(() => {
    let score = 0
    if (password.length >= 8) score++
    if (password.length >= 12) score++
    if (/[a-z]/.test(password)) score++
    if (/[A-Z]/.test(password)) score++
    if (/[0-9]/.test(password)) score++
    if (/[^a-zA-Z0-9]/.test(password)) score++

    if (score <= 2) return { score, label: 'Weak', color: 'bg-destructive' }
    if (score <= 4) return { score, label: 'Medium', color: 'bg-yellow-500' }
    return { score, label: 'Strong', color: 'bg-green-500' }
  }, [password])

  if (!password) return null

  return (
    <div className="space-y-1">
      <div className="flex gap-1">
        {[1, 2, 3, 4, 5, 6].map((i) => (
          <div
            key={i}
            className={`h-1 flex-1 rounded ${
              i <= strength.score ? strength.color : 'bg-muted'
            }`}
          />
        ))}
      </div>
      <p
        className={`text-xs ${
          strength.score <= 2
            ? 'text-destructive'
            : strength.score <= 4
              ? 'text-yellow-600'
              : 'text-green-600'
        }`}
      >
        Password strength: {strength.label}
      </p>
    </div>
  )
}
