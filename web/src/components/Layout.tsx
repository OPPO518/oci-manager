import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { NavLink, useLocation, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useAccount } from '../lib/context'
import type { Region } from '../lib/types'
import { AnchoredMenu } from './AnchoredMenu'
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

type NavLinkItem = { label: string; to: string; icon?: IconName }
type NavSection = { title?: string; items: NavLinkItem[] }
type ServiceNav = {
  name: string
  icon: IconName
  sections: NavSection[]
}

/* ------------------------------------------------------------------
   二级服务侧栏：完全对齐官网「服务名 → 返回 → 分组 → 纯文字菜单」结构。
   每一项都指向真实存在的路由，不放置点了没反应的占位菜单。
   主页按官网惯例不显示侧栏（全宽卡片布局）。
   ------------------------------------------------------------------ */
const COMPUTE_NAV: ServiceNav = {
  name: '计算',
  icon: 'compute',
  sections: [
    { title: '概览', items: [{ label: '实例', to: '/instances' }] },
    {
      title: '资源供应',
      items: [
        { label: 'ARM 抢购', to: '/grabber' },
        { label: '容量探测', to: '/capacity' },
        { label: '开机任务', to: '/power-tasks', icon: 'power' },
      ],
    },
    {
      title: '相关服务',
      items: [
        { label: '虚拟云网络', to: '/network' },
        { label: '成本与用量', to: '/cost' },
      ],
    },
  ],
}

const NETWORK_NAV: ServiceNav = {
  name: '网络',
  icon: 'network',
  sections: [
    {
      title: '概览',
      items: [{ label: '虚拟云网络', to: '/network' }],
    },
    {
      title: '网络资源',
      items: [
        { label: '实例', to: '/instances' },
        { label: '身份与安全', to: '/identity' },
      ],
    },
    { title: '相关服务', items: [{ label: '成本与用量', to: '/cost' }] },
  ],
}

const BILLING_NAV: ServiceNav = {
  name: '计费和成本管理',
  icon: 'cost',
  sections: [
    { title: '概览', items: [{ label: '成本与用量', to: '/cost' }] },
    {
      title: '管理',
      items: [
        { label: '账号', to: '/accounts' },
        { label: '主页', to: '/' },
      ],
    },
  ],
}

const IDENTITY_NAV: ServiceNav = {
  name: '身份和安全',
  icon: 'shield',
  sections: [
    {
      title: '概览',
      items: [{ label: '身份与权限', to: '/identity' }],
    },
    {
      title: '管理',
      items: [
        { label: '账号', to: '/accounts' },
        { label: '设置', to: '/settings' },
      ],
    },
  ],
}

const ADMIN_NAV: ServiceNav = {
  name: '管理',
  icon: 'settings',
  sections: [
    {
      title: '面板设置',
      items: [
        { label: '账号', to: '/accounts' },
        { label: '设置', to: '/settings' },
        { label: '日志查看', to: '/logs', icon: 'logs' },
      ],
    },
    {
      title: '相关服务',
      items: [
        { label: '身份与权限', to: '/identity' },
        { label: '主页', to: '/' },
      ],
    },
  ],
}

const SERVICE_NAV: Record<string, ServiceNav> = {
  '/instances': COMPUTE_NAV,
  '/grabber': COMPUTE_NAV,
  '/capacity': COMPUTE_NAV,
  '/power-tasks': COMPUTE_NAV,
  '/network': NETWORK_NAV,
  '/cost': BILLING_NAV,
  '/identity': IDENTITY_NAV,
  '/accounts': ADMIN_NAV,
  '/settings': ADMIN_NAV,
  '/logs': ADMIN_NAV,
}

type PageMeta = {
  title: string
  lead: string
  desc: string
  home?: boolean
}

