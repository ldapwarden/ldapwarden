import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react'
import { api, type Session, ApiError } from './api'

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

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (token) {
      api.auth.me()
        .then(setSession)
        .catch(() => {
          localStorage.removeItem('token')
        })
        .finally(() => setIsLoading(false))
    } else {
      setIsLoading(false)
    }
  }, [])

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
    }
  }, [])

  const hasPermission = useCallback((permission: string) => {
    return session?.permissions.includes(permission) ?? false
  }, [session])

  return (
    <AuthContext.Provider value={{
      session,
      isLoading,
      isAuthenticated: !!session,
      login,
      logout,
      hasPermission,
    }}>
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
