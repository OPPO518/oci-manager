import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  CapacityBadge,
  Card,
  EmptyState,
  ErrorBanner,
  Field,
  Loading,
  Notice,
  Switch,
  usePager,
} from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { formatDateTime } from '../lib/format'
import type { CapacityResponse } from '../lib/types'

const SHAPES = [
  'VM.Standard.A1.Flex',
  'VM.Standard.E2.1.Micro',
  'VM.Standard.E4.Flex',
  'VM.Standard.E5.Flex',
  'VM.Standard3.Flex',
  'VM.Standard.A2.Flex',
]

export default function Capacity() {
  const { account, region } = useAccount()
  const navigate = useNavigate()

  const [shape, setShape] = useState('VM.Standard.A1.Flex')
  const [ocpus, setOcpus] = useState(4)
  const [memory, setMemory] = useState(24)
  const [ads, setAds] = useState<string[]>([])
  const [selectedAD, setSelectedAD] = useState('')
  const [autoRefresh, setAutoRefresh] = useState(false)
  const [result, setResult] = useState<CapacityResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const effectiveRegion = region || account?.region || ''

  const { paged, pager } = usePager(result?.items ?? [])

  const loadADs = useCallback(async () => {
    if (!account) return
    try {
      const list = await api.resources.availabilityDomains(account.id, {
        region: effectiveRegion || undefined,
      })
      setAds(list)
    } catch {
      setAds([])
    }
  }, [account, effectiveRegion])

  useEffect(() => {
    void loadADs()
  }, [loadADs])

  const check = useCallback(async () => {
    if (!account) return
    setLoading(true)
    setError('')
    try {
      const res = await api.capacity.check(account.id, {
        region: effectiveRegion,
        shape,
        ocpus,
        memory_gbs: memory,
        availability_domains: selectedAD ? [selectedAD] : [],
      })
      setResult(res)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }, [account, effectiveRegion, shape, ocpus, memory, selectedAD])

  useEffect(() => {
    if (!autoRefresh || !account) return
    const timer = window.setInterval(() => void check(), 60000)
    return () => window.clearInterval(timer)
  }, [autoRefresh, account, check])

  if (!account) return null

  const availableCount = result?.items.filter((it) => it.available).length ?? 0

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}

      {result?.unsupported && <Notice tone="info">{result.hint}</Notice>}

      <Card
        title="查询条件"
        extra={
          <button type="button" className="oci-btn-primary" disabled={loading} onClick={() => void check()}>
            {loading ? '查询中…' : '开始探测'}
          </button>
        }
      >
        <Notice tone="info" className="mb-4">
          使用 OCI <strong>Compute Capacity Report</strong> 只读接口查询机型余量，不会创建实例、不占用配额，
          也不会产生 <code className="font-mono">LaunchInstance</code> 写调用。抢购任务会先做这一步，
          确认有货才发起创建，从而大幅减少无效请求、降低触发风控的概率。
        </Notice>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Field label="区域" hint="跟随顶部区域选择器">
            <input className="oci-input" value={effectiveRegion} readOnly />
          </Field>

          <Field label="机型 Shape">
            <input
              className="oci-input font-mono text-xs"
              list="capacity-shapes"
              value={shape}
              onChange={(e) => setShape(e.target.value)}
            />
            <datalist id="capacity-shapes">
              {SHAPES.map((s) => (
                <option key={s} value={s} />
              ))}
            </datalist>
          </Field>

          <Field label="OCPU">
            <input
              type="number"
              min={1}
              className="oci-input"
              value={ocpus}
              onChange={(e) => setOcpus(Number(e.target.value))}
            />
          </Field>

          <Field label="内存（GB）">
            <input
              type="number"
              min={1}
              className="oci-input"
              value={memory}
              onChange={(e) => setMemory(Number(e.target.value))}
            />
          </Field>

          <Field label="可用域" hint="留空则扫描该区域全部可用域">
            <select className="oci-select" value={selectedAD} onChange={(e) => setSelectedAD(e.target.value)}>
              <option value="">全部可用域</option>
              {ads.map((ad) => (
                <option key={ad} value={ad}>
                  {ad}
                </option>
              ))}
            </select>
          </Field>

          <div className="flex items-end gap-4 pb-1">
            <Switch checked={autoRefresh} onChange={setAutoRefresh} label="每 60 秒自动刷新" />
          </div>
        </div>
      </Card>

      <Card
        title="探测结果"
        extra={
          result ? (
            <div className="flex flex-wrap items-center gap-2 text-xs text-oracle-muted">
              <span>
                共 {result.items.length} 个可用域，其中{' '}
                <strong className="text-oracle-ink">{availableCount}</strong> 个有库存
              </span>
              <span>·</span>
              <span>刷新于 {formatDateTime(result.checked_at)}</span>
            </div>
          ) : undefined
        }
      >
        {loading && !result ? (
          <Loading text="正在向 OCI 查询容量报告…" />
        ) : !result ? (
          <EmptyState text="设置查询条件后点击「开始探测」" />
        ) : result.items.length === 0 ? (
          <EmptyState text="未返回任何容量数据" />
        ) : (
          <>
            <div className="oci-table-wrap">
              <table className="oci-table">
                <thead>
                  <tr>
                    <th>可用域</th>
                    <th>故障域</th>
                    <th>机型</th>
                    <th>OCPU / 内存</th>
                    <th>容量状态</th>
                    <th>可创建数量</th>
                    <th>备注</th>
                  </tr>
                </thead>
                <tbody>
                  {paged.map((it, idx) => (
                    <tr key={`${it.availability_domain}-${it.fault_domain}-${idx}`}>
                      <td className="font-mono text-xs">{it.availability_domain}</td>
                      <td className="font-mono text-xs text-oracle-muted">
                        {it.fault_domain || '（全部）'}
                      </td>
                      <td className="font-mono text-xs">{it.shape}</td>
                      <td className="text-xs">
                        {it.ocpus} OCPU / {it.memory_gbs} GB
                      </td>
                      <td>
                        <CapacityBadge status={it.availability_status} />
                      </td>
                      <td className="text-xs">
                        {it.available_count != null && it.available_count > 0 ? (
                          it.available_count
                        ) : (
                          <span className="text-oracle-muted">
                            {it.available ? '可用' : '—'}
                            {it.available_count != null && it.available_count === 0
                              ? '（普通账户不返回具体数量）'
                              : ''}
                          </span>
                        )}
                      </td>
                      <td className="max-w-[260px] break-all text-xs text-oracle-danger">
                        {it.error || ''}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {pager}
          </>
        )}

        {result && (
          <div className="mt-4 flex flex-wrap items-center gap-3 border-t border-oracle-border pt-4">
            <button
              type="button"
              className="oci-btn-secondary"
              onClick={() =>
                navigate('/grabber', {
                  state: {
                    fromCapacity: {
                      shape,
                      ocpus,
                      memory_gbs: memory,
                      ad_name: selectedAD,
                      region: effectiveRegion,
                    },
                  },
                })
              }
            >
              用此配置创建抢购任务
            </button>
            <span className="text-xs text-oracle-muted">
              提示：<code className="font-mono">AVAILABLE</code> 表示当前可创建；普通账户不会返回
              <code className="font-mono">available_count</code>，请以状态为准。
            </span>
          </div>
        )}
      </Card>
    </div>
  )
}