/* 说明文字的蓝色开头词与正文分开，复刻官网「实例是计算主机…」的写法 */
const PAGE_META: Record<string, PageMeta> = {
  '/': {
    title: '主页',
    lead: '',
    desc: '',
    home: true,
  },
  '/instances': {
    title: '实例',
    lead: '实例',
    desc: '是计算主机。可在此启动、停止、重启、调整规格或终止虚拟机，并查看其网络与存储资源。',
  },
  '/grabber': {
    title: 'ARM 抢购',
    lead: '抢购任务',
    desc: '会在容量可用时才发起创建请求，用退避重试降低无效调用与风控风险。',
  },
  '/capacity': {
    title: '容量探测',
    lead: '容量报告',
    desc: '是只读接口，用于提前查询机型余量，不会创建实例，也不占用配额。',
  },
  '/cost': {
    title: '成本与用量',
    lead: '成本分析',
    desc: '按日期与服务列出账期用量，并展示预计账单与订阅信息。',
  },
  '/accounts': {
    title: '账号',
    lead: 'OCI 账号',
    desc: '用于保存 API 凭证与专属出口代理，凭证加密存放，不会回显私钥。',
  },
  '/settings': {
    title: '设置',
    lead: '面板设置',
    desc: '包含通知、两步验证、HTTPS 证书、审计日志与演示模式等选项。',
  },
  '/network': {
    title: '虚拟云网络',
    lead: '虚拟云网络 (VCN)',
    desc: '是您在 Oracle Cloud Infrastructure 中设置的专用虚拟网络。可管理 VNIC、引导卷、公网 IP 与安全列表。',
  },
  '/identity': {
    title: '身份与权限',
    lead: '身份与安全',
    desc: '管理 IAM 用户、组、成员关系与 API Key；所有写操作均需二次验证。',
  },
  '/power-tasks': {
    title: '开机任务',
    lead: '定时开关机',
    desc: '按计划自动启动、停止或重启实例，支持按周重复与多实例批量执行。',
  },
  '/logs': {
    title: '日志查看',
    lead: '实时日志流',
    desc: '汇聚管理面板运行、抢购任务与敏感操作的实时日志，便于排障与审计。',
  },
}

// 这些页面即使还没有账号也必须可用，否则无法完成初始配置
const NO_ACCOUNT_REQUIRED = new Set(['/accounts', '/settings', '/identity'])

/* 全局服务菜单：条目全部映射到现有路由，不造不存在的路由 */
const ALL_SERVICES: NavLinkItem[] = [
  { to: '/', label: '主页', icon: 'overview' },
  { to: '/instances', label: '实例', icon: 'compute' },
  { to: '/grabber', label: 'ARM 抢购', icon: 'grabber' },
  { to: '/capacity', label: '容量探测', icon: 'capacity' },
  { to: '/power-tasks', label: '开机任务', icon: 'power' },
  { to: '/cost', label: '成本与用量', icon: 'cost' },
  { to: '/network', label: '虚拟云网络', icon: 'network' },
  { to: '/identity', label: '身份与权限', icon: 'shield' },
  { to: '/accounts', label: '账号', icon: 'accounts' },
  { to: '/settings', label: '设置', icon: 'settings' },
  { to: '/logs', label: '日志查看', icon: 'logs' },
]

const PINNED = [ALL_SERVICES[0], ALL_SERVICES[1]]
const RECENT = [ALL_SERVICES[5], ALL_SERVICES[8], ALL_SERVICES[3]]

