import { useRouter } from '@tanstack/react-router'
import { AlertTriangle } from 'lucide-react'
import { Button } from '@/components/ui/button'

/**
 * Rendered by the router when a route (or one of its children) throws during
 * render or loading. Without it, an uncaught render error blanks the whole app
 * with no way out. It shows a friendly message and lets the user retry the
 * failed route or go home, while surfacing the message for a bug report.
 */
export function RouteError({ error }: { error: Error }) {
  const router = useRouter()

  return (
    <div className="flex min-h-screen flex-col items-center justify-center py-20 text-center">
      <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10">
        <AlertTriangle className="h-6 w-6 text-destructive" />
      </div>
      <h1 className="text-2xl font-bold">Something went wrong</h1>
      <p className="mt-2 max-w-md text-sm text-muted-foreground">
        An unexpected error occurred while rendering this page. You can try again
        or return home.
      </p>
      {error?.message && (
        <pre className="mt-4 max-w-md overflow-x-auto rounded-md bg-muted p-3 text-left text-xs text-muted-foreground">
          {error.message}
        </pre>
      )}
      <div className="mt-6 flex gap-2">
        <Button variant="outline" onClick={() => router.invalidate()}>
          Try again
        </Button>
        <Button onClick={() => router.navigate({ to: '/' })}>Go to home</Button>
      </div>
    </div>
  )
}
