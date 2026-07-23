import * as React from 'react'
import { cn } from '@/lib/utils'

interface CheckboxProps
  extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'onChange' | 'type'> {
  /** Tri-state visual for a "select all" header when only some rows are picked. */
  indeterminate?: boolean
  onCheckedChange?: (checked: boolean) => void
}

/**
 * Styled native checkbox. Native (rather than a Radix primitive) keeps it
 * dependency-free and keyboard-accessible out of the box; `indeterminate` is
 * driven imperatively since it isn't a reflectable HTML attribute.
 */
export const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  ({ className, indeterminate, onCheckedChange, checked, ...props }, ref) => {
    const innerRef = React.useRef<HTMLInputElement>(null)
    React.useImperativeHandle(ref, () => innerRef.current as HTMLInputElement)
    React.useEffect(() => {
      if (innerRef.current) innerRef.current.indeterminate = !!indeterminate
    }, [indeterminate])

    return (
      <input
        type="checkbox"
        ref={innerRef}
        checked={checked}
        onChange={(e) => onCheckedChange?.(e.target.checked)}
        className={cn(
          'h-4 w-4 shrink-0 cursor-pointer rounded border-input accent-primary focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
          className,
        )}
        {...props}
      />
    )
  },
)
Checkbox.displayName = 'Checkbox'
