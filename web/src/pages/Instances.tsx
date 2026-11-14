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
import { formatDateTime, truncate } from '../lib/format'
import type { BootVolume, Compartment, Instance, InstanceDetail, Vnic } from '../lib/types'

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

  // 重命名 / 实例详情抽屉
  const [renameTarget, setRenameTarget] = useState<Instance | null>(null)
  const [renameForm, setRenameForm] = useState({ display_name: '' })
  const [detailTarget, setDetailTarget] = useState<Instance | null>(null)
  const [detail, setDetail] = useState<InstanceDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState('')
  const [editBusy, setEditBusy] = useState('')
  const [bootVolumeEdit, setBootVolumeEdit] = useState<BootVolume | null>(null)
  const [bootVolumeForm, setBootVolumeForm] = useState({
    display_name: '',
    size_gbs: 50,
    vpus_per_gb: 10,
  })
  const [vnicEdit, setVnicEdit] = useState<Vnic | null>(null)
  const [vnicForm, setVnicForm] = useState({ display_name: '', hostname_label: '' })

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

  async function loadDetail(instanceId: string) {
    if (!account) return
    setDetailLoading(true)
    setDetailError('')
    try {
      const d = await api.instances.detail(account.id, instanceId, { region: effectiveRegion })
      setDetail(d)
    } catch (err) {
      setDetailError((err as Error).message)
    } finally {
      setDetailLoading(false)
    }
  }

  function openDetail(inst: Instance) {
    setDetailTarget(inst)
    setDetail(null)
    setDetailError('')
    void loadDetail(inst.id)
  }

  async function submitRename() {
    if (!account || !renameTarget) return
    setBusyId(renameTarget.id)
    setError('')
    try {
      await api.instances.rename(account.id, renameTarget.id, renameForm.display_name, effectiveRegion)
      setRenameTarget(null)
      await loadInstances()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusyId('')
    }
  }

  function openBootVolumeEdit(vol: BootVolume) {
    setBootVolumeForm({
      display_name: vol.display_name,
      size_gbs: vol.size_gbs,
      vpus_per_gb: vol.vpus_per_gb,
    })
    setBootVolumeEdit(vol)
  }

  async function submitBootVolume() {
    if (!account || !detailTarget || !bootVolumeEdit) return
    setEditBusy(bootVolumeEdit.id)
    setDetailError('')
    try {
      await api.storage.updateBootVolume(
        account.id,
        bootVolumeEdit.id,
        {
          display_name: bootVolumeForm.display_name,
          size_gbs: bootVolumeForm.size_gbs,
          vpus_per_gb: bootVolumeForm.vpus_per_gb,
        },
        { region: effectiveRegion },
      )
      setBootVolumeEdit(null)
      await loadDetail(detailTarget.id)
    } catch (err) {
      setDetailError((err as Error).message)
    } finally {
      setEditBusy('')
    }
  }

  function openVnicEdit(vnic: Vnic) {
    setVnicForm({ display_name: vnic.display_name, hostname_label: vnic.hostname_label })
    setVnicEdit(vnic)
  }

  async function submitVnic() {
    if (!account || !detailTarget || !vnicEdit) return
    setEditBusy(vnicEdit.id)
    setDetailError('')
    try {
      await api.network.updateVnic(
        account.id,
        vnicEdit.id,
        { display_name: vnicForm.display_name, hostname_label: vnicForm.hostname_label },
        { region: effectiveRegion },
      )
      setVnicEdit(null)
      await loadDetail(detailTarget.id)
    } catch (err) {
      setDetailError((err as Error).message)
    } finally {
      setEditBusy('')
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
                                    label: '查看详情',
                                    disabled: busyId === inst.id,
                                    onSelect: () => openDetail(inst),
                                  },
                                  {
                                    label: '重命名',
                                    disabled: busyId === inst.id,
                                    onSelect: () => {
                                      setRenameForm({ display_name: inst.display_name })
                                      setRenameTarget(inst)
                                    },
                                  },
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

      {renameTarget && (
        <Modal
          title={`重命名实例：${renameTarget.display_name || renameTarget.id}`}
          onClose={() => setRenameTarget(null)}
          footer={
            <>
              <button type="button" className="oci-btn-secondary" onClick={() => setRenameTarget(null)}>
                取消
              </button>
              <button
                type="button"
                className="oci-btn-primary"
                disabled={busyId === renameTarget.id || !renameForm.display_name.trim()}
                onClick={() => void submitRename()}
              >
                {busyId === renameTarget.id ? '保存中…' : '保存'}
              </button>
            </>
          }
        >
          <Field label="显示名称">
            <input
              className="oci-input"
              value={renameForm.display_name}
              onChange={(e) => setRenameForm((f) => ({ ...f, display_name: e.target.value }))}
            />
          </Field>
        </Modal>
      )}

      {detailTarget && (
        <Modal
          title={`实例详情：${detailTarget.display_name || detailTarget.id}`}
          onClose={() => setDetailTarget(null)}
          width="max-w-4xl"
          footer={
            <>
              <button
                type="button"
                className="oci-btn-secondary"
                onClick={() => void loadDetail(detailTarget.id)}
              >
                <Icon name="refresh" size={15} />
                刷新
              </button>
              <button type="button" className="oci-btn-primary" onClick={() => setDetailTarget(null)}>
                关闭
              </button>
            </>
          }
        >
          {detailError && <ErrorBanner message={detailError} onClose={() => setDetailError('')} />}
          {detailLoading && !detail ? (
            <Loading text="正在加载实例详情…" />
          ) : !detail ? (
            <EmptyState />
          ) : (
            <div className="space-y-4">
              <Card title="基本信息">
                <dl className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-3">
                  <div>
                    <dt className="text-xs text-oracle-muted">状态</dt>
                    <dd className="mt-1"><Badge>{detail.instance.state}</Badge></dd>
                  </div>
                  <div>
                    <dt className="text-xs text-oracle-muted">规格</dt>
                    <dd className="mt-1 font-mono text-xs">{detail.instance.shape}</dd>
                  </div>
                  <div>
                    <dt className="text-xs text-oracle-muted">OCPU / 内存</dt>
                    <dd className="mt-1">{detail.instance.ocpus} / {detail.instance.memory_gbs} GB</dd>
                  </div>
                  <div>
                    <dt className="text-xs text-oracle-muted">可用域</dt>
                    <dd className="mt-1 text-xs">{detail.instance.availability_domain}</dd>
                  </div>
                  <div>
                    <dt className="text-xs text-oracle-muted">区域</dt>
                    <dd className="mt-1 text-xs">{detail.instance.region}</dd>
                  </div>
                  <div>
                    <dt className="text-xs text-oracle-muted">创建时间</dt>
                    <dd className="mt-1 text-xs">{formatDateTime(detail.instance.time_created)}</dd>
                  </div>
                  <div>
                    <dt className="text-xs text-oracle-muted">区间 ID</dt>
                    <dd className="mt-1 break-all font-mono text-xs">{detail.instance.compartment_id}</dd>
                  </div>
                  <div className="sm:col-span-2">
                    <dt className="text-xs text-oracle-muted">实例 OCID</dt>
                    <dd className="mt-1 break-all font-mono text-xs">{detail.instance.id}</dd>
                  </div>
                </dl>
              </Card>

              <Card title="引导卷" bodyClassName="oci-card-body--flush">
                {detail.boot_volumes.length === 0 ? (
                  <EmptyState />
                ) : (
                  <div className="oci-table-wrap">
                    <table className="oci-table">
                      <thead>
                        <tr>
                          <th>名称</th>
                          <th>容量</th>
                          <th>VPU / GB</th>
                          <th>状态</th>
                          <th>可用域</th>
                          <th className="oci-cell-actions">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {detail.boot_volumes.map((vol) => (
                          <tr key={vol.id}>
                            <td className="font-medium">{vol.display_name || '(未命名)'}</td>
                            <td>{vol.size_gbs} GB</td>
                            <td>{vol.vpus_per_gb}</td>
                            <td><Badge>{vol.state}</Badge></td>
                            <td className="text-xs text-oracle-muted">{vol.availability_domain}</td>
                            <td className="oci-cell-actions">
                              <div className="flex justify-end">
                                <button
                                  type="button"
                                  className="oci-btn-secondary"
                                  disabled={editBusy === vol.id}
                                  onClick={() => openBootVolumeEdit(vol)}
                                >
                                  <Icon name="edit" size={15} />
                                  编辑
                                </button>
                              </div>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </Card>

              <Card title="VNIC" bodyClassName="oci-card-body--flush">
                {detail.vnics.length === 0 ? (
                  <EmptyState />
                ) : (
                  <div className="oci-table-wrap">
                    <table className="oci-table">
                      <thead>
                        <tr>
                          <th>名称</th>
                          <th>主机名标签</th>
                          <th>私有 IP</th>
                          <th>公网 IP</th>
                          <th>MAC</th>
                          <th>状态</th>
                          <th className="oci-cell-actions">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {detail.vnics.map((vnic) => (
                          <tr key={vnic.id}>
                            <td className="font-medium">{vnic.display_name || '(未命名)'}</td>
                            <td className="font-mono text-xs">{vnic.hostname_label || '—'}</td>
                            <td className="font-mono text-xs">{vnic.private_ip || '—'}</td>
                            <td className="font-mono text-xs">{vnic.public_ip || '—'}</td>
                            <td className="font-mono text-xs">{vnic.mac_address || '—'}</td>
                            <td><Badge>{vnic.state}</Badge></td>
                            <td className="oci-cell-actions">
                              <div className="flex justify-end">
                                <button
                                  type="button"
                                  className="oci-btn-secondary"
                                  disabled={editBusy === vnic.id}
                                  onClick={() => openVnicEdit(vnic)}
                                >
                                  <Icon name="edit" size={15} />
                                  改名
                                </button>
                              </div>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </Card>

              <Card title="公网 IP" bodyClassName="oci-card-body--flush">
                {detail.public_ips.length === 0 ? (
                  <EmptyState />
                ) : (
                  <div className="oci-table-wrap">
                    <table className="oci-table">
                      <thead>
                        <tr>
                          <th>IP 地址</th>
                          <th>名称</th>
                          <th>生命周期</th>
                          <th>状态</th>
                        </tr>
                      </thead>
                      <tbody>
                        {detail.public_ips.map((ip) => (
                          <tr key={ip.id}>
                            <td className="font-mono font-medium">{ip.ip_address}</td>
                            <td>{ip.display_name || '—'}</td>
                            <td className="text-xs">{ip.lifetime}</td>
                            <td><Badge>{ip.state}</Badge></td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </Card>

              <Card title="安全列表" bodyClassName="oci-card-body--flush">
                {detail.security_lists.length === 0 ? (
                  <EmptyState />
                ) : (
                  <div className="oci-table-wrap">
                    <table className="oci-table">
                      <thead>
                        <tr>
                          <th>名称</th>
                          <th>VCN ID</th>
                          <th>入站规则</th>
                          <th>出站规则</th>
                        </tr>
                      </thead>
                      <tbody>
                        {detail.security_lists.map((sl) => (
                          <tr key={sl.id}>
                            <td className="font-medium">{sl.display_name}</td>
                            <td className="font-mono text-xs" title={sl.vcn_id}>
                              {truncate(sl.vcn_id, 20, 0)}
                            </td>
                            <td>{sl.ingress_rules.length} 条</td>
                            <td>{sl.egress_rules.length} 条</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </Card>
            </div>
          )}
        </Modal>
      )}

      {bootVolumeEdit && (
        <Modal
          title={`编辑引导卷：${bootVolumeEdit.display_name || bootVolumeEdit.id}`}
          onClose={() => setBootVolumeEdit(null)}
          footer={
            <>
              <button type="button" className="oci-btn-secondary" onClick={() => setBootVolumeEdit(null)}>
                取消
              </button>
              <button
                type="button"
                className="oci-btn-primary"
                disabled={editBusy === bootVolumeEdit.id}
                onClick={() => void submitBootVolume()}
              >
                {editBusy === bootVolumeEdit.id ? '保存中…' : '保存'}
              </button>
            </>
          }
        >
          <div className="space-y-3">
            <Field label="显示名称">
              <input
                className="oci-input"
                value={bootVolumeForm.display_name}
                onChange={(e) => setBootVolumeForm((f) => ({ ...f, display_name: e.target.value }))}
              />
            </Field>
            <Field label="容量（GB）">
              <input
                type="number"
                min={1}
                className="oci-input"
                value={bootVolumeForm.size_gbs}
                onChange={(e) => setBootVolumeForm((f) => ({ ...f, size_gbs: Number(e.target.value) }))}
              />
            </Field>
            <Field label="VPU / GB" hint="可选值通常为 0 / 10 / 20 / 30 等">
              <input
                type="number"
                min={0}
                className="oci-input"
                value={bootVolumeForm.vpus_per_gb}
                onChange={(e) => setBootVolumeForm((f) => ({ ...f, vpus_per_gb: Number(e.target.value) }))}
              />
            </Field>
          </div>
        </Modal>
      )}

      {vnicEdit && (
        <Modal
          title={`编辑 VNIC：${vnicEdit.display_name || vnicEdit.id}`}
          onClose={() => setVnicEdit(null)}
          footer={
            <>
              <button type="button" className="oci-btn-secondary" onClick={() => setVnicEdit(null)}>
                取消
              </button>
              <button
                type="button"
                className="oci-btn-primary"
                disabled={editBusy === vnicEdit.id}
                onClick={() => void submitVnic()}
              >
                {editBusy === vnicEdit.id ? '保存中…' : '保存'}
              </button>
            </>
          }
        >
          <div className="space-y-3">
            <Field label="显示名称">
              <input
                className="oci-input"
                value={vnicForm.display_name}
                onChange={(e) => setVnicForm((f) => ({ ...f, display_name: e.target.value }))}
              />
            </Field>
            <Field label="主机名标签">
              <input
                className="oci-input"
                value={vnicForm.hostname_label}
                onChange={(e) => setVnicForm((f) => ({ ...f, hostname_label: e.target.value }))}
              />
            </Field>
          </div>
        </Modal>
      )}
    </div>
  )
}
