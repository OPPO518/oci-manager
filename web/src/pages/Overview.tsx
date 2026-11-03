import { useEffect, useState } from 'react'
import { Card, ErrorBanner, Loading, Stat } from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { formatDateTime, formatMoney } from '../lib/format'
import type { Overview as OverviewData } from '../lib/types'

export default function Overview() {
  const { account, region } = useAccount()
  const [data, setData] = useState<OverviewData | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

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

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} />}

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
        <Stat label="实例总数" value={data?.instance_count ?? '—'} />
        <Stat label="运行中" value={data?.running_count ?? '—'} />
        <Stat label="运行中 OCPU" value={data ? data.total_ocpus.toFixed(1) : '—'} />
        <Stat label="运行中内存" value={data ? data.total_memory_gbs.toFixed(0) : '—'} unit="GB" />
        <Stat
          label="本月消费"
          value={data ? formatMoney(data.month_cost, data.currency) : '—'}
        />
      </div>

      {loading && !data ? (
        <Card>
          <Loading text="正在汇总资源与成本…" />
        </Card>
      ) : (
        <Card title="账号信息">
          <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
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
            <p className="mt-3 text-xs text-amber-700">
              区间列表读取失败：{data.compartment_error}
            </p>
          )}
          {data?.cost_error && (
            <p className="mt-3 text-xs text-amber-700">成本数据读取失败：{data.cost_error}</p>
          )}
        </Card>
      )}
    </div>
  )
}
