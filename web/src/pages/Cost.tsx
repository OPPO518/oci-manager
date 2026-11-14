import { useCallback, useEffect, useRef, useState } from 'react'
import { usePageAction } from '../components/Layout'
import {
  EmptyState,
  ErrorBanner,
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

type GroupDim = 'service' | 'region' | 'resource_name'
type Measure = 'cost' | 'quantity'
type Granularity = 'hourly' | 'daily' | 'monthly' | 'total'

type ServiceFilter = { id: number; service: string }

type Settings = {
  report: string
  start: string
  end: string
  granularity: Granularity
  measure: Measure
  groupBy: GroupDim
  filters: ServiceFilter[]
}

const DEFAULT_SETTINGS: Settings = {
  report: 'byService',
  start: daysAgo(30),
  end: isoDate(new Date()),
  granularity: 'daily',
  measure: 'cost',
  groupBy: 'service',
  filters: [],
}

const REPORT_PRESETS: Record<
  string,
  { label: string; groupBy: GroupDim; measure: Measure }
> = {
  byService: { label: '按服务列出的成本', groupBy: 'service', measure: 'cost' },
  byRegion: { label: '按区域列出的成本', groupBy: 'region', measure: 'cost' },
  byResource: { label: '按资源列出的成本', groupBy: 'resource_name', measure: 'cost' },
  usageByService: { label: '按服务列出的用量', groupBy: 'service', measure: 'quantity' },
}

const GROUP_LABELS: Record<GroupDim, string> = {
  service: '服务',
  region: '区域',
  resource_name: '资源',
}

const GRANULARITY_LABELS: Record<Granularity, string> = {
  hourly: '每小时',
  daily: '每天',
  monthly: '每月',
  total: '合计',
}

/** 图例 / 柱状图配色（纯 CSS，无第三方图表库） */
const PALETTE = ['#3f6fb5', '#4f9a6a', '#d9913d', '#8a5fb0', '#c0504d', '#3f9a9a', '#7a7f87']

function fieldOf(p: UsagePoint, dim: GroupDim): string {
  if (dim === 'region') return p.region || '(未分类)'
  if (dim === 'resource_name') return p.resource_name || '(未分类)'
  return p.service || '(未分类)'
}

/** 按粒度把 UTC 时间折叠成横轴刻度 */
function dateKey(p: UsagePoint, granularity: Granularity): string {
  const raw = p.start_time || ''
  if (!raw) return '—'
  if (granularity === 'total') return '全部'
  if (granularity === 'monthly') return raw.slice(0, 7)
  if (granularity === 'hourly') return `${raw.slice(0, 13)}:00`
  return raw.slice(0, 10)
}

function withinRange(p: UsagePoint, start: string, end: string): boolean {
  const day = (p.start_time || '').slice(0, 10)
  if (!day) return true
  return (!start || day >= start) && (!end || day <= end)
}

function formatMeasure(value: number, measure: Measure, currency: string): string {
  return measure === 'cost' ? formatMoney(value, currency) : value.toFixed(2)
}

/** 把坐标轴上限取整到 1/2/5×10^n，数据全为 0 时退回 1，保证空网格也有刻度 */
function niceCeil(value: number): number {
  if (value <= 0) return 1
  const pow = Math.pow(10, Math.floor(Math.log10(value)))
  const n = value / pow
  const step = n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10
  return step * pow
}

export default function Cost() {
  const { account, region } = useAccount()
  const [settings, setSettings] = useState<Settings>({ ...DEFAULT_SETTINGS })
  const [snapshot, setSnapshot] = useState<Settings>({ ...DEFAULT_SETTINGS })
  const [points, setPoints] = useState<UsagePoint[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const filterIdRef = useRef(0)

  const patch = (p: Partial<Settings>) => setSettings((s) => ({ ...s, ...p }))

  // 成本/用量拉取：报告条件变更、区域变更、点击「应用」时都会真实重新查询
  const query = useCallback(async () => {
    if (!account) return
    setLoading(true)
    setError('')
    try {
      // resource_name 不是 OCI Usage 的合法 groupBy 维度，后端仍按 service 聚合，
      // 前端再用 resource_name 字段做本地分组，保证控件真实生效
      const groupBy = settings.groupBy === 'region' ? 'region' : 'service'
      const list = await api.cost.query(account.id, {
        start: settings.start,
        end: settings.end,
        granularity: settings.granularity,
        group_by: groupBy,
      })
      setPoints(list)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }, [account, region, settings.start, settings.end, settings.granularity, settings.groupBy])

  useEffect(() => {
    void query()
  }, [query])

  // 本地 Blob 导出 CSV，不引入任何依赖
  function exportCsv() {
    const header = ['服务', '资源', '区域', '起始时间 (UTC)', '用量', '费用', '币种']
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
    link.download = `oci-cost-${settings.start}-${settings.end}.csv`
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
      导出 CSV
    </button>,
  )

  // ---- 筛选器（添加筛选器 / 移除 chip）----
  function addFilter() {
    const id = ++filterIdRef.current
    setSettings((s) => ({ ...s, filters: [...s.filters, { id, service: '' }] }))
  }
  function updateFilter(id: number, service: string) {
    setSettings((s) => ({
      ...s,
      filters: s.filters.map((f) => (f.id === id ? { ...f, service } : f)),
    }))
  }
  function removeFilter(id: number) {
    setSettings((s) => ({ ...s, filters: s.filters.filter((f) => f.id !== id) }))
  }

  function apply() {
    setSnapshot({
      ...settings,
      filters: settings.filters.map((f) => ({ ...f })),
    })
    void query()
  }

  function cancel() {
    setSettings({ ...snapshot, filters: snapshot.filters.map((f) => ({ ...f })) })
  }

  // ---- 基于现有 API 数据做本地过滤（日期区间 / 服务筛选），控件即时生效 ----
  const activeServices = settings.filters.map((f) => f.service).filter(Boolean)
  const filteredPoints = points.filter(
    (p) =>
      withinRange(p, settings.start, settings.end) &&
      (activeServices.length === 0 || activeServices.includes(p.service || '')),
  )

  const { paged, pager } = usePager(filteredPoints)

  const currency = filteredPoints.find((p) => p.currency)?.currency ?? ''
  const measureOf = (p: UsagePoint) => (settings.measure === 'cost' ? p.cost : p.quantity)
  const fmt = (v: number) => formatMeasure(v, settings.measure, currency)
  const total = filteredPoints.reduce((sum, p) => sum + measureOf(p), 0)
  const serviceCount = new Set(filteredPoints.map((p) => p.service || '')).size

  const serviceOptions = [...new Set(points.map((p) => p.service || '').filter(Boolean))].sort()

  // 分组维 → 图例系列；日期 → 横轴刻度；值 → 度量
  const series = [...new Set(filteredPoints.map((p) => fieldOf(p, settings.groupBy)))].sort()
  const bucketsMap = new Map<string, Map<string, number>>()
  for (const p of filteredPoints) {
    const key = dateKey(p, settings.granularity)
    if (!bucketsMap.has(key)) bucketsMap.set(key, new Map())
    const row = bucketsMap.get(key)!
    const s = fieldOf(p, settings.groupBy)
    row.set(s, (row.get(s) ?? 0) + measureOf(p))
  }
  const buckets = [...bucketsMap.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  const bucketTotals = buckets.map(([, row]) => [...row.values()].reduce((a, b) => a + b, 0))
  const maxTotal = Math.max(...bucketTotals, 0)
  const axisMax = niceCeil(maxTotal)
  const ticks = [1, 0.75, 0.5, 0.25, 0].map((f) => axisMax * f)

  const periodText = `${settings.start} 至 ${settings.end}（UTC），${GRANULARITY_LABELS[settings.granularity]}汇总，按${GROUP_LABELS[settings.groupBy]}分色`

  if (!account) return null

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}

      {/* 顶部报告 / 筛选面板 */}
      <div className="oci-filter-panel">
        <div className="oci-filter-fields">
          <div className="oci-filter-field">
            <label htmlFor="cost-report">报告</label>
            <select
              id="cost-report"
              className="oci-select"
              value={settings.report}
              onChange={(e) => {
                const preset = REPORT_PRESETS[e.target.value]
                if (preset) {
                  patch({
                    report: e.target.value,
                    groupBy: preset.groupBy,
                    measure: preset.measure,
                  })
                }
              }}
            >
              {Object.entries(REPORT_PRESETS).map(([key, preset]) => (
                <option key={key} value={key}>
                  {preset.label}
                </option>
              ))}
            </select>
          </div>
          <div className="oci-filter-field">
            <label htmlFor="cost-start">开始日期 (UTC)</label>
            <input
              id="cost-start"
              type="date"
              className="oci-input"
              value={settings.start}
              onChange={(e) => patch({ start: e.target.value })}
            />
          </div>
          <div className="oci-filter-field">
            <label htmlFor="cost-end">结束日期 (UTC)</label>
            <input
              id="cost-end"
              type="date"
              className="oci-input"
              value={settings.end}
              onChange={(e) => patch({ end: e.target.value })}
            />
          </div>
          <div className="oci-filter-field">
            <label htmlFor="cost-granularity">粒度</label>
            <select
              id="cost-granularity"
              className="oci-select"
              value={settings.granularity}
              onChange={(e) => patch({ granularity: e.target.value as Granularity })}
            >
              {(Object.keys(GRANULARITY_LABELS) as Granularity[]).map((g) => (
                <option key={g} value={g}>
                  {GRANULARITY_LABELS[g]}
                </option>
              ))}
            </select>
          </div>
          <div className="oci-filter-field">
            <label htmlFor="cost-show">显示</label>
            <select
              id="cost-show"
              className="oci-select"
              value={settings.measure}
              onChange={(e) => patch({ measure: e.target.value as Measure })}
            >
              <option value="cost">成本</option>
              <option value="quantity">用量</option>
            </select>
          </div>
          <div className="oci-filter-field">
            <label htmlFor="cost-group">分组维</label>
            <select
              id="cost-group"
              className="oci-select"
              value={settings.groupBy}
              onChange={(e) => patch({ groupBy: e.target.value as GroupDim })}
            >
              {(Object.keys(GROUP_LABELS) as GroupDim[]).map((g) => (
                <option key={g} value={g}>
                  {GROUP_LABELS[g]}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div className="oci-filter-row">
          <span className="oci-filter-label">筛选器</span>
          {settings.filters.map((f) => (
            <span className="oci-filter-chip" key={f.id}>
              服务：
              <select
                className="max-w-[190px] border-0 bg-transparent text-xs outline-none"
                value={f.service}
                onChange={(e) => updateFilter(f.id, e.target.value)}
                aria-label="筛选服务"
              >
                <option value="">全部服务</option>
                {serviceOptions.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
              <button
                type="button"
                aria-label="移除筛选器"
                title="移除筛选器"
                onClick={() => removeFilter(f.id)}
              >
                <Icon name="close" size={12} />
              </button>
            </span>
          ))}
          <button type="button" className="oci-btn-secondary oci-btn-sm" onClick={addFilter}>
            <Icon name="plus" size={14} />
            添加筛选器
          </button>
        </div>

        <div className="oci-toolbar-actions">
          <button type="button" className="oci-btn-primary" onClick={apply} disabled={loading}>
            应用
          </button>
          <button type="button" className="oci-btn-secondary" onClick={cancel}>
            取消
          </button>
          <span className="oci-inline-hint">「应用」按当前条件重新拉取数据；「取消」还原到上次应用的设置。</span>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Stat
          label={settings.measure === 'cost' ? '区间总消费' : '区间总用量'}
          value={settings.measure === 'cost' ? formatMoney(total, currency) : total.toFixed(2)}
          accent="brand"
        />
        <Stat label="计费条目" value={filteredPoints.length} accent="pine" />
        <Stat label="服务数量" value={serviceCount} />
      </div>

      {loading && points.length === 0 ? (
        <div className="oci-panel oci-panel-pad">
          <Loading text="正在加载成本与用量数据…" />
        </div>
      ) : (
        <>
          {/* 成本详细信息：柱状图 + 图例 + 详细信息透视表 */}
          <div className="oci-section-title">成本详细信息</div>
          <div className="oci-panel oci-panel-pad">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <h3 className="text-base font-semibold">
                按 日期 (UTC) 列出{settings.measure === 'cost' ? '成本' : '用量'}
              </h3>
              <span className="oci-inline-hint">{periodText}</span>
            </div>

            <div className="mt-4 flex flex-col gap-4 md:flex-row">
              <div className="oci-chart min-w-0 flex-1">
                {/* 网格与坐标：数据全为 0 时也照常渲染 */}
                <div className="oci-chart-grid">
                  {ticks.map((t, i) => (
                    <div className="oci-chart-line" key={i}>
                      <span>{fmt(t)}</span>
                    </div>
                  ))}
                </div>
                {/* 纯 CSS 堆叠柱状图：每个日期一列，按分组维分色 */}
                <div className="oci-chart-bars">
                  {buckets.map(([key, row]) => (
                    <div
                      key={key}
                      className="oci-chart-bar"
                      style={{
                        height: '100%',
                        background: 'transparent',
                        borderRadius: 0,
                        display: 'flex',
                        flexDirection: 'column',
                        justifyContent: 'flex-end',
                        overflow: 'hidden',
                      }}
                    >
                      {series.map((s, idx) => {
                        const value = row.get(s) ?? 0
                        const height = axisMax > 0 ? (value / axisMax) * 100 : 0
                        return (
                          <div
                            key={s}
                            title={`${key} · ${s}：${fmt(value)}`}
                            style={{
                              height: `${height}%`,
                              minHeight: value > 0 ? 2 : 0,
                              background: PALETTE[idx % PALETTE.length],
                            }}
                          />
                        )
                      })}
                    </div>
                  ))}
                </div>
                <div className="oci-chart-x">
                  {buckets.length === 0 ? (
                    <span>无数据</span>
                  ) : (
                    buckets.map(([key]) => <span key={key}>{key}</span>)
                  )}
                </div>
              </div>

              <div className="oci-chart-legend md:w-40 md:shrink-0">
                {series.length === 0 ? (
                  <span className="oci-inline-hint">暂无可分组的数据</span>
                ) : (
                  series.map((s, i) => (
                    <span key={s}>
                      <i style={{ background: PALETTE[i % PALETTE.length] }} />
                      {s}
                    </span>
                  ))
                )}
              </div>
            </div>
          </div>

          {/* 详细信息表格：日期 / 各分组维 / 总计 */}
          <div className="oci-panel">
            <div className="oci-panel-pad">
              <span className="text-sm font-semibold">详细信息</span>
            </div>
            <div className="oci-table-wrap">
              <table className="oci-table">
                <thead>
                  <tr>
                    <th>日期 (UTC)</th>
                    {series.map((s) => (
                      <th key={s} className="text-right">
                        {s}
                      </th>
                    ))}
                    <th className="text-right">总计</th>
                  </tr>
                </thead>
                <tbody>
                  {buckets.length === 0 ? (
                    <tr>
                      <td colSpan={series.length + 2}>
                        <EmptyState text="该时间范围内没有成本数据。" />
                      </td>
                    </tr>
                  ) : (
                    buckets.map(([key, row]) => {
                      const rowTotal = [...row.values()].reduce((a, b) => a + b, 0)
                      return (
                        <tr key={key}>
                          <td className="text-xs">{key}</td>
                          {series.map((s) => (
                            <td key={s} className="text-right text-xs">
                              {fmt(row.get(s) ?? 0)}
                            </td>
                          ))}
                          <td className="text-right text-xs font-semibold">{fmt(rowTotal)}</td>
                        </tr>
                      )
                    })
                  )}
                </tbody>
              </table>
            </div>
          </div>

          {/* 计费明细：保留原有原始行表格与 Pager */}
          <div className="oci-section-title">计费明细</div>
          <div className="oci-panel">
            {filteredPoints.length === 0 ? (
              <div className="oci-panel-pad">
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
                        <th>起始时间 (UTC)</th>
                        <th className="text-right">用量</th>
                        <th className="text-right">费用</th>
                      </tr>
                    </thead>
                    <tbody>
                      {paged.map((p, index) => (
                        <tr key={`${p.service}-${p.start_time}-${index}`}>
                          <td className="text-xs">{p.service || '—'}</td>
                          <td className="text-xs text-oracle-muted">{p.resource_name || '—'}</td>
                          <td className="text-xs text-oracle-muted">{p.region || '—'}</td>
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
          </div>

          {/* 订阅：当前接口未提供订阅数据，按官网列结构展示空态，不伪造数据 */}
          <div className="oci-section-title">订阅</div>
          <div className="oci-panel">
            <div className="oci-table-wrap">
              <table className="oci-table">
                <thead>
                  <tr>
                    <th>名称</th>
                    <th>类型</th>
                    <th>状态</th>
                    <th>开始日期 (UTC)</th>
                    <th>结束日期 (UTC)</th>
                    <th>承诺总值</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td colSpan={6}>
                      <EmptyState text="暂无订阅信息（当前接口未提供订阅数据）。" />
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
