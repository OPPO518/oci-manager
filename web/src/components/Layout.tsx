import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useAccount } from '../lib/context'
import type { Region } from '../lib/types'
import { Icon, Notice, type IconName } from './ui'

/* ------------------------------------------------------------------
   页头主操作插槽：页面通过 usePageAction(node) 把主操作放到页头右侧，
   不改变 App.tsx 的路由结构。
   ------------------------------------------------------------------ */
const PageActionContext = createContext<((node: ReactNode) => void) | null>(null)

export function usePageAction(node: ReactNode) {
  const setAction = useContext(PageActionContext)
  useEffect(() => {
    if (!setAction) return
    setAction(node)
    return () => setAction(null)
  }, [setAction, node])
}

type NavItem = { to: string; label: string; icon: IconName }

const NAV_GROUPS: { group: string; items: NavItem[] }[] = [
  {
    group: '概览',
    items: [{ to: '/', label: '总览', icon: 'overview' }],
  },
  {
    group: '计算',
    items: [
      { to: '/instances', label: '实例', icon: 'compute' },
      { to: '/grabber', label: 'ARM 抢购', icon: 'grabber' },
      { to: '/capacity', label: '容量探测', icon: 'capacity' },
    ],
  },
  {
    group: '成本与用量',
    items: [{ to: '/cost', label: '成本', icon: 'cost' }],
  },
  {
    group: '管理',
    items: [
      { to: '/accounts', label: '账号', icon: 'accounts' },
      { to: '/settings', label: '设置', icon: 'settings' },
    ],
  },
]

const PAGE_META: Record<string, { title: string; desc: string; group: string }> = {
  '/': {
    title: '总览',
    desc: '查看所选账号与区域下的资源概况、运行规格与本月消费。',
    group: '概览',
  },
  '/instances': {
    title: '实例',
    desc: '管理计算实例：启动、停止、重启、调整 Flex 规格或终止。',
    group: '计算',
  },
  '/grabber': {
    title: 'ARM 抢购',
    desc: '为 Ampere A1 资源创建自动抢购任务，仅在容量可用时发起创建请求。',
    group: '计算',
  },
  '/capacity': {
    title: '容量探测',
    desc: '通过 OCI 容量报告只读接口提前查询机型余量，减少无效的创建请求与风控风险。',
    group: '计算',
  },
  '/cost': {
    title: '成本',
    desc: '按服务与资源查看账期用量与消费明细。',
    group: '成本与用量',
  },
  '/accounts': {
    title: '账号',
    desc: '管理多个 OCI 账户的 API 凭证与专属出口代理。',
    group: '管理',
  },
  '/settings': {
    title: '设置',
    desc: '配置 Telegram 通知、两步验证、HTTPS 证书与管理员密码。',
    group: '管理',
  },
}

// 这些页面即使还没有账号也必须可用，否则无法完成初始配置
const NO_ACCOUNT_REQUIRED = new Set(['/accounts', '/settings'])

/* ------------------------------------------------------------------
   全局服务菜单 tray：条目全部映射到现有 7 条路由，不造不存在的路由。
   ------------------------------------------------------------------ */
const ALL_SERVICES: NavItem[] = [
  { to: '/', label: '总览', icon: 'overview' },
  { to: '/instances', label: '实例', icon: 'compute' },
  { to: '/grabber', label: 'ARM 抢购', icon: 'grabber' },
  { to: '/capacity', label: '容量探测', icon: 'capacity' },
  { to: '/cost', label: '成本与用量', icon: 'cost' },
  { to: '/accounts', label: '账号', icon: 'accounts' },
  { to: '/settings', label: '设置', icon: 'settings' },
]

const PINNED: NavItem[] = [ALL_SERVICES[0], ALL_SERVICES[1]]
const RECENT: NavItem[] = [ALL_SERVICES[4], ALL_SERVICES[5], ALL_SERVICES[3]]

const TRAY_GROUPS: { group: string; icon: IconName; items: NavItem[] }[] = [
  { group: 'Compute', icon: 'compute', items: [ALL_SERVICES[1], ALL_SERVICES[2], ALL_SERVICES[3]] },
  { group: 'Storage', icon: 'storage', items: [] },
  { group: 'Networking', icon: 'network', items: [] },
  { group: 'Database', icon: 'database', items: [] },
  { group: 'Identity & Security', icon: 'shield', items: [ALL_SERVICES[5], ALL_SERVICES[6]] },
  { group: 'Observability & Management', icon: 'eye', items: [ALL_SERVICES[4]] },
]

