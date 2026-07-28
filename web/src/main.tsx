import { StrictMode, useMemo } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { routeTree } from './routeTree.gen'
import { AuthProvider, useAuth } from './lib/auth'
import { ThemeProvider } from './lib/theme'
import { ApiError } from './lib/api'
import { NotFound } from './components/not-found'
import { LoadingScreen } from './components/loading-screen'
import './index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60,
      retry: (failureCount, error) => {
        // Never retry a cancelled request: the abort came from React Query
        // itself (navigation/unmount) and retrying it races with the
        // library's own cancellation handling.
        if (error instanceof DOMException && error.name === 'AbortError') return false
        if (error instanceof ApiError && error.status === 401) return false
        return failureCount < 1
      },
    },
  },
})

const router = createRouter({
  routeTree,
  context: {
    auth: undefined!,
    queryClient,
  },
  defaultPreload: false,
  defaultNotFoundComponent: NotFound,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

function InnerApp() {
  const auth = useAuth()
  // Memoize context to prevent unnecessary router re-evaluations on every render
  const context = useMemo(() => ({ auth, queryClient }), [auth])

  // Mount the router only once the session probe has answered. Route
  // `beforeLoad` guards read `context.auth`, and while the probe is in flight
  // that context says "not authenticated, no permissions" — so opening
  // /audit-logs or /users/new directly (a bookmark, a shared link, a plain
  // reload) used to be redirected away before the session arrived, landing the
  // user on /users instead of the page they asked for.
  if (auth.isLoading) {
    return <LoadingScreen />
  }

  return <RouterProvider router={router} context={context} />
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <AuthProvider>
          <InnerApp />
        </AuthProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
