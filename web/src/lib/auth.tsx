import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { api } from './api'
import type { AuthStatus, LoginResult } from './types'

interface AuthContextValue {
  /** null 表示还在加载 */
  status: AuthStatus | null
  loading: boolean
  /** 已完成登录（含两步验证） */
  authenticated: boolean
  username: string
  /** 登录过程中等待输入 Telegram 验证码的挑战令牌 */
  pendingChallenge: string
  refresh: () => Promise<void>
  setup: (username: string, password: string) => Promise<void>
  login: (username: string, password: string) => Promise<LoginResult>
  verifyOtp: (code: string) => Promise<void>
  logout: () => Promise<void>
  /** 会话过期时由调用方触发，回到登录页 */
  expire: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [authenticated, setAuthenticated] = useState(false)
  const [username, setUsername] = useState('')
  const [pendingChallenge, setPendingChallenge] = useState('')

  const refresh = useCallback(async () => {
    try {
      const next = await api.auth.status()
      setStatus(next)
      setAuthenticated(next.authenticated)
      setUsername(next.authenticated ? next.username : '')
      return
    } catch {
      setStatus(null)
      setAuthenticated(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const next = await api.auth.status()
        if (!cancelled) {
          setStatus(next)
          setAuthenticated(next.authenticated)
          setUsername(next.authenticated ? next.username : '')
        }
      } catch {
        if (!cancelled) setStatus(null)
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  const setup = useCallback(async (name: string, password: string) => {
    await api.auth.setup(name, password)
    const next = await api.auth.status()
    setStatus(next)
  }, [])

  const login = useCallback(async (name: string, password: string) => {
    const result = await api.auth.login(name, password)
    if (result.otp_required) {
      setPendingChallenge(result.challenge)
      return result
    }
    setAuthenticated(true)
    setUsername(result.username)
    setPendingChallenge('')
    return result
  }, [])

  const verifyOtp = useCallback(
    async (code: string) => {
      const res = await api.auth.verifyOtp(pendingChallenge, code)
      setAuthenticated(true)
      setUsername(res.username)
      setPendingChallenge('')
    },
    [pendingChallenge],
  )

  const logout = useCallback(async () => {
    try {
      await api.auth.logout()
    } finally {
      setAuthenticated(false)
      setUsername('')
      setPendingChallenge('')
      await refresh()
    }
  }, [refresh])

  const expire = useCallback(() => {
    setAuthenticated(false)
    setUsername('')
    setPendingChallenge('')
  }, [])

  const value = useMemo(
    () => ({
      status,
      loading,
      authenticated,
      username,
      pendingChallenge,
      refresh,
      setup,
      login,
      verifyOtp,
      logout,
      expire,
    }),
    [status, loading, authenticated, username, pendingChallenge, refresh, setup, login, verifyOtp, logout, expire],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth 必须在 AuthProvider 内使用')
  }
  return ctx
}
