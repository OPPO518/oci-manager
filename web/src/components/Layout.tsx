import { useEffect, useState, type ReactNode } from 'react'
import { NavLink, useLocation, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useAccount } from '../lib/context'
import type { Region } from '../lib/types'

type IconName =
  | 'overview'
  | 'compute'
  | 'grabber'
  | 'capacity'
  | 'cost'
  | 'accounts'
  | 'settings'
  | 'menu'
  | 'search'
  | 'globe'
  | 'help'
  | 'bell'
  | 'chevron'
  | 'logout'

/** 简化的 Redwood 线性图标，统一 16px。 */
function Icon({ name, size = 16 }: { name: IconName; size?: number }) {
  const paths: Record<IconName, ReactNode> = {
    overview: (
      <>
        <rect x="2" y="2" width="5" height="5" rx="1" />
        <rect x="9" y="2" width="5" height="5" rx="1" />
        <rect x="2" y="9" width="5" height="5" rx="1" />
        <rect x="9" y="9" width="5" height="5" rx="1" />
      </>
    ),
    compute: (
      <>
        <rect x="2" y="3" width="12" height="4" rx="1" />
        <rect x="2" y="9" width="12" height="4" rx="1" />
        <circle cx="4.5" cy="5" r="0.7" fill="currentColor" stroke="none" />
        <circle cx="4.5" cy="11" r="0.7" fill="currentColor" stroke="none" />
      </>
    ),
    grabber: (
      <>
        <path d="M8 1.5v3M8 11.5v3M1.5 8h3M11.5 8h3" />
        <circle cx="8" cy="8" r="3" />
        <circle cx="8" cy="8" r="0.8" fill="currentColor" stroke="none" />
      </>
    ),
    capacity: (
      <>
        <path d="M2 12V7M6 12V4M10 12V9M14 12V2" />
        <path d="M1 14h14" />
      </>
    ),
    cost: (
      <>
        <circle cx="8" cy="8" r="6" />
        <path d="M10.5 5.5c-.6-.7-1.5-1-2.5-1-1.4 0-2.5.7-2.5 1.8 0 2.5 5 1.2 5 3.7 0 1.1-1.1 1.9-2.5 1.9-1 0-1.9-.3-2.5-1" />
        <path d="M8 3v10" />
      </>
    ),
    accounts: (
      <>
        <circle cx="8" cy="5.5" r="2.5" />
        <path d="M2.5 14c0-3 2.5-4.5 5.5-4.5s5.5 1.5 5.5 4.5" />
      </>
    ),
    settings: (
      <>
        <circle cx="8" cy="8" r="2.5" />
        <path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.4 1.4M11.2 11.2l1.4 1.4M12.6 3.4l-1.4 1.4M4.8 11.2l-1.4 1.4" />
      </>
    ),
    menu: <path d="M2 4h12M2 8h12M2 12h12" />,
    search: (
      <>
        <circle cx="7" cy="7" r="4.5" />
        <path d="M10.5 10.5L14 14" />
      </>
    ),
    globe: (
      <>
        <circle cx="8" cy="8" r="6" />
        <path d="M2 8h12M8 2c2 2 2 10 0 12M8 2c-2 2-2 10 0 12" />
      </>
    ),
    help: (
      <>
        <circle cx="8" cy="8" r="6" />
        <path d="M6.3 6.2a1.8 1.8 0 013.4.7c0 1.4-1.7 1.6-1.7 3" />
        <circle cx="8" cy="11.7" r="0.7" fill="currentColor" stroke="none" />
      </>
    ),
    bell: (
      <>
        <path d="M4 6a4 4 0 018 0c0 3 1 4 1 4H3s1-1 1-4" />
        <path d="M6.5 12.5a1.5 1.5 0 003 0" />
      </>
    ),
    chevron: <path d="M4 6l4 4 4-4" />,
    logout: (
      <>
        <path d="M6 2H3v12h3" />
        <path d="M9 5l3 3-3 3M12 8H6" />
      </>
    ),
  }
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.3"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[name]}
    </svg>
  )
}

