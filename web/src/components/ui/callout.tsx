import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { AlertTriangle, CheckCircle2, Info, XCircle } from 'lucide-react'
import { cn } from '@/lib/utils'

const calloutVariants = cva(
  'flex items-start gap-2 rounded-md border px-3 py-2 text-sm',
  {
    variants: {
      variant: {
        error: 'border-destructive/40 bg-destructive/10 text-destructive',
        warning: 'border-warning/40 bg-warning/10 text-warning',
        success: 'border-success/40 bg-success/10 text-success',
        info: 'border-info/40 bg-info/10 text-info',
      },
    },
    defaultVariants: {
      variant: 'info',
    },
  },
)

const ICONS = {
  error: XCircle,
  warning: AlertTriangle,
  success: CheckCircle2,
  info: Info,
} as const

export interface CalloutProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof calloutVariants> {
  // Set to false for dense inline placements where the icon adds noise.
  icon?: boolean
}

// Callout is the single inline message surface: query failures, truncation
// warnings, and the few confirmations that have to stay on the page rather
// than fly past in a toast. Colours come from the theme's status tokens so the
// same markup reads correctly in light and dark mode.
function Callout({ className, variant = 'info', icon = true, children, ...props }: CalloutProps) {
  const Icon = ICONS[variant ?? 'info']
  return (
    <div
      role={variant === 'error' ? 'alert' : 'status'}
      className={cn(calloutVariants({ variant }), className)}
      {...props}
    >
      {icon && <Icon className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />}
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  )
}

export { Callout }
