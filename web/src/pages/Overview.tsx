import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { AnchoredMenu } from '../components/AnchoredMenu'
import { usePageAction } from '../components/Layout'
import {
  Badge,
  Card,
  CardDots,
  ErrorBanner,
  Icon,
  Loading,
  Stat,
  type CardAction,
  type IconName,
} from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { formatDateTime, formatMoney } from '../lib/format'
import type { Overview as OverviewData } from '../lib/types'

type ResourceRow = { name: string; type: string; state: string; note: string; to: string }

const FREQUENT: ResourceRow[] = [
  { name: '实例', type: '计算', state: '可用', note: '最常访问', to: '/instances' },
  { name: 'ARM 抢购', type: '计算', state: '可用', note: '最常访问', to: '/grabber' },
  { name: '容量探测', type: '计算', state: '可用', note: '最常访问', to: '/capacity' },
  { name: '虚拟云网络', type: '网络', state: '可用', note: '最常访问', to: '/network' },
]

const RECENT: ResourceRow[] = [
  { name: '成本与用量', type: '计费', state: '可用', note: '最近访问', to: '/cost' },
  { name: '账号', type: '管理', state: '可用', note: '最近访问', to: '/accounts' },
  { name: '设置', type: '管理', state: '可用', note: '最近访问', to: '/settings' },
  { name: '身份与权限', type: '身份', state: '可用', note: '最近访问', to: '/identity' },
]

const PINNED: { to: string; label: string; icon: IconName; tag: string }[] = [
  { to: '/instances', label: '实例', icon: 'compute', tag: '计算' },
  { to: '/network', label: '虚拟云网络', icon: 'network', tag: '网络' },
  { to: '/grabber', label: 'ARM 抢购', icon: 'grabber', tag: '计算' },
]

const RECENT_SERVICES: { to: string; label: string; icon: IconName; tag: string }[] = [
  { to: '/capacity', label: '容量探测', icon: 'capacity', tag: '计算' },
  { to: '/cost', label: '成本与用量', icon: 'cost', tag: '计费' },
  { to: '/identity', label: '身份与权限', icon: 'shield', tag: '身份' },
]

const WORK_ITEMS: { icon: IconName; cat: string; label: string; to: string; time: string }[] = [
  { icon: 'compute', cat: '计算', label: '创建实例', to: '/instances', time: '2 分钟' },
  { icon: 'grabber', cat: '计算', label: '创建 ARM 抢购任务', to: '/grabber', time: '5 分钟' },
  { icon: 'capacity', cat: '计算', label: '检查机型库存', to: '/capacity', time: '1 分钟' },
  { icon: 'network', cat: '网络', label: '查看虚拟云网络', to: '/network', time: '3 分钟' },
]

const WIDGETS: { key: string; label: string }[] = [
  { key: 'resources', label: '资源' },
  { key: 'metrics', label: '资源概览' },
  { key: 'services', label: '服务' },
  { key: 'center', label: '服务中心区域' },
]

const STORAGE_KEY = 'oci-manager:home-widgets'

function readWidgets(): Record<string, boolean> {
  const fallback = Object.fromEntries(WIDGETS.map((w) => [w.key, true]))
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return fallback
    const parsed = JSON.parse(raw) as Record<string, boolean>
    return { ...fallback, ...parsed }
  } catch {
    return fallback
  }
}

