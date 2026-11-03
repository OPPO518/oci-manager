import { useCallback, useEffect, useState } from 'react'
import { Card, EmptyState, ErrorBanner, Field, Loading, Stat } from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { daysAgo, formatDate, formatMoney, isoDate } from '../lib/format'
import type { UsagePoint } from '../lib/types'

export default function Cost() {
  const { account, region } = useAccount()
  const [start, setStart] = useState(daysAgo(30))
  const [end, setEnd] = useState(isoDate(new Date()))
  const [granularity, setGranularity] = useState('daily')
  const [points, setPoints] = useState<UsagePoint[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const query = useCallback(async () => {
    if (!account) return
    setLoading(true)
    setError('')
    try {
      const list = await api.cost.query(account.id, {
        start,
        end,
        granularity,
        group_by: 'service',
      })
      setPoints(list)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }, [account, region, start, end, granularity])

  useEffect(() => {
    void query()
  }, [query])

  const currency = points.find((p) => p.currency)?.currency ?? ''
  const total = points.reduce((sum, p) => sum + p.cost, 0)

  const byService = new Map<string, number>()
  for (const p of points) {
    byService.set(p.service || '(未分类)', (byService.get(p.service || '(未分类)') ?? 0) + p.cost)
  }
  const serviceRows = [...byService.entries()].sort((a, b) => b[1] - a[1])
  const maxService = serviceRows[0]?.[1] ?? 0

  if (!account) return null

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}

      <Card
        title="查询条件"
        extra={
          <button type="button" className="oci-btn-primary" onClick={() => void query()}>
            查询
          </button>
        }
      >
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
          <Field label="开始日期">
            <input
              type="date"
              className="oci-input"
              value={start}
              onChange={(e) => setStart(e.target.value)}
            />
          </Field>
          <Field label="结束日期">
            <input
              type="date"
              className="oci-input"
              value={end}
              onChange={(e) => setEnd(e.target.value)}
            />
          </Field>
          <Field label="时间粒度">
            <select
              className="oci-input"
              value={granularity}
              onChange={(e) => setGranularity(e.target.value)}
            >
              <option value="daily">按天</option>
              <option value="monthly">按月</option>
              <option value="total">合计</option>
            </select>
          </Field>
        </div>
      </Card>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Stat label="区间总消费" value={formatMoney(total, currency)} />
        <Stat label="计费条目" value={points.length} />
        <Stat label="服务数量" value={serviceRows.length} />
      </div>

      <Card title="按服务拆分">
        {loading && points.length === 0 ? (
          <Loading />
        ) : serviceRows.length === 0 ? (
          <EmptyState text="该区间没有消费记录" />
        ) : (
          <div className="space-y-2">
            {serviceRows.slice(0, 15).map(([service, cost]) => (
              <div key={service} className="flex items-center gap-3">
                <span className="w-48 shrink-0 truncate text-xs">{service}</span>
                <div className="h-3 flex-1 rounded bg-oracle-surface">
                  <div
                    className="h-3 rounded bg-oracle-red"
                    style={{ width: `${maxService > 0 ? (cost / maxService) * 100 : 0}%` }}
                  />
                </div>
                <span className="w-24 shrink-0 text-right text-xs">{formatMoney(cost, currency)}</span>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card title="明细">
        {points.length === 0 ? (
          <EmptyState text="暂无数据" />
        ) : (
          <div className="max-h-96 overflow-auto">
            <table className="oci-table">
              <thead>
                <tr>
                  <th>服务</th>
                  <th>资源</th>
                  <th>区域</th>
                  <th>起始时间</th>
                  <th className="text-right">用量</th>
                  <th className="text-right">费用</th>
                </tr>
              </thead>
              <tbody>
                {points.slice(0, 200).map((p, index) => (
                  <tr key={`${p.service}-${p.start_time}-${index}`}>
                    <td className="text-xs">{p.service}</td>
                    <td className="text-xs text-oracle-muted">{p.resource_name}</td>
                    <td className="text-xs text-oracle-muted">{p.region}</td>
                    <td className="text-xs text-oracle-muted">{formatDate(p.start_time)}</td>
                    <td className="text-right text-xs">{p.quantity.toFixed(2)}</td>
                    <td className="text-right text-xs">{formatMoney(p.cost, p.currency)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  )
}
