import { useCallback, useEffect, useState } from 'react'
import { usePageAction } from '../components/Layout'
import {
  Card,
  EmptyState,
  ErrorBanner,
  Field,
  Icon,
  Loading,
  Stat,
  usePager,
} from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { daysAgo, formatDate, formatMoney, isoDate } from '../lib/format'
import type { UsagePoint } from '../lib/types'

function csvCell(value: string | number): string {
  const s = String(value ?? '')
  return /[",\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

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

  // 本地 Blob 导出 CSV，不引入任何依赖
  function exportCsv() {
    const header = ['Service', 'Resource', 'Region', 'Start time', 'Quantity', 'Cost', 'Currency']
    const rows = points.map((p) =>
      [p.service, p.resource_name, p.region, p.start_time, p.quantity, p.cost, p.currency]
        .map(csvCell)
        .join(','),
    )
    const blob = new Blob(['\uFEFF' + [header.join(','), ...rows].join('\r\n')], {
      type: 'text/csv;charset=utf-8',
    })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `oci-cost-${start}-${end}.csv`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  }

  usePageAction(
    <button
      type="button"
      className="oci-btn-secondary"
      onClick={exportCsv}
      disabled={points.length === 0}
    >
      <Icon name="download" size={15} />
      Export CSV
    </button>,
  )

  const currency = points.find((p) => p.currency)?.currency ?? ''
  const total = points.reduce((sum, p) => sum + p.cost, 0)

  const byService = new Map<string, number>()
  for (const p of points) {
    byService.set(p.service || '(未分类)', (byService.get(p.service || '(未分类)') ?? 0) + p.cost)
  }
  const serviceRows = [...byService.entries()].sort((a, b) => b[1] - a[1])
  const maxService = serviceRows[0]?.[1] ?? 0

  const { paged, pager } = usePager(points)

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
              className="oci-select"
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
        <Stat label="区间总消费" value={formatMoney(total, currency)} accent="brand" />
        <Stat label="计费条目" value={points.length} accent="pine" />
        <Stat label="服务数量" value={serviceRows.length} />
      </div>

      <Card title="按服务拆分">
        {loading && points.length === 0 ? (
          <Loading />
        ) : serviceRows.length === 0 ? (
          <EmptyState />
        ) : (
          <div className="space-y-2">
            {serviceRows.slice(0, 15).map(([service, cost]) => (
              <div key={service} className="flex items-center gap-3">
                <span className="w-48 shrink-0 truncate text-xs">{service}</span>
                <div className="oci-progress-track flex-1">
                  <div
                    className="oci-progress-fill"
                    style={{ width: `${maxService > 0 ? (cost / maxService) * 100 : 0}%` }}
                  />
                </div>
                <span className="w-24 shrink-0 text-right text-xs">
                  {formatMoney(cost, currency)}
                </span>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card title="明细" bodyClassName="oci-card-body--flush">
        {points.length === 0 ? (
          <div className="p-6">
            <EmptyState />
          </div>
        ) : (
          <>
            <div className="oci-table-wrap max-h-96">
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
                  {paged.map((p, index) => (
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
            {pager}
          </>
        )}
      </Card>
    </div>
  )
}
