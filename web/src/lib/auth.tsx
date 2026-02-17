import { createContext, useContext, useState, useEffect, useCallback, useMemo, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api, setOnAuthError, type Session, ApiError } from './api'

interface AuthContextType {
  session: Session | null
  isLoading: boolean
  isAuthenticated: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  hasPermission: (permission: string) => boolean
}

const AuthContext = createContext<AuthContextType | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const queryClient = useQueryClient()

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (token) {
      let cancelled = false
      api.auth.me()
        .then(session => {
          if (!cancelled) setSession(session)
        })
        .catch(() => {
          if (!cancelled) localStorage.removeItem('token')
        })
        .finally(() => {
          if (!cancelled) setIsLoading(false)
        })
      return () => { cancelled = true }
    } else {
      setIsLoading(false)
    }
  }, [])

  // Register a callback so the API layer can clear auth state on 401
  useEffect(() => {
    setOnAuthError(() => {
      setSession(null)
      queryClient.clear()
    })
    return () => setOnAuthError(null)
  }, [queryClient])

  const login = useCallback(async (username: string, password: string) => {
    const response = await api.auth.login(username, password)
    localStorage.setItem('token', response.token)
    setSession(response.session)
  }, [])

  const logout = useCallback(async () => {
    try {
      await api.auth.logout()
    } catch (error) {
      if (!(error instanceof ApiError && error.status === 401)) {
        throw error
      }
    } finally {
      localStorage.removeItem('token')
      setSession(null)
      queryClient.clear()
    }
  }, [queryClient])

  const hasPermission = useCallback((permission: string) => {
    return session?.permissions.includes(permission) ?? false
  }, [session])

  const value = useMemo<AuthContextType>(() => ({
    session,
    isLoading,
    isAuthenticated: !!session,
    login,
    logout,
    hasPermission,
  }), [session, isLoading, login, logout, hasPermission])

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}
