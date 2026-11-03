import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api } from './api'
import type { Account } from './types'

interface AccountContextValue {
  accounts: Account[]
  account: Account | null
  selectAccount: (id: string) => void
  /** 区域覆盖值，为空表示使用账号的默认区域 */
  region: string
  setRegion: (region: string) => void
  loading: boolean
  error: string
  reload: () => Promise<void>
}

const AccountContext = createContext<AccountContextValue | null>(null)

const STORAGE_KEY = 'oci-manager.account'

export function AccountProvider({ children }: { children: ReactNode }) {
  const [accounts, setAccounts] = useState<Account[]>([])
  const [currentId, setCurrentId] = useState<string>(() => localStorage.getItem(STORAGE_KEY) ?? '')
  const [region, setRegion] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const reload = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const list = await api.accounts.list()
      setAccounts(list)
      setCurrentId((prev) => (prev && list.some((a) => a.id === prev) ? prev : (list[0]?.id ?? '')))
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void reload()
  }, [reload])

  const selectAccount = useCallback((id: string) => {
    setCurrentId(id)
    localStorage.setItem(STORAGE_KEY, id)
    // 切换账号后旧的区域覆盖值通常不再适用
    setRegion('')
  }, [])

  const account = useMemo(
    () => accounts.find((a) => a.id === currentId) ?? null,
    [accounts, currentId],
  )

  const value = useMemo(
    () => ({ accounts, account, selectAccount, region, setRegion, loading, error, reload }),
    [accounts, account, selectAccount, region, loading, error, reload],
  )

  return <AccountContext.Provider value={value}>{children}</AccountContext.Provider>
}

export function useAccount(): AccountContextValue {
  const ctx = useContext(AccountContext)
  if (!ctx) {
    throw new Error('useAccount 必须在 AccountProvider 内使用')
  }
  return ctx
}
