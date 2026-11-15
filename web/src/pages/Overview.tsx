import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { usePageAction } from '../components/Layout'
import { Badge, Card, ErrorBanner, Icon, Loading, Stat } from '../components/ui'
import { api } from '../lib/api'
import { formatDateTime, formatMoney } from '../lib/format'
import type { Dashboard } from '../lib/types'

function Donut({ running, stopped }: { running: number; stopped: number }) {
  const total = Math.max(1, running + stopped)
  const r = 52
  const c = 2 * Math.PI * r
  const runPct = running / total
  return (
    <div className="flex items-center gap-5">
      <svg width="140" height="140" viewBox="0 0 140 140" className="shrink-0">
        <circle cx="70" cy="70" r={r} fill="none" stroke="var(--rw-border)" strokeWidth="16" />
        <circle
          cx="70"
          cy="70"
          r={r}
          fill="none"
          stroke="var(--oracle-brand)"
          strokeWidth="16"
          strokeLinecap="round"
          strokeDasharray={`${runPct * c} ${c}`}
          transform="rotate(-90 70 70)"
        />
        <text x="70" y="66" textAnchor="middle" fontSize="22" fontWeight="600" fill="var(--rw-ink)">
          {total}
        </text>
        <text x="70" y="84" textAnchor="middle" fontSize="11" fill="var(--rw-muted)">
          实例总量
        </text>
      </svg>
      <div className="space-y-2 text-sm">
        <div className="flex items-center gap-2">
          <span className="inline-block h-3 w-3 rounded-sm" style={{ background: 'var(--oracle-brand)' }} />
          <span className="text-rw-muted">运行中</span>
          <span className="font-medium text-rw-ink">{running}</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="inline-block h-3 w-3 rounded-sm" style={{ background: 'var(--rw-border)' }} />
          <span className="text-rw-muted">已停止</span>
          <span className="font-medium text-rw-ink">{stopped}</span>
        </div>
      </div>
    </div>
  )
}

function RegionBars({ ranks }: { ranks: { region: string; count: number }[] }) {
  const max = Math.max(1, ...ranks.map((r) => r.count))
  if (ranks.length === 0) {
    return <p className="oci-inline-hint">暂无区域数据。</p>
  }
  return (
    <div className="space-y-3">
      {ranks.slice(0, 6).map((r) => (
        <div key={r.region} className="flex items-center gap-3 text-sm">
          <span className="w-32 shrink-0 truncate font-mono text-xs text-rw-muted">{r.region}</span>
          <div className="h-3 flex-1 overflow-hidden rounded bg-rw-canvas">
            <div
              className="h-full rounded bg-oracle-brand"
              style={{ width: `${(r.count / max) * 100}%` }}
            />
          </div>
          <span className="w-8 shrink-0 text-right font-medium text-rw-ink">{r.count}</span>
        </div>
      ))}
    </div>
  )
}

export default function Overview() {
  const navigate = useNavigate()
  const [data, setData] = useState<Dashboard | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [reloadKey, setReloadKey] = useState(0)
  const refresh = () => setReloadKey((k) => k + 1)
  const refreshRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    api.dashboard
      .get()
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
  }, [reloadKey])

  const pageAction = useMemo(
    () => (
      <button
        ref={refreshRef}
        type="button"
        className="oci-btn-secondary"
        onClick={refresh}
      >
        <Icon name="refresh" size={15} />
        刷新
      </button>
    ),
    [],
  )
  usePageAction(pageAction)

  const activityTone = (result: string) => {
    const r = result.toLowerCase()
    if (['success', 'ok', 'updated', 'true'].includes(r)) return 'success' as const
    if (['failed', 'error', 'denied'].includes(r)) return 'danger' as const
    if (r === 'blocked') return 'warning' as const
    return 'neutral' as const
  }

  return (
    <div>
      {error && <ErrorBanner message={error} />}

      {loading && !data ? (
        <Loading text="正在聚合所有账号的资源与成本…" />
      ) : data ? (
        <>
          {/* 顶部统计卡片：oci-start 系统资源监控风格的聚合指标 */}
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-3 xl:grid-cols-6">
            <Stat
              label="实例总数"
              value={data.instances_total}
              accent="brand"
              hint={`${data.running_count} 运行 / ${data.stopped_count} 停止`}
            />
            <Stat label="运行中 OCPU" value={data.total_ocpus.toFixed(1)} hint="跨账号合计" />
            <Stat label="运行中内存" value={data.total_memory_gbs.toFixed(0)} unit="GB" accent="yellow" />
            <Stat
              label="本月费用"
              value={formatMoney(data.month_cost, data.currency || 'USD')}
              accent="pine"
              hint="跨账号合计"
            />
            <Stat
              label="抢购任务"
              value={data.grab_tasks_total}
              hint={`${data.grab_tasks_running} 进行中 / ${data.grab_tasks_success} 成功`}
            />
            <Stat label="账号数" value={data.accounts_total} accent="pine" />
          </div>

          {data.errors.length > 0 && (
            <div className="oci-status-strip is-warning mt-4">
              <Icon name="warning" size={16} />
              <div className="oci-status-main">
                {data.errors.slice(0, 3).map((e) => (
                  <span key={e}>{e}</span>
                ))}
              </div>
            </div>
          )}

          <div className="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card title="实例分布">
              <Donut running={data.running_count} stopped={data.stopped_count} />
            </Card>
            <Card title="区域资源排行" extra={
              <button type="button" className="oci-link text-sm" onClick={() => navigate('/instances')}>
                查看实例
              </button>
            }>
              <RegionBars ranks={data.region_ranking} />
            </Card>
          </div>

          <Card
            title="最近活动"
            className="mt-4"
            extra={
              <Link to="/settings" className="oci-link text-sm">
                审计日志
              </Link>
            }
            bodyClassName="oci-card-body--compact"
          >
            {data.activities.length === 0 ? (
              <p className="oci-inline-hint">暂无活动记录。</p>
            ) : (
              <ul className="divide-y divide-oracle-border">
                {data.activities.map((a) => (
                  <li key={a.id} className="flex items-center gap-3 py-2.5 text-sm">
                    <Badge tone={activityTone(a.result)}>{a.result_label || a.result}</Badge>
                    <span className="w-28 shrink-0 truncate text-rw-muted">
                      {a.account || '面板'}
                    </span>
                    <span className="flex-1 truncate">
                      {a.category_label || a.category} · {a.operation_label || a.operation}
                      {a.target ? `（${a.target}）` : ''}
                    </span>
                    <span className="shrink-0 text-xs text-rw-muted">
                      {formatDateTime(a.created_at)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </>
      ) : null}
    </div>
  )
}
