import * as React from 'react'
import { cn } from '@/lib/utils'
import { Calendar } from 'lucide-react'

export interface DatePickerProps
  extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type' | 'value' | 'onChange'> {
  value?: string
  onChange?: (value: string) => void
}

const DatePicker = React.forwardRef<HTMLInputElement, DatePickerProps>(
  ({ className, value, onChange, ...props }, ref) => {
    return (
      <div className="relative">
        <input
          type="date"
          className={cn(
            'flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 md:text-sm',
            // Custom styling for the date input
            '[&::-webkit-calendar-picker-indicator]:opacity-0 [&::-webkit-calendar-picker-indicator]:absolute [&::-webkit-calendar-picker-indicator]:right-0 [&::-webkit-calendar-picker-indicator]:w-full [&::-webkit-calendar-picker-indicator]:h-full [&::-webkit-calendar-picker-indicator]:cursor-pointer',
            className
          )}
          ref={ref}
          value={value || ''}
          onChange={(e) => onChange?.(e.target.value)}
          {...props}
        />
        <Calendar className="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
      </div>
    )
  }
)
DatePicker.displayName = 'DatePicker'

export { DatePicker }
