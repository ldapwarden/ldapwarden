/**
 * Centred loading spinner for in-page content (lists, detail panels). Unlike
 * LoadingScreen it doesn't take the full viewport. Carries an accessible status
 * role so screen readers announce the load; the glyph itself is hidden.
 */
export function InlineSpinner({ label = 'Loading…' }: { label?: string }) {
  return (
    <div
      className="flex items-center justify-center py-8"
      role="status"
      aria-live="polite"
    >
      <div
        className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"
        aria-hidden="true"
      ></div>
      <span className="sr-only">{label}</span>
    </div>
  )
}