export default function Overview() {
  const { account, region } = useAccount()
  const navigate = useNavigate()
  const [data, setData] = useState<OverviewData | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const [resourceTab, setResourceTab] = useState<'frequent' | 'recent' | 'metrics'>('frequent')
  const [widgets, setWidgets] = useState(readWidgets)
  const [customizeOpen, setCustomizeOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const customizeRef = useRef<HTMLButtonElement>(null)

  const refresh = () => setReloadKey((k) => k + 1)

  useEffect(() => {
    if (!account) return
    let cancelled = false
    setLoading(true)
    setError('')
    api.cost
      .overview(account.id, region || undefined)
      .then((res) => {
        if (!cancelled) setData(res)
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [account, region, reloadKey])

  const toggleWidget = (key: string) => {
    setWidgets((prev) => {
      const next = { ...prev, [key]: !prev[key] }
      try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
      } catch {
        /* 隐私模式下 localStorage 不可用，布局仍在本会话生效 */
      }
      return next
    })
  }

  // 页头「定制」：真实控制四张卡片显隐，设置持久化到本机
  const pageAction = useMemo(
    () => (
      <>
        <button
          ref={customizeRef}
          type="button"
          className="oci-btn-secondary"
          onClick={() => setCustomizeOpen((v) => !v)}
          aria-haspopup="menu"
          aria-expanded={customizeOpen}
        >
          <Icon name="sliders" size={15} />
          定制
        </button>
        <AnchoredMenu
          open={customizeOpen}
          anchorRef={customizeRef}
          onClose={() => setCustomizeOpen(false)}
          minWidth={210}
        >
          <div className="oci-actions-note">显示在主页的部件</div>
          {WIDGETS.map((w) => (
            <label key={w.key} className="oci-actions-item" style={{ cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={widgets[w.key]}
                onChange={() => toggleWidget(w.key)}
              />
              {w.label}
            </label>
          ))}
        </AnchoredMenu>
      </>
    ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [customizeOpen, widgets],
  )
  usePageAction(pageAction)

  if (!account) return null

  const copyTenancy = async () => {
    try {
      await navigator.clipboard.writeText(account.tenancy_ocid)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1800)
    } catch {
      setCopied(false)
    }
  }

  const metricRows: ResourceRow[] = [
    {
      name: '实例总数（含已终止）',
      type: '计算',
      state: String(data?.instance_count ?? '—'),
      note: '总数',
      to: '/instances',
    },
    {
      name: '运行中实例',
      type: '计算',
      state: String(data?.running_count ?? '—'),
      note: '运行中',
      to: '/instances',
    },
    {
      name: '运行中 OCPU',
      type: '计算',
      state: data ? data.total_ocpus.toFixed(1) : '—',
      note: 'OCPU',
      to: '/instances',
    },
    {
      name: '运行中内存',
      type: '计算',
      state: data ? `${data.total_memory_gbs.toFixed(0)} GB` : '—',
      note: 'GB',
      to: '/instances',
    },
  ]

  const rows = resourceTab === 'frequent' ? FREQUENT : resourceTab === 'recent' ? RECENT : metricRows

  const dots = (items: CardAction[]) => <CardDots items={items} />

  return (
    <div>
      {error && <ErrorBanner message={error} />}

      <div className="oci-home-grid">
        <div className="oci-home-col">
          {/* 资源：官网左上是「左侧子导航 + 资源表格 + 底部按钮」的复合卡片 */}
          {widgets.resources && (
            <Card title="资源" extra={dots([
              { label: '刷新数据', onSelect: refresh },
              { label: '前往实例列表', onSelect: () => navigate('/instances') },
            ])} bodyClassName="oci-card-body--compact">
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-[150px_minmax(0,1fr)]">
                <div className="border-r border-oracle-border pr-3">
                  <div className="oci-home-tabs" style={{ flexDirection: 'column', alignItems: 'stretch', border: 'none', gap: 2 }}>
                    {([
                      { key: 'frequent', label: '我最常看的' },
                      { key: 'recent', label: '最近访问' },
                      { key: 'metrics', label: '资源集合' },
                    ] as const).map((t) => (
                      <button
                        key={t.key}
                        type="button"
                        className={resourceTab === t.key ? 'active' : ''}
                        onClick={() => setResourceTab(t.key)}
                      >
                        {t.label}
                      </button>
                    ))}
                  </div>
                </div>
                <div className="min-w-0">
                  {loading && !data ? (
                    <Loading text="正在汇总资源与成本…" />
                  ) : (
                    <table className="oci-table">
                      <thead>
                        <tr>
                          <th>名称</th>
                          <th>类型</th>
                          <th>状态</th>
                          <th>说明</th>
                        </tr>
                      </thead>
                      <tbody>
                        {rows.map((r) => (
                          <tr key={r.name}>
                            <td>
                              <Link to={r.to} className="oci-link">
                                {r.name}
                              </Link>
                            </td>
                            <td>{r.type}</td>
                            <td>
                              {r.note === '运行中' || r.note === 'OCPU' || r.note === 'GB' ? (
                                <span>{r.state}</span>
                              ) : (
                                <Badge tone="success">{r.state}</Badge>
                              )}
                            </td>
                            <td>{r.note}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  )}
                  <p className="oci-inline-hint mt-2">共显示 {rows.length} 项。</p>
                </div>
              </div>
              <div className="mt-3 flex items-center gap-2">
                <button type="button" className="oci-btn-secondary oci-btn-sm" onClick={() => navigate('/instances')}>
                  资源浏览器
                </button>
                <button type="button" className="oci-btn-secondary oci-btn-sm" onClick={refresh}>
                  <Icon name="refresh" size={14} />
                  刷新
                </button>
              </div>
            </Card>
          )}

          {/* 服务：官网左下「固定 / 最近访问」双栏 */}
          {widgets.services && (
            <Card title="服务" extra={dots([
              { label: '打开全局服务菜单', onSelect: () => navigate('/') },
              { label: '刷新数据', onSelect: refresh },
            ])}>
              <div className="oci-service-list">
                <div>
                  <div className="oci-label mb-1">固定</div>
                  {PINNED.map((s) => (
                    <Link key={s.to} to={s.to} className="oci-service-row">
                      <Icon name="pin" size={13} />
                      <span className="oci-link flex-1">{s.label}</span>
                      <span className="text-xs text-oracle-muted">{s.tag}</span>
                    </Link>
                  ))}
                </div>
                <div>
                  <div className="oci-label mb-1">最近访问</div>
                  {RECENT_SERVICES.map((s) => (
                    <Link key={s.to} to={s.to} className="oci-service-row">
                      <Icon name={s.icon} size={13} />
                      <span className="oci-link flex-1">{s.label}</span>
                      <span className="text-xs text-oracle-muted">{s.tag}</span>
                    </Link>
                  ))}
                </div>
              </div>
            </Card>
          )}
        </div>

        <div className="oci-home-col">
          {/* 资源概览：五个指标，口径标注明确，避免「总数 / 数量」误读 */}
          {widgets.metrics && (
            <Card title="资源概览" extra={dots([
              { label: '查看成本明细', onSelect: () => navigate('/cost') },
              { label: '刷新数据', onSelect: refresh },
            ])} bodyClassName="oci-card-body--compact">
              <div className="grid grid-cols-2 gap-4">
                <Stat label="实例总数（含已终止）" value={data?.instance_count ?? '—'} accent="brand" />
                <Stat label="运行中实例" value={data?.running_count ?? '—'} accent="pine" />
                <Stat label="运行中 OCPU" value={data ? data.total_ocpus.toFixed(1) : '—'} />
                <Stat label="运行中内存" value={data ? data.total_memory_gbs.toFixed(0) : '—'} unit="GB" accent="yellow" />
              </div>
              <div className="mt-4 border-t border-oracle-border pt-3">
                <p className="oci-stat-label">预计账单 · 本账期 · {data?.currency || 'USD'}</p>
                <p className="mt-1 text-[26px] font-bold leading-8 text-oracle-ink">
                  {data ? formatMoney(data.month_cost, data.currency) : '—'}
                </p>
                <Link to="/cost" className="oci-link mt-2 inline-block text-sm">
                  查看成本详细信息
                </Link>
              </div>
            </Card>
          )}

          {/* 工作版本：官网右上的步骤列表 */}
          <Card title="工作版本" extra={dots([
            { label: '前往实例', onSelect: () => navigate('/instances') },
            { label: '前往抢购', onSelect: () => navigate('/grabber') },
          ])}>
            <div className="oci-step-list">
              {WORK_ITEMS.map((w) => (
                <div className="oci-step" key={w.label}>
                  <Icon name={w.icon} size={16} />
                  <div className="oci-step-main">
                    <div className="oci-step-cat">{w.cat}</div>
                    <Link to={w.to} className="oci-link">{w.label}</Link>
                  </div>
                  <span className="oci-step-time">{w.time}</span>
                </div>
              ))}
            </div>
            <button type="button" className="oci-btn-secondary oci-btn-sm mt-3" onClick={() => navigate('/instances')}>
              全部显示
            </button>
          </Card>

          {/* 服务中心区域：官网右下的建议卡片 */}
          {widgets.center && (
            <Card title="服务中心区域" extra={dots([
              { label: '查看服务健康状况', onSelect: () => navigate('/settings') },
            ])}>
              <p className="oci-inline-hint mb-3">
                通过选择概要信息来定制建议。
                <span className="oci-link ml-1">更新</span>
              </p>
              <ul className="space-y-3">
                <li className="flex items-start gap-3">
                  <Icon name="shield" size={16} />
                  <div>
                    <div className="oci-step-cat">身份</div>
                    <Link to="/identity" className="oci-link text-sm">检查 IAM 用户与组成员</Link>
                  </div>
                </li>
                <li className="flex items-start gap-3">
                  <Icon name="key" size={16} />
                  <div>
                    <div className="oci-step-cat">安全</div>
                    <Link to="/settings" className="oci-link text-sm">配置两步验证与 HTTPS 证书</Link>
                  </div>
                </li>
                <li className="flex items-start gap-3">
                  <Icon name="capacity" size={16} />
                  <div>
                    <div className="oci-step-cat">计算</div>
                    <Link to="/capacity" className="oci-link text-sm">查询 Ampere A1 机型余量</Link>
                  </div>
                </li>
              </ul>
            </Card>
          )}
        </div>
      </div>

      {/* 账户信息：OCID 省略展示 + 一键复制，不再平铺完整字符串 */}
      <Card
        title="账户信息"
        className="mt-4"
        extra={dots([
          { label: '前往账号设置', onSelect: () => navigate('/accounts') },
          { label: '刷新数据', onSelect: refresh },
        ])}
      >
        <div className="grid grid-cols-1 gap-x-10 gap-y-0 sm:grid-cols-2">
          <div className="oci-kv-row">
            <div className="oci-kv-label">账号名称</div>
            <div className="oci-kv-value">{account.name}</div>
          </div>
          <div className="oci-kv-row">
            <div className="oci-kv-label">当前区域</div>
            <div className="oci-kv-value">{data?.region ?? account.region}</div>
          </div>
          <div className="oci-kv-row">
            <div className="oci-kv-label">租户 OCID</div>
            <div className="oci-kv-value">
              <span className="oci-ocid" title={account.tenancy_ocid}>
                …{account.tenancy_ocid.slice(-24)}
              </span>
              <button type="button" className="oci-copy-btn" onClick={() => void copyTenancy()}>
                {copied ? '已复制' : '复制'}
              </button>
            </div>
          </div>
          <div className="oci-kv-row">
            <div className="oci-kv-label">出口代理</div>
            <div className="oci-kv-value">{account.proxy_url ? '已配置专属代理' : '直连'}</div>
          </div>
          <div className="oci-kv-row">
            <div className="oci-kv-label">区间数量</div>
            <div className="oci-kv-value">{data?.compartment_count ?? '—'}</div>
          </div>
          <div className="oci-kv-row">
            <div className="oci-kv-label">同步时间</div>
            <div className="oci-kv-value">{formatDateTime(new Date().toISOString())}</div>
          </div>
        </div>
        {data?.compartment_error && (
          <p className="oci-error-text mt-3">区间列表读取失败：{data.compartment_error}</p>
        )}
        {data?.cost_error && (
          <p className="oci-error-text mt-3">成本数据读取失败：{data.cost_error}</p>
        )}
      </Card>
    </div>
  )
}