const NAV_GROUPS: {
  group: string
  items: { to: string; label: string; icon: IconName }[]
}[] = [
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

export function Layout({ children }: { children: ReactNode }) {
  const { accounts, account, selectAccount, region, setRegion } = useAccount()
  const { username, logout } = useAuth()
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const [regions, setRegions] = useState<Region[]>([])
  const [collapsed, setCollapsed] = useState(false)
  const [userMenu, setUserMenu] = useState(false)

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
  const meta = PAGE_META[pathname] ?? PAGE_META['/']
  const needAccount = !account && !NO_ACCOUNT_REQUIRED.has(pathname)

  return (
    <div className="min-h-full">
      {/* ---------- 全局顶栏 ---------- */}
      <header className="oci-topbar">
        <button
          type="button"
          className="oci-topbar-icon"
          onClick={() => setCollapsed((v) => !v)}
          title={collapsed ? '展开导航' : '收起导航'}
        >
          <Icon name="menu" size={18} />
        </button>

        <div className="oci-brand">
          <span className="oci-brand-mark" />
          <span>Oracle Cloud</span>
        </div>
        <span className="oci-brand-divider" />
        <span className="hidden text-[13px] text-white/70 sm:inline">OCI 管理器</span>

        <div className="oci-topbar-search ml-2 hidden lg:flex">
          <Icon name="search" size={14} />
          <span>搜索资源、服务和文档</span>
        </div>

        <div className="ml-auto flex items-center gap-2">
          <div className="hidden items-center gap-1.5 md:flex">
            <Icon name="globe" size={14} />
            <select
              className="oci-topbar-select"
              value={region}
              onChange={(e) => setRegion(e.target.value)}
              title="区域"
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
          >
            {accounts.length === 0 && <option value="">暂无账号</option>}
            {accounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
                {a.proxy_url ? '（代理）' : ''}
              </option>
            ))}
          </select>

          <button type="button" className="oci-topbar-icon hidden sm:inline-flex" title="帮助">
            <Icon name="help" size={16} />
          </button>
          <button type="button" className="oci-topbar-icon hidden sm:inline-flex" title="通知">
            <Icon name="bell" size={16} />
          </button>

          <div className="relative">
            <button
              type="button"
              className="flex items-center gap-1.5 rounded px-1.5 py-1 hover:bg-white/10"
              onClick={() => setUserMenu((v) => !v)}
              title="用户菜单"
            >
              <span className="oci-avatar">{username ? username.slice(0, 1).toUpperCase() : 'U'}</span>
              <span className="hidden text-xs text-white/85 sm:inline">{username}</span>
              <Icon name="chevron" size={12} />
            </button>
            {userMenu && (
              <div
                className="absolute right-0 top-10 z-50 w-44 overflow-hidden rounded border border-oracle-border bg-white py-1 shadow-lg"
                onMouseLeave={() => setUserMenu(false)}
              >
                <button
                  type="button"
                  className="block w-full px-3 py-2 text-left text-[13px] hover:bg-oracle-surface"
                  onClick={() => {
                    setUserMenu(false)
                    navigate('/settings')
                  }}
                >
                  设置
                </button>
                <button
                  type="button"
                  className="flex w-full items-center gap-2 px-3 py-2 text-left text-[13px] text-oracle-redHover hover:bg-oracle-redSoft"
                  onClick={() => {
                    setUserMenu(false)
                    void logout()
                  }}
                >
                  <Icon name="logout" size={14} />
                  退出登录
                </button>
              </div>
            )}
          </div>
        </div>
      </header>

      <div className="flex min-h-[calc(100vh-48px)]">
        {/* ---------- 侧栏 ---------- */}
        <aside className={`oci-sidebar ${collapsed ? '!w-[56px]' : ''}`}>
          {NAV_GROUPS.map((section) => (
            <div key={section.group}>
              {!collapsed && <div className="oci-sidebar-title">{section.group}</div>}
              {section.items.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.to === '/'}
                  title={collapsed ? item.label : undefined}
                  className={({ isActive }) =>
                    `oci-nav-item ${isActive ? 'active' : ''} ${collapsed ? 'justify-center !px-0' : ''}`
                  }
                >
                  <Icon name={item.icon} />
                  {!collapsed && <span>{item.label}</span>}
                </NavLink>
              ))}
            </div>
          ))}
          {!collapsed && (
            <div className="mt-auto border-t border-oracle-border px-5 py-3 text-[11px] leading-relaxed text-oracle-muted">
              OCI 管理器
              <br />
              当前区域：{effectiveRegion || '未选择'}
            </div>
          )}
        </aside>

        {/* ---------- 内容区 ---------- */}
        <main className="oci-content">
          <nav className="oci-breadcrumb">
            <span>Oracle Cloud</span>
            <span className="text-oracle-borderStrong">/</span>
            <span>{meta.group}</span>
            <span className="text-oracle-borderStrong">/</span>
            <span className="font-medium text-oracle-ink">{meta.title}</span>
          </nav>

          <div className="oci-page-header">
            <div>
              <h1 className="oci-page-title">{meta.title}</h1>
              <p className="oci-page-desc">{meta.desc}</p>
            </div>
          </div>

          {needAccount ? (
            <div className="oci-notice oci-notice-info flex items-center justify-between gap-4">
              <span>还没有配置 OCI 账号。请先添加 API 凭证，之后即可查看资源与执行操作。</span>
              <button
                type="button"
                className="oci-btn-secondary shrink-0"
                onClick={() => navigate('/accounts')}
              >
                前往账号设置
              </button>
            </div>
          ) : (
            children
          )}
        </main>
      </div>
    </div>
  )
}
