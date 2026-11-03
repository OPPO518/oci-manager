import { useEffect, useState, type ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { api } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useAccount } from '../lib/context'
import type { Region } from '../lib/types'

const NAV_ITEMS = [
  { to: '/', label: '总览' },
  { to: '/instances', label: '实例' },
  { to: '/grabber', label: 'ARM 抢购' },
  { to: '/cost', label: '成本' },
  { to: '/accounts', label: '账号' },
  { to: '/settings', label: '设置' },
]

// 这些页面即使还没有账号也必须可用，否则无法完成初始配置
const NO_ACCOUNT_REQUIRED = new Set(['/accounts', '/settings'])

export function Layout({ children }: { children: ReactNode }) {
  const { accounts, account, selectAccount, region, setRegion } = useAccount()
  const { username, logout } = useAuth()
  const { pathname } = useLocation()
  const [regions, setRegions] = useState<Region[]>([])

  useEffect(() => {
    if (!account) {
      setRegions([])
      return
    }
    let cancelled = false
    api.resources
      .regions(account.id)
      .then((list) => {
        if (!cancelled) setRegions(list)
      })
      .catch(() => {
        if (!cancelled) setRegions([])
      })
    return () => {
      cancelled = true
    }
  }, [account])

  const effectiveRegion = region || account?.region || ''

  return (
    <div className="flex min-h-full flex-col">
      <header className="bg-oracle-ink text-white">
        <div className="mx-auto flex h-12 max-w-7xl items-center gap-6 px-4">
          <div className="flex items-center gap-2 text-sm font-semibold">
            <span className="inline-block h-3.5 w-3.5 rounded-sm bg-oracle-red" />
            OCI 管理器
          </div>

          <nav className="flex items-center gap-1 text-sm">
            {NAV_ITEMS.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === '/'}
                className={({ isActive }) =>
                  `rounded px-3 py-1.5 transition-colors ${
                    isActive ? 'bg-white/10 text-white' : 'text-white/70 hover:bg-white/5 hover:text-white'
                  }`
                }
              >
                {item.label}
              </NavLink>
            ))}
          </nav>

          <div className="ml-auto flex items-center gap-2 text-sm">
            {username && <span className="hidden text-white/70 sm:inline">{username}</span>}
            <button
              type="button"
              className="rounded border border-white/20 bg-white/10 px-2 py-1 text-white/80 hover:bg-white/20"
              onClick={() => void logout()}
              title="退出登录"
            >
              退出
            </button>

            <select
              className="rounded border border-white/20 bg-white/10 px-2 py-1 text-white outline-none"
              value={region}
              onChange={(e) => setRegion(e.target.value)}
              title="区域"
            >
              <option value="">区域：{account?.region ?? '未选择'}</option>
              {regions.map((r) => (
                <option key={r.name} value={r.name} className="text-black">
                  {r.name}
                  {r.is_home_region ? '（主区域）' : ''}
                </option>
              ))}
            </select>

            <select
              className="max-w-[220px] rounded border border-white/20 bg-white/10 px-2 py-1 text-white outline-none"
              value={account?.id ?? ''}
              onChange={(e) => selectAccount(e.target.value)}
              title="账号"
            >
              {accounts.length === 0 && <option value="">暂无账号</option>}
              {accounts.map((a) => (
                <option key={a.id} value={a.id} className="text-black">
                  {a.name}
                  {a.proxy_url ? '（代理）' : ''}
                </option>
              ))}
            </select>
          </div>
        </div>
      </header>

      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6">
        {account || NO_ACCOUNT_REQUIRED.has(pathname) ? (
          children
        ) : (
          <p className="py-16 text-center text-sm text-oracle-muted">
            还没有配置账号，请先到「账号」页面添加。
          </p>
        )}
      </main>

      <footer className="border-t border-oracle-border py-3 text-center text-xs text-oracle-muted">
        当前区域：{effectiveRegion || '未选择'}
      </footer>
    </div>
  )
}
