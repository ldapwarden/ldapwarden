/**
 * Full-viewport loading indicator with an accessible status role so screen
 * readers announce that the app is busy. The spinning glyph itself is hidden
 * from assistive tech; the visually-hidden label carries the meaning.
 */
export function LoadingScreen({ label = 'Loading…' }: { label?: string }) {
  return (
    <div
      className="flex items-center justify-center min-h-screen"
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
