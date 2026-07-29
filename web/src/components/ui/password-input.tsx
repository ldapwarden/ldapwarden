import * as React from 'react'
import { Eye, EyeOff, Sparkles } from 'lucide-react'
import { toast } from 'sonner'
import { Input } from './input'
import { Button } from './button'
import { cn } from '@/lib/utils'
import { generatePassword } from '@/lib/password'

interface PasswordInputProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type'> {
  value: string
  onValueChange: (value: string) => void
  // Offer a generate button. For a password the operator sets on someone else's
  // account; pointless when they are typing their own.
  generate?: boolean
  // Called with the generated value, so a form can fill a confirmation field too.
  onGenerate?: (value: string) => void
}

/**
 * Password field with a reveal toggle, and optionally a generator.
 *
 * Every password input was a bare `type="password"`: an admin setting someone
 * else's initial password had to invent one and could not check what they had
 * typed, which is how "Welcome123" ends up in a directory.
 */
export const PasswordInput = React.forwardRef<HTMLInputElement, PasswordInputProps>(
  ({ value, onValueChange, generate = false, onGenerate, className, disabled, ...props }, ref) => {
    const [revealed, setRevealed] = React.useState(false)

    const handleGenerate = async () => {
      const password = generatePassword()
      onValueChange(password)
      onGenerate?.(password)
      // Reveal it: a generated password the operator cannot see is one they
      // cannot pass on.
      setRevealed(true)

      // Copying needs a secure context, so it can legitimately fail over plain
      // HTTP on a LAN address — say what happened either way.
      try {
        await navigator.clipboard.writeText(password)
        toast.success('Password generated and copied to the clipboard')
      } catch {
        toast.success('Password generated — copy it from the field')
      }
    }

    return (
      <div className="flex gap-2">
        <div className="relative flex-1">
          <Input
            ref={ref}
            type={revealed ? 'text' : 'password'}
            value={value}
            onChange={(e) => onValueChange(e.target.value)}
            disabled={disabled}
            className={cn('pr-9', className)}
            {...props}
          />
          <button
            type="button"
            onClick={() => setRevealed((r) => !r)}
            // The value is already on screen for the sighted user when revealed;
            // the label is what tells a screen-reader user which state they are in.
            aria-label={revealed ? 'Hide password' : 'Show password'}
            aria-pressed={revealed}
            className="absolute right-0 top-0 flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:text-foreground focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring"
          >
            {revealed ? (
              <EyeOff className="h-4 w-4" aria-hidden="true" />
            ) : (
              <Eye className="h-4 w-4" aria-hidden="true" />
            )}
          </button>
        </div>
        {generate && (
          <Button type="button" variant="outline" onClick={handleGenerate} disabled={disabled}>
            <Sparkles className="h-4 w-4 mr-1" aria-hidden="true" />
            Generate
          </Button>
        )}
      </div>
    )
  },
)
PasswordInput.displayName = 'PasswordInput'