export function Layout({ children }: { children: ReactNode }) {
  const { accounts, account, selectAccount, region, setRegion } = useAccount()
  const { username, logout } = useAuth()
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const [regions, setRegions] = useState<Region[]>([])
  const [trayOpen, setTrayOpen] = useState(false)
  const [trayQuery, setTrayQuery] = useState('')
  const [globalSearch, setGlobalSearch] = useState('')
  const [userMenu, setUserMenu] = useState(false)
  const [action, setAction] = useState<ReactNode>(null)
  const setActionStable = useCallback((node: ReactNode) => setAction(node), [])

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

  // 明文 HTTP 且非本机访问时，禁止录入真实凭证的醒目警告
  const [insecure] = useState(() => {
    if (typeof window === 'undefined') return false
    const host = window.location.hostname
    const local = host === 'localhost' || host === '127.0.0.1' || host === '::1'
    return window.location.protocol === 'http:' && !local
  })

  const effectiveRegion = region || account?.region || ''
  const meta = PAGE_META[pathname] ?? PAGE_META['/']
  const needAccount = !account && !NO_ACCOUNT_REQUIRED.has(pathname)

  const trayQueryTrimmed = trayQuery.trim().toLowerCase()
  const searchResults = trayQueryTrimmed
    ? ALL_SERVICES.filter(
        (s) => s.label.toLowerCase().includes(trayQueryTrimmed) || s.to.includes(trayQueryTrimmed),
      )
    : []

  const renderTrayLink = (item: NavItem) => (
    <NavLink
      key={item.to}
      to={item.to}
      end={item.to === '/'}
      className="oci-tray-link"
      onClick={() => setTrayOpen(false)}
    >
      <Icon name={item.icon} size={16} />
      <span>{item.label}</span>
    </NavLink>
  )

  return (
    <PageActionContext.Provider value={setActionStable}>
      <div className="min-h-full">
        {/* ---------- 全局顶栏（56px） ---------- */}
        <header className="oci-topbar">
          <button
            type="button"
            className="oci-topbar-icon"
            onClick={() => setTrayOpen((v) => !v)}
            title="打开全局服务菜单"
            aria-label="打开全局服务菜单"
            aria-expanded={trayOpen}
          >
            <Icon name="menu" size={18} />
          </button>

          <div className="oci-brand">
            <span className="oci-wordmark">ORACLE</span>
            <span className="oci-brand-divider" />
            <span className="oci-brand-label">Oracle Cloud</span>
          </div>
          <span className="hidden text-[13px] text-white/70 lg:inline">OCI 管理器</span>

          {/* 真输入框：界面演示，不假装能全局检索、不抛错 */}
          <div className="oci-topbar-search ml-2 hidden lg:flex">
            <span className="oci-topbar-search-icon">
              <Icon name="search" size={14} />
            </span>
            <input
              value={globalSearch}
              onChange={(e) => setGlobalSearch(e.target.value)}
              placeholder="Search resources, services, and documentation"
              aria-label="搜索资源、服务和文档（界面演示，暂不支持全局检索）"
            />
          </div>

          <div className="ml-auto flex items-center gap-1.5">
            <div className="hidden items-center gap-1.5 md:flex">
              <Icon name="globe" size={14} />
              <select
                className="oci-topbar-select"
                value={region}
                onChange={(e) => setRegion(e.target.value)}
                title="区域"
                aria-label="区域"
              >
                <option value="">区域：{account?.region ?? '未选择'}</option>
                {regions.map((r) => (
                  <option key={r.name} value={r.name}>
                    {r.name}
                    {r.is_home_region ? '（主区域）' : ''}
                  </option>
                ))}
              </select>
            </div>

            <select
              className="oci-topbar-select hidden max-w-[190px] sm:block"
              value={account?.id ?? ''}
              onChange={(e) => selectAccount(e.target.value)}
              title="账号"
              aria-label="账号"
            >
              {accounts.length === 0 && <option value="">暂无账号</option>}
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                  {a.proxy_url ? '（代理）' : ''}
                </option>
              ))}
            </select>

            <span className="mx-1 hidden h-6 w-px bg-white/20 md:block" />

            {/* OCI 图标序列：展示性占位，hover 半透明白 */}
            <button
              type="button"
              className="oci-topbar-icon hidden md:inline-flex"
              title="Announcements（界面演示）"
              aria-label="Announcements（界面演示）"
            >
              <Icon name="announcement" size={18} />
            </button>
            <button
              type="button"
              className="oci-topbar-icon hidden sm:inline-flex"
              title="Help（界面演示）"
              aria-label="Help（界面演示）"
            >
              <Icon name="help" size={18} />
            </button>
            <button
              type="button"
              className="oci-topbar-icon hidden sm:inline-flex"
              title="Cloud Shell（界面演示）"
              aria-label="Cloud Shell（界面演示）"
            >
              <Icon name="terminal" size={18} />
            </button>
            <button
              type="button"
              className="oci-topbar-icon"
              title="Notifications（界面演示）"
              aria-label="Notifications（界面演示）"
            >
              <Icon name="bell" size={18} />
            </button>

            {/* Profile 头像下拉：保留设置与退出登录 */}
            <div className="relative">
              <button
                type="button"
                className="oci-user-trigger"
                onClick={() => setUserMenu((v) => !v)}
                title="用户菜单"
                aria-label="用户菜单"
                aria-expanded={userMenu}
              >
                <span className="oci-avatar">
                  {username ? username.slice(0, 1).toUpperCase() : 'U'}
                </span>
                <span className="hidden text-[13px] text-white/90 sm:inline">{username}</span>
                <Icon name="chevron" size={12} />
              </button>
              {userMenu && (
                <div
                  className="oci-user-menu"
                  onMouseLeave={() => setUserMenu(false)}
                >
                  <div className="px-3.5 py-2 text-[12px] text-oracle-muted">
                    当前用户：{username || '—'}
                  </div>
                  <div className="oci-user-menu-sep" />
                  <button
                    type="button"
                    className="oci-user-menu-item"
                    onClick={() => {
                      setUserMenu(false)
                      navigate('/settings')
                    }}
                  >
                    <Icon name="settings" size={15} />
                    设置
                  </button>
                  <button
                    type="button"
                    className="oci-user-menu-item is-danger"
                    onClick={() => {
                      setUserMenu(false)
                      void logout()
                    }}
                  >
                    <Icon name="logout" size={15} />
                    退出登录
                  </button>
                </div>
              )}
            </div>
          </div>
        </header>

        {/* ---------- 全局服务菜单 tray ---------- */}
        {trayOpen && (
          <div className="oci-tray-overlay">
            <div className="oci-tray-backdrop" onClick={() => setTrayOpen(false)} />
            <div className="oci-tray">
              <div className="oci-tray-head">
                <div className="oci-tray-title">Global navigation</div>
                <div className="relative">
                  <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-oracle-muted">
                    <Icon name="search" size={14} />
                  </span>
                  <input
                    className="oci-input pl-9"
                    placeholder="Search services"
                    aria-label="搜索服务"
                    value={trayQuery}
                    onChange={(e) => setTrayQuery(e.target.value)}
                    autoFocus
                  />
                </div>
              </div>
              <div className="oci-tray-body">
                {trayQueryTrimmed ? (
                  <>
                    <div className="oci-tray-section-title">Search results</div>
                    {searchResults.length > 0 ? (
                      searchResults.map(renderTrayLink)
                    ) : (
                      <span className="oci-tray-empty">No services found.</span>
                    )}
                  </>
                ) : (
                  <>
                    <div className="oci-tray-section-title">Pinned</div>
                    {PINNED.map(renderTrayLink)}
                    <div className="oci-tray-section-title">Recently visited</div>
                    {RECENT.map(renderTrayLink)}
                    {TRAY_GROUPS.map((g) => (
                      <div key={g.group}>
                        <div className="oci-tray-section-title">
                          <span className="inline-flex items-center gap-2">
                            <Icon name={g.icon} size={14} />
                            {g.group}
                          </span>
                        </div>
                        {g.items.length > 0 ? (
                          g.items.map(renderTrayLink)
                        ) : (
                          <span className="oci-tray-empty">本演示未接入该服务</span>
                        )}
                      </div>
                    ))}
                  </>
                )}
              </div>
            </div>
          </div>
        )}

        <div className="flex min-h-[calc(100vh-var(--rw-topbar-h))]">
          {/* ---------- 服务页内持久侧栏（248px） ---------- */}
          <aside className="oci-sidebar">
            <div className="oci-sidebar-title">服务导航</div>
            {NAV_GROUPS.map((section) => (
              <div key={section.group}>
                <div className="oci-sidebar-title">{section.group}</div>
                {section.items.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={item.to === '/'}
                    className={({ isActive }) => `oci-nav-item ${isActive ? 'active' : ''}`}
                  >
                    <Icon name={item.icon} />
                    <span>{item.label}</span>
                  </NavLink>
                ))}
              </div>
            ))}
            <div className="mt-auto border-t border-oracle-border px-4 py-3 text-[12px] leading-relaxed text-oracle-muted">
              OCI 管理器
              <br />
              当前区域：{effectiveRegion || '未选择'}
            </div>
          </aside>

          {/* ---------- 内容区 ---------- */}
          <main className="oci-content">
            <nav className="oci-breadcrumb">
              <Link to="/">Oracle Cloud</Link>
              <span>/</span>
              <span>{meta.group}</span>
              <span>/</span>
              <span className="font-medium text-oracle-ink">{meta.title}</span>
            </nav>

            {/* 明文 HTTP 安全提示 */}
            {insecure && (
              <div className="oci-security-banner">
                <Icon name="warning" />
                <div>
                  当前为明文 HTTP 连接，请勿录入真实 OCI 凭证；请先在「设置 → HTTPS 证书」启用 HTTPS。
                </div>
              </div>
            )}

            <div className="oci-page-header">
              <div>
                <h1 className="oci-page-title">{meta.title}</h1>
                <p className="oci-page-desc">{meta.desc}</p>
              </div>
              {action && <div className="oci-page-actions">{action}</div>}
            </div>

            {needAccount ? (
              <Notice tone="info">
                <div className="flex flex-wrap items-center justify-between gap-4">
                  <span>还没有配置 OCI 账号。请先添加 API 凭证，之后即可查看资源与执行操作。</span>
                  <button
                    type="button"
                    className="oci-btn-secondary shrink-0"
                    onClick={() => navigate('/accounts')}
                  >
                    前往账号设置
                  </button>
                </div>
              </Notice>
            ) : (
              children
            )}
          </main>
        </div>
      </div>
    </PageActionContext.Provider>
  )
}
