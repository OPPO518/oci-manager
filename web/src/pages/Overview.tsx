import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { usePageAction } from '../components/Layout'
import {
  Badge,
  Card,
  CardDots,
  ErrorBanner,
  Icon,
  Loading,
  Notice,
  Stat,
  type IconName,
} from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { formatDateTime, formatMoney } from '../lib/format'
import type { Overview as OverviewData } from '../lib/types'

const QUICK_ACTIONS: { to: string; label: string; icon: IconName }[] = [
  { to: '/instances', label: '查看实例', icon: 'compute' },
  { to: '/grabber', label: '创建抢购任务', icon: 'grabber' },
  { to: '/capacity', label: '容量探测', icon: 'capacity' },
  { to: '/cost', label: '成本与用量', icon: 'cost' },
]

const RECENT_LINKS: { to: string; label: string }[] = [
  { to: '/instances', label: '实例' },
  { to: '/cost', label: '成本' },
  { to: '/capacity', label: '容量探测' },
  { to: '/accounts', label: '账号' },
]

export default function Overview() {
  const { account, region } = useAccount()
  const [data, setData] = useState<OverviewData | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [customizeHint, setCustomizeHint] = useState(false)

  // 页头右侧主操作（展示性，明确告知演示限制，不假装保存布局）
  usePageAction(
    <button
      type="button"
      className="oci-btn-secondary"
      onClick={() => setCustomizeHint((v) => !v)}
    >
      <Icon name="sliders" size={15} />
      Customize
    </button>,
  )

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
  }, [account, region])

  if (!account) return null

  const costTone = data?.cost_error ? 'warning' : data ? 'success' : 'neutral'
  const compartmentTone = data?.compartment_error ? 'warning' : data ? 'success' : 'neutral'

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} />}
      {customizeHint && (
        <Notice tone="info" onClose={() => setCustomizeHint(false)}>
          布局自定义功能在此演示中不可用；widget 仅用于展示。
        </Notice>
      )}

      {/* Resources 指标条 */}
      <Card title="Resources" extra={<CardDots />} bodyClassName="oci-card-body--compact">
        {loading && !data ? (
          <Loading text="正在汇总资源与成本…" />
        ) : (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
            <Stat label="实例总数" value={data?.instance_count ?? '—'} accent="brand" />
            <Stat label="运行中" value={data?.running_count ?? '—'} accent="pine" />
            <Stat label="运行中 OCPU" value={data ? data.total_ocpus.toFixed(1) : '—'} />
            <Stat
              label="运行中内存"
              value={data ? data.total_memory_gbs.toFixed(0) : '—'}
              unit="GB"
              accent="yellow"
            />
            <Stat
              label="本月消费"
              value={data ? formatMoney(data.month_cost, data.currency) : '—'}
              accent="brand"
            />
          </div>
        )}
      </Card>

      {/* widget 网格（grid-cols-12） */}
      <div className="grid grid-cols-12 gap-4">
        <div className="col-span-12 space-y-4 lg:col-span-8">
          {/* Quick actions / Launch resources */}
          <Card title="Quick actions / Launch resources" extra={<CardDots />}>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {QUICK_ACTIONS.map((a) => (
                <Link
                  key={a.to}
                  to={a.to}
                  className="flex items-center gap-3 rounded border border-oracle-border px-4 py-3 text-sm font-medium text-oracle-ink transition-colors hover:border-oracle-brand hover:bg-oracle-selected"
                >
                  <span className="text-oracle-brand">
                    <Icon name={a.icon} />
                  </span>
                  {a.label}
                </Link>
              ))}
            </div>
          </Card>

          {/* Cost & usage */}
          <Card title="Cost & usage" extra={<CardDots />}>
            <div className="flex flex-wrap items-center justify-between gap-4">
              <div>
                <p className="oci-stat-label">本月消费</p>
                <p className="mt-2 text-[28px] font-bold leading-9 text-oracle-ink">
                  {data ? formatMoney(data.month_cost, data.currency) : '—'}
                </p>
                <p className="oci-hint mt-1">
                  统计区间：本账期 · 币种 {data?.currency || '—'}
                </p>
              </div>
              <Link to="/cost" className="oci-btn-secondary">
                查看成本明细
              </Link>
            </div>
          </Card>

          {/* Recently visited */}
          <Card title="Recently visited" extra={<CardDots />}>
            <ul className="divide-y divide-oracle-border">
              {RECENT_LINKS.map((item) => (
                <li key={item.to}>
                  <Link
                    to={item.to}
                    className="flex items-center gap-3 py-2.5 text-sm text-oracle-brandText hover:underline"
                  >
                    <Icon name="clock" size={14} />
                    {item.label}
                  </Link>
                </li>
              ))}
            </ul>
          </Card>
        </div>

        <div className="col-span-12 space-y-4 lg:col-span-4">
          {/* Account info */}
          <Card title="Account info" extra={<CardDots />}>
            <dl className="grid grid-cols-1 gap-3 text-sm">
              <div>
                <dt className="text-xs text-oracle-muted">账号名称</dt>
                <dd className="mt-0.5">{account.name}</dd>
              </div>
              <div>
                <dt className="text-xs text-oracle-muted">区域</dt>
                <dd className="mt-0.5">{data?.region ?? account.region}</dd>
              </div>
              <div>
                <dt className="text-xs text-oracle-muted">租户 OCID</dt>
                <dd className="mt-0.5 break-all font-mono text-xs">{account.tenancy_ocid}</dd>
              </div>
              <div>
                <dt className="text-xs text-oracle-muted">出口代理</dt>
                <dd className="mt-0.5">{account.proxy_url || '直连'}</dd>
              </div>
              <div>
                <dt className="text-xs text-oracle-muted">区间数量</dt>
                <dd className="mt-0.5">{data?.compartment_count ?? '—'}</dd>
              </div>
              <div>
                <dt className="text-xs text-oracle-muted">统计时间</dt>
                <dd className="mt-0.5">{formatDateTime(new Date().toISOString())}</dd>
              </div>
            </dl>

            {data?.compartment_error && (
              <p className="oci-error-text mt-3">区间列表读取失败：{data.compartment_error}</p>
            )}
            {data?.cost_error && (
              <p className="oci-error-text mt-3">成本数据读取失败：{data.cost_error}</p>
            )}
          </Card>

          {/* Service status */}
          <Card title="Service status" extra={<CardDots />}>
            <ul className="space-y-3 text-sm">
              <li className="flex items-center justify-between gap-3">
                <span>控制台会话</span>
                <Badge tone="success">正常</Badge>
              </li>
              <li className="flex items-center justify-between gap-3">
                <span>成本数据</span>
                <Badge tone={costTone}>{data?.cost_error ? '部分异常' : data ? '正常' : '未知'}</Badge>
              </li>
              <li className="flex items-center justify-between gap-3">
                <span>区间列表</span>
                <Badge tone={compartmentTone}>
                  {data?.compartment_error ? '部分异常' : data ? '正常' : '未知'}
                </Badge>
              </li>
              <li className="flex items-center justify-between gap-3">
                <span>当前区域</span>
                <span className="text-xs text-oracle-muted">{region || account.region || '—'}</span>
              </li>
            </ul>
          </Card>
        </div>
      </div>
    </div>
  )
}
