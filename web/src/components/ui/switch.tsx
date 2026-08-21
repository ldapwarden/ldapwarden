import * as React from "react"
import { cn } from "@/lib/utils"

interface SwitchBaseProps
  extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "children"> {
  checked?: boolean
  onCheckedChange?: (checked: boolean) => void
  /** Secondary line under the label, for the "setting + explanation" row. */
  description?: React.ReactNode
  /** Which side of the track the label sits on. */
  labelPosition?: "start" | "end"
  /** Extra classes for the label text, e.g. to mute a compact list filter. */
  labelClassName?: string
}

/**
 * A switch has to carry its own name and its own label.
 *
 * Every call site used to put a `<Label>` (or a plain `<span>`) next to the
 * control and wrap the pair in a `<label>`. That does nothing here: a `<label>`
 * names and activates *form controls*, not buttons. So each switch was
 * announced to a screen reader with no accessible name, and the visible text —
 * under a `cursor-pointer` promising otherwise — was not clickable, leaving a
 * 36×20px track as the only target.
 *
 * Passing `label` makes the component render the text inside the button, so the
 * name and the hit area come from the same element. A switch whose meaning
 * genuinely lives elsewhere can pass `aria-label` instead; the types below
 * require one or the other, so a new unnamed switch will not typecheck.
 */
type SwitchProps =
  | (SwitchBaseProps & { label: React.ReactNode; "aria-label"?: never })
  | (SwitchBaseProps & { label?: undefined; "aria-label": string })

const Switch = React.forwardRef<HTMLButtonElement, SwitchProps>(
  (
    {
      className,
      checked = false,
      onCheckedChange,
      disabled,
      label,
      description,
      labelPosition = "start",
      labelClassName,
      ...props
    },
    ref
  ) => {
    // Decorative: the button itself carries role="switch" and aria-checked, so
    // the track must not be announced a second time.
    const track = (
      <span
        aria-hidden="true"
        className={cn(
          "inline-flex h-5 w-9 shrink-0 items-center rounded-full border-2 border-transparent shadow-xs transition-colors",
          checked ? "bg-primary" : "bg-input"
        )}
      >
        <span
          className={cn(
            "pointer-events-none block h-4 w-4 rounded-full bg-background shadow-lg ring-0 transition-transform",
            checked ? "translate-x-4" : "translate-x-0"
          )}
        />
      </span>
    )

    const text =
      label === undefined ? null : (
        <span className="flex min-w-0 flex-col gap-0.5 text-left">
          <span className={cn("text-sm font-medium leading-snug", labelClassName)}>
            {label}
          </span>
          {description && (
            <span className="text-xs font-normal text-muted-foreground">{description}</span>
          )}
        </span>
      )

    return (
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        data-state={checked ? "checked" : "unchecked"}
        disabled={disabled}
        ref={ref}
        onClick={() => onCheckedChange?.(!checked)}
        className={cn(
          "group inline-flex cursor-pointer items-center gap-3 rounded-md text-left focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50",
          // A labelled setting row owns its full width, so the whole strip
          // becomes the target rather than the track alone. A trailing label is
          // a compact filter and stays intrinsically sized.
          text && labelPosition === "start" && "w-full justify-between",
          className
        )}
        {...props}
      >
        {labelPosition === "start" ? (
          <>
            {text}
            {track}
          </>
        ) : (
          <>
            {track}
            {text}
          </>
        )}
      </button>
    )
  }
)
Switch.displayName = "Switch"

export { Switch }