const TRAY_GROUPS: { group: string; icon: IconName; items: NavLinkItem[] }[] = [
  { group: '计算', icon: 'compute', items: ALL_SERVICES.slice(1, 5) },
  { group: '网络与存储', icon: 'network', items: [ALL_SERVICES[6]] },
  { group: '计费和成本管理', icon: 'cost', items: [ALL_SERVICES[5]] },
  { group: '身份和安全', icon: 'shield', items: [ALL_SERVICES[7]] },
  { group: '管理', icon: 'settings', items: [ALL_SERVICES[8], ALL_SERVICES[9], ALL_SERVICES[10]] },
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
  const [statusDismissed, setStatusDismissed] = useState(false)
  const [action, setAction] = useState<ReactNode>(null)
  const userBtnRef = useRef<HTMLButtonElement>(null)
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

  const meta = PAGE_META[pathname] ?? PAGE_META['/']
  const service = SERVICE_NAV[pathname]
  const needAccount = !account && !NO_ACCOUNT_REQUIRED.has(pathname)

  const query = (trayQuery || globalSearch).trim().toLowerCase()
  const searchResults = query
    ? ALL_SERVICES.filter(
        (s) => s.label.toLowerCase().includes(query) || s.to.includes(query),
      )
    : []

  const renderTrayLink = (item: NavLinkItem) => (
    <NavLink
      key={item.to}
      to={item.to}
      end={item.to === '/'}
      className="oci-tray-link"
      onClick={() => {
        setTrayOpen(false)
        setTrayQuery('')
      }}
    >
      <Icon name={item.icon ?? 'overview'} size={16} />
      <span>{item.label}</span>
    </NavLink>
  )

  const statusMessages: string[] = []
  if (insecure) {
    statusMessages.push('当前为明文 HTTP 连接，请勿录入真实 OCI 凭证；请先在「设置 → HTTPS 证书」启用 HTTPS。')
  }
  if (account?.is_demo) {
    statusMessages.push('演示账号：所有数据均为模拟，不会发出任何真实 OCI 请求，可放心试用全部功能。')
  }

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
            <span className="oci-brand-label">Cloud</span>
          </div>

          {/* 全局搜索：输入即打开服务菜单并过滤，回车跳第一个结果 */}
          <div className="oci-topbar-search ml-2 hidden lg:flex">
            <span className="oci-topbar-search-icon">
              <Icon name="search" size={14} />
            </span>
            <input
              value={globalSearch}
              onChange={(e) => {
                setGlobalSearch(e.target.value)
                setTrayQuery(e.target.value)
              }}
              onFocus={() => setTrayOpen(true)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && searchResults.length > 0) {
                  navigate(searchResults[0].to)
                  setTrayOpen(false)
                  setGlobalSearch('')
                  setTrayQuery('')
                }
                if (e.key === 'Escape') setTrayOpen(false)
              }}
              placeholder="搜索资源、服务、文档和市场"
              aria-label="搜索资源、服务与页面"
            />
          </div>

          <div className="ml-auto flex items-center gap-1.5">
            <div className="hidden items-center gap-1.5 lg:flex">
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
              className="oci-topbar-select hidden max-w-[210px] sm:block"
              value={account?.id ?? ''}
              onChange={(e) => selectAccount(e.target.value)}
              title="账号"
              aria-label="账号"
            >
              {accounts.length === 0 && <option value="">暂无账号</option>}
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                  {a.is_demo ? '（演示）' : ''}
                  {a.proxy_url ? '（代理）' : ''}
                </option>
              ))}
            </select>

            <span className="mx-1 hidden h-6 w-px bg-white/20 md:block" />

            {/* 明确标注「暂未接入」并禁用，不假装可点 */}
            {[
              { icon: 'announcement' as IconName, title: '公告（暂未接入）' },
              { icon: 'help' as IconName, title: '帮助（暂未接入）' },
              { icon: 'terminal' as IconName, title: 'Cloud Shell（暂未接入）' },
              { icon: 'bell' as IconName, title: '通知（暂未接入）' },
            ].map((item) => (
              <button
                key={item.title}
                type="button"
                className="oci-topbar-icon hidden opacity-45 md:inline-flex"
                title={item.title}
                aria-label={item.title}
                disabled
              >
                <Icon name={item.icon} size={18} />
              </button>
            ))}

            {/* 用户菜单同样用 portal，避免顶栏容器裁剪 */}
            <button
              ref={userBtnRef}
              type="button"
              className="oci-user-trigger"
              onClick={() => setUserMenu((v) => !v)}
              title="用户菜单"
              aria-label="用户菜单"
              aria-haspopup="menu"
              aria-expanded={userMenu}
            >
              <span className="oci-avatar">
                {username ? username.slice(0, 1).toUpperCase() : 'U'}
              </span>
              <span className="hidden text-[13px] text-white/90 sm:inline">{username}</span>
              <Icon name="chevron" size={12} />
            </button>
            <AnchoredMenu open={userMenu} anchorRef={userBtnRef} onClose={() => setUserMenu(false)} className="oci-user-menu">
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
            </AnchoredMenu>
          </div>
        </header>

        {/* ---------- 全局服务菜单 tray ---------- */}
        {trayOpen && (
          <div className="oci-tray-overlay">
            <div className="oci-tray-backdrop" onClick={() => setTrayOpen(false)} />
            <div className="oci-tray">
              <div className="oci-tray-head">
                <div className="oci-tray-title">全局导航</div>
                <div className="relative">
                  <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-oracle-muted">
                    <Icon name="search" size={14} />
                  </span>
                  <input
                    className="oci-input pl-9"
                    placeholder="搜索服务"
                    aria-label="搜索服务"
                    value={trayQuery}
                    onChange={(e) => setTrayQuery(e.target.value)}
                    autoFocus
                  />
                </div>
              </div>
              <div className="oci-tray-body">
                {query ? (
                  <>
                    <div className="oci-tray-section-title">搜索结果</div>
                    {searchResults.length > 0 ? (
                      searchResults.map(renderTrayLink)
                    ) : (
                      <span className="oci-tray-empty">未找到匹配的服务。</span>
                    )}
                  </>
                ) : (
                  <>
                    <div className="oci-tray-section-title">固定</div>
                    {PINNED.map(renderTrayLink)}
                    <div className="oci-tray-section-title">最近访问</div>
                    {RECENT.map(renderTrayLink)}
                    {TRAY_GROUPS.map((g) => (
                      <div key={g.group}>
                        <div className="oci-tray-section-title">
                          <span className="inline-flex items-center gap-2">
                            <Icon name={g.icon} size={14} />
                            {g.group}
                          </span>
                        </div>
                        {g.items.map(renderTrayLink)}
                      </div>
                    ))}
                  </>
                )}
              </div>
            </div>
          </div>
        )}

        <div className="flex min-h-[calc(100vh-var(--rw-topbar-h))]">
          {/* ---------- 二级服务侧栏（主页不显示，与官网一致） ---------- */}
          {service && (
            <aside className="oci-sidebar">
              <div className="oci-sidebar-service">
                <Icon name={service.icon} size={17} />
                {service.name}
              </div>
              <NavLink to="/" className="oci-sidebar-back">
                <Icon name="chevron-left" size={13} />
                主页
              </NavLink>
              {service.sections.map((section, i) => (
                <div key={section.title ?? i}>
                  {section.title && <div className="oci-sidebar-section">{section.title}</div>}
                  {section.items.map((item) => (
                    <NavLink
                      key={item.to}
                      to={item.to}
                      end={item.to === '/'}
                      className={({ isActive }) => `oci-nav-item ${isActive ? 'active' : ''}`}
                    >
                      <span>{item.label}</span>
                    </NavLink>
                  ))}
                </div>
              ))}
              <div className="oci-sidebar-footer">
                <button
                  type="button"
                  className="oci-btn-text"
                  onClick={() => setTrayOpen(true)}
                  title="打开全局服务菜单"
                >
                  浏览所有服务
                </button>
              </div>
            </aside>
          )}

          {/* ---------- 内容区 ---------- */}
          <main className="oci-content">
            {meta.home ? (
              <div className="oci-home-head">
                <div>
                  <div className="oci-home-title">
                    <span className="oci-home-app-icon">
                      <Icon name="home" size={20} />
                    </span>
                    {meta.title}
                  </div>
                  <div className="oci-home-tenant">
                    租户：{account?.name ?? '未选择账号'}
                  </div>
                </div>
                <div className="flex items-center gap-4">
                  <div className="oci-home-health">
                    <Icon name="success" size={16} />
                    <span>正常性能</span>
                    <span className="oci-link">查看服务健康状况</span>
                  </div>
                  {action && <div className="oci-page-actions">{action}</div>}
                </div>
              </div>
            ) : (
              <div className="oci-page-header">
                <div>
                  <h1 className="oci-page-title">{meta.title}</h1>
                  <p className="oci-page-desc">
                    {meta.lead && <span className="oci-link">{meta.lead}</span>}
                    {meta.desc}
                  </p>
                </div>
                {action && <div className="oci-page-actions">{action}</div>}
              </div>
            )}

            {/* 合并后的单条可收起状态条：HTTP 警告 + 演示提示不再各占一行 */}
            {!statusDismissed && statusMessages.length > 0 && (
              <div className={`oci-status-strip ${insecure ? 'is-warning' : ''}`}>
                <Icon name={insecure ? 'warning' : 'info'} size={16} />
                <div className="oci-status-main">
                  {statusMessages.map((msg, i) => (
                    <span key={msg}>
                      {i > 0 && <span className="oci-status-sep" />}
                      {msg}
                    </span>
                  ))}
                </div>
                <button
                  type="button"
                  className="oci-status-close"
                  onClick={() => setStatusDismissed(true)}
                  title="收起提示"
                  aria-label="收起提示"
                >
                  ×
                </button>
              </div>
            )}

            <div className="oci-accent-bar" />

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

            <footer className="oci-footer">
              <span>使用条款和条件 · 隐私 · Cookie 偏好设置</span>
              <span>OCI 管理器 · 自托管面板 · 当前区域 {region || account?.region || '未选择'}</span>
            </footer>
          </main>
        </div>
      </div>
    </PageActionContext.Provider>
  )
}
