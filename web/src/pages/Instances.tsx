import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { usePageAction } from '../components/Layout'
import {
  ActionsMenu,
  Badge,
  Card,
  Checkbox,
  EmptyState,
  ErrorBanner,
  Field,
  Icon,
  Loading,
  Modal,
  SortableTh,
  usePager,
} from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { formatDateTime } from '../lib/format'
import type { Compartment, Instance } from '../lib/types'

type SortKey = 'name' | 'state' | 'shape' | 'ocpus' | 'ad' | 'created'

function sortValue(inst: Instance, key: SortKey): string | number {
  switch (key) {
    case 'name':
      return inst.display_name || ''
    case 'state':
      return inst.state
    case 'shape':
      return inst.shape
    case 'ocpus':
      return inst.ocpus
    case 'ad':
      return inst.availability_domain
    case 'created':
      return inst.time_created ?? ''
    default:
      return ''
  }
}

export default function Instances() {
  const { account, region } = useAccount()
  const effectiveRegion = region || undefined
  const navigate = useNavigate()

  const [compartments, setCompartments] = useState<Compartment[]>([])
  const [compartmentId, setCompartmentId] = useState('')
  const [instances, setInstances] = useState<Instance[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [busyId, setBusyId] = useState('')

  const [shapeTarget, setShapeTarget] = useState<Instance | null>(null)
  const [shapeForm, setShapeForm] = useState({ ocpus: 4, memory_gbs: 24 })
  const [terminateTarget, setTerminateTarget] = useState<Instance | null>(null)
  const [preserveBootVolume, setPreserveBootVolume] = useState(true)

  // 展示性状态：List Scope 过滤、行选择、表头排序
  const [stateFilter, setStateFilter] = useState('ALL')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [sort, setSort] = useState<{ key: SortKey | ''; dir: 'asc' | 'desc' }>({
    key: '',
    dir: 'asc',
  })

  // 页头右侧主操作：复用真实「抢购任务」入口；刷新降级为次按钮
  usePageAction(
    <>
      <button type="button" className="oci-btn-secondary" onClick={() => void loadInstances()}>
        <Icon name="refresh" size={15} />
        刷新
      </button>
      <button type="button" className="oci-btn-primary" onClick={() => navigate('/grabber')}>
        <Icon name="plus" size={15} />
        创建抢购任务
      </button>
    </>,
  )

  const loadInstances = useCallback(async () => {
    if (!account) return
    setLoading(true)
    setError('')
    try {
      const list = await api.instances.list(account.id, {
        region: effectiveRegion,
        compartment_id: compartmentId || undefined,
      })
      setInstances(list)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }, [account, effectiveRegion, compartmentId])

  useEffect(() => {
    if (!account) return
    let cancelled = false
    api.resources
      .compartments(account.id, effectiveRegion)
      .then((list) => {
        if (cancelled) return
        setCompartments(list)
        setCompartmentId((prev) => (prev && list.some((c) => c.id === prev) ? prev : account.tenancy_ocid))
      })
      .catch(() => {
        if (!cancelled) setCompartmentId(account.tenancy_ocid)
      })
    return () => {
      cancelled = true
    }
  }, [account, effectiveRegion])

  useEffect(() => {
    void loadInstances()
  }, [loadInstances])

  async function runAction(instance: Instance, action: string) {
    if (!account) return
    setBusyId(instance.id)
    setError('')
    try {
      await api.instances.action(account.id, instance.id, action, effectiveRegion)
      await loadInstances()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusyId('')
    }
  }

  async function submitShape() {
    if (!account || !shapeTarget) return
    setBusyId(shapeTarget.id)
    setError('')
    try {
      await api.instances.updateShape(account.id, shapeTarget.id, shapeForm, effectiveRegion)
      setShapeTarget(null)
      await loadInstances()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusyId('')
    }
  }

  async function submitTerminate() {
    if (!account || !terminateTarget) return
    setBusyId(terminateTarget.id)
    setError('')
    try {
      await api.instances.terminate(account.id, terminateTarget.id, preserveBootVolume, effectiveRegion)
      setTerminateTarget(null)
      await loadInstances()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusyId('')
    }
  }

  function requestSort(key: string) {
    const k = key as SortKey
    setSort((s) => (s.key === k ? { key: k, dir: s.dir === 'asc' ? 'desc' : 'asc' } : { key: k, dir: 'asc' }))
  }

  const states = [...new Set(instances.map((i) => i.state))].sort()
  const filtered = instances.filter((i) => stateFilter === 'ALL' || i.state === stateFilter)
  const sorted = sort.key
    ? [...filtered].sort((a, b) => {
        const av = sortValue(a, sort.key as SortKey)
        const bv = sortValue(b, sort.key as SortKey)
        const cmp =
          typeof av === 'number' && typeof bv === 'number'
            ? av - bv
            : String(av).localeCompare(String(bv))
        return sort.dir === 'asc' ? cmp : -cmp
      })
    : filtered

  const { paged, pager } = usePager(sorted)

  const pageIds = paged.map((i) => i.id)
  const allSelected = pageIds.length > 0 && pageIds.every((id) => selected.has(id))

  function toggleAll(checked: boolean) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (checked) pageIds.forEach((id) => next.add(id))
      else pageIds.forEach((id) => next.delete(id))
      return next
    })
  }

  function toggleOne(id: string, checked: boolean) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (checked) next.add(id)
      else next.delete(id)
      return next
    })
  }

  if (!account) return null

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}

      <div className="flex flex-col items-start gap-4 lg:flex-row">
        {/* List Scope / Filtering */}
        <aside className="oci-list-scope w-full lg:w-auto">
          <div className="oci-scope-block">
            <div className="oci-scope-title">List scope</div>
            <label className="oci-label">COMPARTMENT</label>
            <select
              className="oci-select"
              value={compartmentId}
              onChange={(e) => setCompartmentId(e.target.value)}
            >
              {compartments.length === 0 && <option value={account.tenancy_ocid}>根区间</option>}
              {compartments.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </div>

          <div className="oci-scope-block">
            <div className="oci-scope-title">Filtering</div>
            <label className="oci-label">State</label>
            <select
              className="oci-select"
              value={stateFilter}
              onChange={(e) => setStateFilter(e.target.value)}
            >
              <option value="ALL">全部状态</option>
              {states.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </div>
        </aside>

        {/* 资源表 */}
        <div className="min-w-0 flex-1">
          <Card
            title="计算实例"
            extra={<span className="text-[13px] text-oracle-muted">{filtered.length} 个实例</span>}
            bodyClassName="oci-card-body--flush"
          >
            {loading && instances.length === 0 ? (
              <div className="p-6">
                <Loading />
              </div>
            ) : filtered.length === 0 ? (
              <EmptyState />
            ) : (
              <>
                <div className="oci-table-wrap">
                  <table className="oci-table">
                    <thead>
                      <tr>
                        <th className="oci-checkbox-cell">
                          <Checkbox checked={allSelected} onChange={toggleAll} />
                        </th>
                        <SortableTh
                          label="名称"
                          sortKey="name"
                          activeKey={sort.key}
                          dir={sort.dir}
                          onSort={requestSort}
                        />
                        <SortableTh
                          label="状态"
                          sortKey="state"
                          activeKey={sort.key}
                          dir={sort.dir}
                          onSort={requestSort}
                        />
                        <SortableTh
                          label="规格"
                          sortKey="shape"
                          activeKey={sort.key}
                          dir={sort.dir}
                          onSort={requestSort}
                        />
                        <SortableTh
                          label="OCPU / 内存"
                          sortKey="ocpus"
                          activeKey={sort.key}
                          dir={sort.dir}
                          onSort={requestSort}
                        />
                        <SortableTh
                          label="可用域"
                          sortKey="ad"
                          activeKey={sort.key}
                          dir={sort.dir}
                          onSort={requestSort}
                        />
                        <SortableTh
                          label="创建时间"
                          sortKey="created"
                          activeKey={sort.key}
                          dir={sort.dir}
                          onSort={requestSort}
                        />
                        <th className="oci-cell-actions">操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {paged.map((inst) => (
                        <tr key={inst.id} className={selected.has(inst.id) ? 'is-selected' : ''}>
                          <td className="oci-checkbox-cell">
                            <Checkbox
                              checked={selected.has(inst.id)}
                              onChange={(c) => toggleOne(inst.id, c)}
                            />
                          </td>
                          <td>
                            <div className="oci-link" title={inst.display_name || inst.id}>
                              {inst.display_name || '(未命名)'}
                            </div>
                            <div className="font-mono text-xs text-oracle-muted">
                              {inst.id.slice(-16)}
                            </div>
                          </td>
                          <td>
                            <Badge>{inst.state}</Badge>
                          </td>
                          <td className="font-mono text-xs">{inst.shape}</td>
                          <td>{inst.ocpus ? `${inst.ocpus} / ${inst.memory_gbs} GB` : '—'}</td>
                          <td className="text-xs text-oracle-muted">{inst.availability_domain}</td>
                          <td className="text-xs text-oracle-muted">
                            {formatDateTime(inst.time_created)}
                          </td>
                          <td className="oci-cell-actions">
                            <div className="flex justify-end">
                              <ActionsMenu
                                label="实例操作"
                                items={[
                                  {
                                    label: '启动',
                                    disabled: busyId === inst.id || inst.state === 'RUNNING',
                                    onSelect: () => void runAction(inst, 'start'),
                                  },
                                  {
                                    label: '停止',
                                    disabled: busyId === inst.id || inst.state !== 'RUNNING',
                                    onSelect: () => void runAction(inst, 'stop'),
                                  },
                                  {
                                    label: '重启',
                                    disabled: busyId === inst.id || inst.state !== 'RUNNING',
                                    onSelect: () => void runAction(inst, 'softreset'),
                                  },
                                  {
                                    label: '改规格',
                                    disabled: busyId === inst.id,
                                    separatorBefore: true,
                                    onSelect: () => {
                                      setShapeForm({
                                        ocpus: inst.ocpus || 4,
                                        memory_gbs: inst.memory_gbs || 24,
                                      })
                                      setShapeTarget(inst)
                                    },
                                  },
                                  {
                                    label: '终止',
                                    disabled: busyId === inst.id,
                                    danger: true,
                                    onSelect: () => setTerminateTarget(inst),
                                  },
                                ]}
                              />
                            </div>
                          </td>
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
      </div>

      {shapeTarget && (
        <Modal
          title={`调整规格：${shapeTarget.display_name || shapeTarget.id}`}
          onClose={() => setShapeTarget(null)}
          footer={
            <>
              <button type="button" className="oci-btn-secondary" onClick={() => setShapeTarget(null)}>
                取消
              </button>
              <button type="button" className="oci-btn-primary" onClick={() => void submitShape()}>
                应用
              </button>
            </>
          }
        >
          <div className="space-y-3">
            <p className="text-xs text-oracle-muted">
              仅 Flex 系列规格支持在线调整。Always Free 账号的 ARM 资源总额度受账号配额限制。
            </p>
            <Field label="OCPU 数量">
              <input
                type="number"
                min={1}
                className="oci-input"
                value={shapeForm.ocpus}
                onChange={(e) => setShapeForm((f) => ({ ...f, ocpus: Number(e.target.value) }))}
              />
            </Field>
            <Field label="内存（GB）">
              <input
                type="number"
                min={1}
                className="oci-input"
                value={shapeForm.memory_gbs}
                onChange={(e) => setShapeForm((f) => ({ ...f, memory_gbs: Number(e.target.value) }))}
              />
            </Field>
          </div>
        </Modal>
      )}

      {terminateTarget && (
        <Modal
          title={`终止实例：${terminateTarget.display_name || terminateTarget.id}`}
          onClose={() => setTerminateTarget(null)}
          footer={
            <>
              <button
                type="button"
                className="oci-btn-secondary"
                onClick={() => setTerminateTarget(null)}
              >
                取消
              </button>
              <button type="button" className="oci-btn-danger" onClick={() => void submitTerminate()}>
                确认终止
              </button>
            </>
          }
        >
          <div className="space-y-3">
            <p className="text-sm">终止后实例将被销毁，且不可恢复。</p>
            <Checkbox
              checked={preserveBootVolume}
              onChange={setPreserveBootVolume}
              label="保留启动卷"
            />
          </div>
        </Modal>
      )}
    </div>
  )
}
