import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { AnchoredMenu } from '../components/AnchoredMenu'
import { usePageAction } from '../components/Layout'
import { OtpConfirm } from '../components/OtpConfirm'
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
  Notice,
  SortableTh,
  SuccessBanner,
  usePager,
  type ActionItem,
} from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { formatDateTime, truncate } from '../lib/format'
import type {
  BootVolume,
  Compartment,
  Instance,
  InstanceDetail,
  IpRotationSettings,
  PublicIp,
  RotateIpResult,
  SecurityList,
  SecurityRule,
  Vnic,
} from '../lib/types'

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

type DetailTab = 'info' | 'network' | 'storage' | 'security' | 'management' | 'tags'

const DETAIL_TABS: { id: DetailTab; label: string }[] = [
  { id: 'info', label: '详细信息' },
  { id: 'network', label: '网络' },
  { id: 'storage', label: '存储' },
  { id: 'security', label: '安全' },
  { id: 'management', label: '管理' },
  { id: 'tags', label: '标记' },
]

/** 官网风格「操作 ▾」下拉：同样复用 portal 菜单，不会被卡片裁剪 */
function ActionDropdown({ items, label = '操作' }: { items: ActionItem[]; label?: string }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLButtonElement>(null)

  return (
    <span className="oci-actions">
      <button
        ref={ref}
        type="button"
        className="oci-btn-secondary"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        {label}
        <Icon name="chevron-down" size={14} />
      </button>
      <AnchoredMenu open={open} anchorRef={ref} onClose={() => setOpen(false)}>
        {items.map((it, i) => (
          <div key={i}>
            {it.separatorBefore && <div className="oci-actions-sep" />}
            <button
              type="button"
              className={`oci-actions-item ${it.danger ? 'is-danger' : ''}`}
              disabled={it.disabled}
              onClick={() => {
                setOpen(false)
                it.onSelect()
              }}
            >
              {it.label}
            </button>
          </div>
        ))}
      </AnchoredMenu>
    </span>
  )
}

/** 双栏键值明细中的一行 */
function KvRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="oci-kv-row">
      <div className="oci-kv-label">{label}</div>
      <div className="oci-kv-value">{children}</div>
    </div>
  )
}

function copyText(text: string) {
  if (navigator.clipboard?.writeText) void navigator.clipboard.writeText(text)
}

const STATE_TEXT: Record<string, string> = {
  RUNNING: '正在运行',
  STOPPED: '已停止',
  STOPPING: '正在停止',
  STARTING: '正在启动',
  PROVISIONING: '正在预配',
  TERMINATED: '已终止',
  TERMINATING: '正在终止',
  CREATING: '正在创建',
}

function stateText(state: string) {
  return STATE_TEXT[state] ?? state
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

  // 重命名 / 实例详情
  const [renameTarget, setRenameTarget] = useState<Instance | null>(null)
  const [renameForm, setRenameForm] = useState({ display_name: '' })
  const [detailTarget, setDetailTarget] = useState<Instance | null>(null)
  const [detailTab, setDetailTab] = useState<DetailTab>('info')
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

  // 列表筛选、行选择、表头排序
  const [stateFilter, setStateFilter] = useState('ALL')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [sort, setSort] = useState<{ key: SortKey | ''; dir: 'asc' | 'desc' }>({
    key: '',
    dir: 'asc',
  })
  // 搜索和筛选：输入框草稿 / 已应用的搜索词；视图图标控制行辅助信息
  const [searchInput, setSearchInput] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [showOcid, setShowOcid] = useState(true)

  // 更换公网 IP：二次确认 → OTP 验证 → 结果
  const [rotation, setRotation] = useState<IpRotationSettings | null>(null)
  const [rotateConfirm, setRotateConfirm] = useState<PublicIp | null>(null)
  const [rotateOtp, setRotateOtp] = useState<PublicIp | null>(null)
  const [rotateResult, setRotateResult] = useState<RotateIpResult | null>(null)

  // 安全列表规则编辑（以 JSON 数组提交）
  const [securityListEdit, setSecurityListEdit] = useState<SecurityList | null>(null)
  const [securityListForm, setSecurityListForm] = useState({ ingress: '[]', egress: '[]' })
  const [securityListParseError, setSecurityListParseError] = useState('')

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

  async function bulkAction(action: string) {
    const targets = instances.filter((i) => selected.has(i.id))
    for (const inst of targets) await runAction(inst, action)
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
    setDetailTab('info')
    setDetail(null)
    setDetailError('')
    setBootVolumeEdit(null)
    setVnicEdit(null)
    setSecurityListEdit(null)
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
    setDetailError('')
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
    setDetailError('')
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

  function openRotateConfirm(ip: PublicIp) {
    setRotateConfirm(ip)
    api.ipRotation
      .get()
      .then(setRotation)
      .catch(() => setRotation(null))
  }

  function proceedRotate() {
    const ip = rotateConfirm
    setRotateConfirm(null)
    setRotateOtp(ip)
  }

  function openSecurityListEdit(sl: SecurityList) {
    setSecurityListForm({
      ingress: JSON.stringify(sl.ingress_rules, null, 2),
      egress: JSON.stringify(sl.egress_rules, null, 2),
    })
    setSecurityListParseError('')
    setDetailError('')
    setSecurityListEdit(sl)
  }

  async function submitSecurityList() {
    if (!account || !detailTarget || !securityListEdit) return
    let ingress_rules: SecurityRule[]
    let egress_rules: SecurityRule[]
    try {
      const inParsed: unknown = JSON.parse(securityListForm.ingress)
      const outParsed: unknown = JSON.parse(securityListForm.egress)
      if (!Array.isArray(inParsed) || !Array.isArray(outParsed)) {
        throw new Error('入站与出站规则都必须是数组')
      }
      ingress_rules = inParsed as SecurityRule[]
      egress_rules = outParsed as SecurityRule[]
    } catch (err) {
      setSecurityListParseError((err as Error).message)
      return
    }
    setSecurityListParseError('')
    setEditBusy(securityListEdit.id)
    setDetailError('')
    try {
      await api.network.updateSecurityList(
        account.id,
        securityListEdit.id,
        { ingress_rules, egress_rules },
        { region: effectiveRegion },
      )
      setSecurityListEdit(null)
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

  function applySearch() {
    setAppliedSearch(searchInput.trim())
  }

  function clearSearch() {
    setSearchInput('')
    setAppliedSearch('')
  }

  const states = [...new Set(instances.map((i) => i.state))].sort()
  const filtered = instances.filter((i) => {
    if (stateFilter !== 'ALL' && i.state !== stateFilter) return false
    if (appliedSearch) {
      const q = appliedSearch.toLowerCase()
      const hay = [i.display_name, i.shape, i.id, i.availability_domain, i.state]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
      if (!hay.includes(q)) return false
    }
    return true
  })
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

  const listActions: ActionItem[] = [
    { label: '刷新列表', onSelect: () => void loadInstances() },
    { label: '创建抢购任务', onSelect: () => navigate('/grabber') },
    { label: '全选本页', disabled: paged.length === 0, onSelect: () => toggleAll(true) },
    { label: '清除所选', disabled: selected.size === 0, onSelect: () => setSelected(new Set()) },
    {
      label: '启动所选',
      separatorBefore: true,
      disabled: selected.size === 0,
      onSelect: () => void bulkAction('start'),
    },
    { label: '停止所选', disabled: selected.size === 0, onSelect: () => void bulkAction('stop') },
    { label: '重启所选', disabled: selected.size === 0, onSelect: () => void bulkAction('softreset') },
  ]

  function renderList() {
    return (
      <>
        {/* 白底工具栏：紧接 Layout 统一渲染的彩条，形成无缝卡片 */}
        <div className="oci-toolbar">
          <div className="flex items-center justify-between gap-3">
            <div className="oci-toolbar-search">
              <input
                className="oci-input"
                placeholder="搜索和筛选"
                aria-label="搜索和筛选"
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') applySearch()
                }}
              />
              <button type="button" className="oci-btn-secondary" onClick={applySearch}>
                <Icon name="search" size={15} />
                搜索
              </button>
            </div>
            <button
              type="button"
              className="oci-toolbar-view"
              aria-pressed={showOcid}
              title={showOcid ? '隐藏行辅助信息' : '显示行辅助信息'}
              aria-label={showOcid ? '隐藏行辅助信息' : '显示行辅助信息'}
              onClick={() => setShowOcid((v) => !v)}
            >
              <Icon name="sliders" size={16} />
            </button>
          </div>

          <div className="oci-filter-row">
            <span className="oci-filter-label">应用的筛选器</span>
            <span className="oci-filter-chip">
              区间：
              <select
                className="max-w-[190px] border-0 bg-transparent text-xs outline-none"
                value={compartmentId}
                onChange={(e) => setCompartmentId(e.target.value)}
                aria-label="区间"
              >
                {compartments.length === 0 && (
                  <option value={account!.tenancy_ocid}>根区间</option>
                )}
                {compartments.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            </span>
            <span className="oci-filter-chip">
              状态：
              <select
                className="max-w-[130px] border-0 bg-transparent text-xs outline-none"
                value={stateFilter}
                onChange={(e) => setStateFilter(e.target.value)}
                aria-label="状态"
              >
                <option value="ALL">全部状态</option>
                {states.map((s) => (
                  <option key={s} value={s}>
                    {stateText(s)}
                  </option>
                ))}
              </select>
              {stateFilter !== 'ALL' && (
                <button
                  type="button"
                  aria-label="清除状态筛选"
                  title="清除状态筛选"
                  onClick={() => setStateFilter('ALL')}
                >
                  <Icon name="close" size={12} />
                </button>
              )}
            </span>
            {appliedSearch && (
              <span className="oci-filter-chip">
                名称：{appliedSearch}
                <button type="button" aria-label="清除名称筛选" title="清除名称筛选" onClick={clearSearch}>
                  <Icon name="close" size={12} />
                </button>
              </span>
            )}
          </div>

          <div className="oci-toolbar-actions">
            <button
              type="button"
              className="oci-btn-primary"
              title="通过抢购任务创建实例：先做容量预检，有货才发起创建请求"
              onClick={() => navigate('/grabber', { state: { openCreate: true } })}
            >
              <Icon name="plus" size={15} />
              创建实例
            </button>
            <ActionDropdown label="操作" items={listActions} />
          </div>
        </div>

        {/* 资源表 */}
        <div className="mt-4">
          {loading && instances.length === 0 ? (
            <div className="oci-panel oci-panel-pad">
              <Loading />
            </div>
          ) : filtered.length === 0 ? (
            <div className="oci-panel oci-panel-pad">
              <EmptyState />
            </div>
          ) : (
            <>
              <div className="oci-table-wrap">
                <table className="oci-table">
                  <thead>
                    <tr>
                      <th className="oci-checkbox-cell">
                        <Checkbox checked={allSelected} onChange={toggleAll} />
                      </th>
                      <SortableTh label="名称" sortKey="name" activeKey={sort.key} dir={sort.dir} onSort={requestSort} />
                      <SortableTh label="状态" sortKey="state" activeKey={sort.key} dir={sort.dir} onSort={requestSort} />
                      <th>公共 IP</th>
                      <th>专用 IP</th>
                      <SortableTh label="配置" sortKey="shape" activeKey={sort.key} dir={sort.dir} onSort={requestSort} />
                      <SortableTh label="OCPU 计数" sortKey="ocpus" activeKey={sort.key} dir={sort.dir} onSort={requestSort} />
                      <th>内存 (GB)</th>
                      <SortableTh label="可用性域" sortKey="ad" activeKey={sort.key} dir={sort.dir} onSort={requestSort} />
                      <th>容错域</th>
                      <SortableTh label="创建时间" sortKey="created" activeKey={sort.key} dir={sort.dir} onSort={requestSort} />
                      <th className="oci-cell-actions">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {paged.map((inst) => (
                      <tr key={inst.id} className={selected.has(inst.id) ? 'is-selected' : ''}>
                        <td className="oci-checkbox-cell">
                          <Checkbox checked={selected.has(inst.id)} onChange={(c) => toggleOne(inst.id, c)} />
                        </td>
                        <td>
                          <button
                            type="button"
                            className="oci-link text-left"
                            title={inst.display_name || inst.id}
                            onClick={() => openDetail(inst)}
                          >
                            {inst.display_name || '(未命名)'}
                          </button>
                          {showOcid && (
                            <div className="font-mono text-xs text-oracle-muted">
                              {truncate(inst.id, 16, 8)}
                            </div>
                          )}
                        </td>
                        <td>
                          <Badge>{inst.state}</Badge>
                        </td>
                        <td className="text-oracle-muted">—</td>
                        <td className="text-oracle-muted">—</td>
                        <td className="font-mono text-xs">{inst.shape}</td>
                        <td>{inst.ocpus || '—'}</td>
                        <td>{inst.memory_gbs || '—'}</td>
                        <td className="text-xs text-oracle-muted">{inst.availability_domain}</td>
                        <td className="text-oracle-muted">—</td>
                        <td className="text-xs text-oracle-muted">{formatDateTime(inst.time_created)}</td>
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
        </div>
      </>
    )
  }

  function renderDetail(inst: Instance) {
    const detailActions: ActionItem[] = [
      { label: '刷新详情', onSelect: () => void loadDetail(inst.id) },
      {
        label: '启动',
        separatorBefore: true,
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
        separatorBefore: true,
        disabled: busyId === inst.id,
        onSelect: () => {
          setShapeForm({ ocpus: inst.ocpus || 4, memory_gbs: inst.memory_gbs || 24 })
          setShapeTarget(inst)
        },
      },
      {
        label: '重命名',
        onSelect: () => {
          setRenameForm({ display_name: inst.display_name })
          setRenameTarget(inst)
        },
      },
      {
        label: '终止',
        separatorBefore: true,
        danger: true,
        disabled: busyId === inst.id,
        onSelect: () => setTerminateTarget(inst),
      },
    ]

    return (
      <div className="oci-panel">
        <div className="oci-panel-pad">
          <button
            type="button"
            className="oci-link inline-flex items-center gap-1 text-[13px]"
            onClick={() => {
              setDetailTarget(null)
              setDetailTab('info')
              setBootVolumeEdit(null)
              setVnicEdit(null)
              setSecurityListEdit(null)
            }}
          >
            <Icon name="chevron-left" size={13} />
            实例
          </button>

          <div className="oci-detail-head mt-3">
            <div className="min-w-0">
              <h2 className="oci-detail-title">
                <span className="truncate">{inst.display_name || '(未命名)'}</span>
                <Badge>{inst.state}</Badge>
              </h2>
              <div className="oci-detail-sub flex items-center gap-2">
                <span>实例 OCID</span>
                <span className="oci-ocid min-w-0 flex-1" title={inst.id}>
                  {inst.id}
                </span>
                <button type="button" className="oci-copy-btn" onClick={() => copyText(inst.id)}>
                  复制
                </button>
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <ActionDropdown label="操作" items={detailActions} />
              <button
                type="button"
                className="oci-btn-primary"
                onClick={() => {
                  setRenameForm({ display_name: inst.display_name })
                  setRenameTarget(inst)
                }}
              >
                <Icon name="edit" size={15} />
                编辑
              </button>
            </div>
          </div>
        </div>

        <div className="oci-page-tabs">
          {DETAIL_TABS.map((t) => (
            <button
              key={t.id}
              type="button"
              className={`oci-page-tab ${detailTab === t.id ? 'active' : ''}`}
              onClick={() => setDetailTab(t.id)}
            >
              {t.label}
            </button>
          ))}
        </div>

        {detailError && (
          <div className="oci-panel-pad pb-0">
            <ErrorBanner message={detailError} onClose={() => setDetailError('')} />
          </div>
        )}

        {detailLoading && !detail ? (
          <div className="oci-panel-pad">
            <Loading text="正在加载实例详情…" />
          </div>
        ) : !detail ? (
          <div className="oci-panel-pad">
            <EmptyState />
          </div>
        ) : detailTab === 'info' ? (
          <div className="oci-kv-grid">
            <div className="oci-kv-section">
              <div className="oci-kv-title">实例信息</div>
              <KvRow label="状态">
                <Badge>{detail.instance.state}</Badge>
              </KvRow>
              <KvRow label="配置">
                <span className="font-mono text-xs">{detail.instance.shape}</span>
              </KvRow>
              <KvRow label="OCPU 计数">{detail.instance.ocpus || '—'}</KvRow>
              <KvRow label="内存 (GB)">{detail.instance.memory_gbs || '—'}</KvRow>
              <KvRow label="映像 ID">
                <span className="oci-ocid min-w-0 flex-1" title={detail.instance.image_id}>
                  {detail.instance.image_id || '—'}
                </span>
              </KvRow>
              <KvRow label="创建时间">{formatDateTime(detail.instance.time_created)}</KvRow>
            </div>
            <div className="oci-kv-section">
              <div className="oci-kv-title">位置与标识</div>
              <KvRow label="可用性域">{detail.instance.availability_domain || '—'}</KvRow>
              <KvRow label="容错域">—</KvRow>
              <KvRow label="区域">{detail.instance.region || '—'}</KvRow>
              <KvRow label="区间 OCID">
                <span className="oci-ocid min-w-0 flex-1" title={detail.instance.compartment_id}>
                  {detail.instance.compartment_id || '—'}
                </span>
                <button
                  type="button"
                  className="oci-copy-btn"
                  onClick={() => copyText(detail.instance.compartment_id)}
                >
                  复制
                </button>
              </KvRow>
              <KvRow label="实例 OCID">
                <span className="oci-ocid min-w-0 flex-1" title={detail.instance.id}>
                  {detail.instance.id}
                </span>
                <button type="button" className="oci-copy-btn" onClick={() => copyText(detail.instance.id)}>
                  复制
                </button>
              </KvRow>
              <KvRow label="公共 IP">—</KvRow>
              <KvRow label="专用 IP">—</KvRow>
              <p className="oci-kv-note">
                列表与详情接口未提供公共 IP、专用 IP 与容错域字段，因此显示为「—」，不会编造数据。
              </p>
            </div>
          </div>
        ) : (
          <div className="oci-panel-pad space-y-4">
            {detailTab === 'network' && (
              <>
                <Card title="虚拟网卡（VNIC）" bodyClassName="oci-card-body--flush">
                  {detail.vnics.length === 0 ? (
                    <EmptyState />
                  ) : (
                    <div className="oci-table-wrap">
                      <table className="oci-table">
                        <thead>
                          <tr>
                            <th>名称</th>
                            <th>主机名标签</th>
                            <th>专用 IP</th>
                            <th>公共 IP</th>
                            <th>MAC 地址</th>
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
                              <td>
                                <Badge>{vnic.state}</Badge>
                              </td>
                              <td className="oci-cell-actions">
                                <div className="flex justify-end">
                                  <button
                                    type="button"
                                    className="oci-btn-secondary"
                                    disabled={editBusy === vnic.id}
                                    onClick={() => openVnicEdit(vnic)}
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
                            <th className="oci-cell-actions">操作</th>
                          </tr>
                        </thead>
                        <tbody>
                          {detail.public_ips.map((ip) => (
                            <tr key={ip.id}>
                              <td className="font-mono font-medium">{ip.ip_address || '—'}</td>
                              <td>{ip.display_name || '—'}</td>
                              <td className="text-xs">{ip.lifetime || '—'}</td>
                              <td>
                                <Badge>{ip.state}</Badge>
                              </td>
                              <td className="oci-cell-actions">
                                <div className="flex justify-end">
                                  <button
                                    type="button"
                                    className="oci-btn-secondary"
                                    disabled={!ip.private_ip_id}
                                    title={!ip.private_ip_id ? '该地址无私用 IP 关联，无法更换' : '更换公网 IP'}
                                    onClick={() => openRotateConfirm(ip)}
                                  >
                                    <Icon name="rotate" size={15} />
                                    更换公网 IP
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
              </>
            )}

            {detailTab === 'storage' && (
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
                          <th>每 GB 的 VPU</th>
                          <th>状态</th>
                          <th>可用性域</th>
                          <th className="oci-cell-actions">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {detail.boot_volumes.map((vol) => (
                          <tr key={vol.id}>
                            <td className="font-medium">{vol.display_name || '(未命名)'}</td>
                            <td>{vol.size_gbs} GB</td>
                            <td>{vol.vpus_per_gb}</td>
                            <td>
                              <Badge>{vol.state}</Badge>
                            </td>
                            <td className="text-xs text-oracle-muted">{vol.availability_domain || '—'}</td>
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
            )}

            {detailTab === 'security' && (
              <Card title="安全列表" bodyClassName="oci-card-body--flush">
                {detail.security_lists.length === 0 ? (
                  <EmptyState />
                ) : (
                  <div className="oci-table-wrap">
                    <table className="oci-table">
                      <thead>
                        <tr>
                          <th>名称</th>
                          <th>所属 VCN</th>
                          <th>入站规则</th>
                          <th>出站规则</th>
                          <th className="oci-cell-actions">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {detail.security_lists.map((sl) => (
                          <tr key={sl.id}>
                            <td className="font-medium">{sl.display_name || '(未命名)'}</td>
                            <td className="font-mono text-xs" title={sl.vcn_id}>
                              {truncate(sl.vcn_id, 20, 0)}
                            </td>
                            <td>{sl.ingress_rules.length} 条</td>
                            <td>{sl.egress_rules.length} 条</td>
                            <td className="oci-cell-actions">
                              <div className="flex justify-end">
                                <button
                                  type="button"
                                  className="oci-btn-secondary"
                                  disabled={editBusy === sl.id}
                                  onClick={() => openSecurityListEdit(sl)}
                                >
                                  <Icon name="edit" size={15} />
                                  编辑规则
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
            )}

            {detailTab === 'management' && (
              <div className="oci-panel oci-panel-pad">
                <div className="oci-section-title">实例操作</div>
                <div className="flex flex-wrap gap-2">
                  <button
                    type="button"
                    className="oci-btn-secondary"
                    disabled={busyId === inst.id || inst.state === 'RUNNING'}
                    onClick={() => void runAction(inst, 'start')}
                  >
                    启动
                  </button>
                  <button
                    type="button"
                    className="oci-btn-secondary"
                    disabled={busyId === inst.id || inst.state !== 'RUNNING'}
                    onClick={() => void runAction(inst, 'stop')}
                  >
                    停止
                  </button>
                  <button
                    type="button"
                    className="oci-btn-secondary"
                    disabled={busyId === inst.id || inst.state !== 'RUNNING'}
                    onClick={() => void runAction(inst, 'softreset')}
                  >
                    重启
                  </button>
                  <button
                    type="button"
                    className="oci-btn-secondary"
                    disabled={busyId === inst.id}
                    onClick={() => {
                      setShapeForm({ ocpus: inst.ocpus || 4, memory_gbs: inst.memory_gbs || 24 })
                      setShapeTarget(inst)
                    }}
                  >
                    改规格
                  </button>
                  <button
                    type="button"
                    className="oci-btn-secondary"
                    onClick={() => {
                      setRenameForm({ display_name: inst.display_name })
                      setRenameTarget(inst)
                    }}
                  >
                    重命名
                  </button>
                  <button
                    type="button"
                    className="oci-btn-danger"
                    disabled={busyId === inst.id}
                    onClick={() => setTerminateTarget(inst)}
                  >
                    终止
                  </button>
                </div>
                <p className="oci-inline-hint mt-4">
                  区间 OCID：{detail.instance.compartment_id || '—'}；区域：{detail.instance.region || '—'}；
                  创建时间：{formatDateTime(detail.instance.time_created)}。所有写操作都会调用对应的 OCI 接口并刷新详情。
                </p>
              </div>
            )}

            {detailTab === 'tags' && (
              <div className="oci-panel oci-panel-pad">
                <EmptyState text="未找到任何项目。" />
                <p className="oci-inline-hint text-center">
                  当前接口未提供标记数据，因此此页签不展示任何内容。
                </p>
              </div>
            )}
          </div>
        )}
      </div>
    )
  }

  if (!account) return null

  return (
    <>
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}
      {rotateResult && (
        <SuccessBanner
          message={`公网 IP 已更换：${rotateResult.new_ip.ip_address}。${rotateResult.message}`}
          onClose={() => setRotateResult(null)}
        />
      )}

      {detailTarget ? renderDetail(detailTarget) : renderList()}

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
            {error && <ErrorBanner message={error} onClose={() => setError('')} />}
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
          title={`编辑实例：${renameTarget.display_name || renameTarget.id}`}
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
          <div className="space-y-3">
            {error && <ErrorBanner message={error} onClose={() => setError('')} />}
            <Field label="显示名称">
              <input
                className="oci-input"
                value={renameForm.display_name}
                onChange={(e) => setRenameForm((f) => ({ ...f, display_name: e.target.value }))}
              />
            </Field>
          </div>
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
            {detailError && <ErrorBanner message={detailError} onClose={() => setDetailError('')} />}
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
            <Field label="每 GB 的 VPU" hint="可选值通常为 0 / 10 / 20 / 30 等">
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
            {detailError && <ErrorBanner message={detailError} onClose={() => setDetailError('')} />}
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

      {securityListEdit && (
        <Modal
          title={`编辑安全列表：${securityListEdit.display_name || securityListEdit.id}`}
          onClose={() => setSecurityListEdit(null)}
          width="max-w-2xl"
          footer={
            <>
              <button type="button" className="oci-btn-secondary" onClick={() => setSecurityListEdit(null)}>
                取消
              </button>
              <button
                type="button"
                className="oci-btn-primary"
                disabled={editBusy === securityListEdit.id}
                onClick={() => void submitSecurityList()}
              >
                {editBusy === securityListEdit.id ? '保存中…' : '保存'}
              </button>
            </>
          }
        >
          <div className="space-y-3">
            <p className="oci-inline-hint">
              以 JSON 数组编辑规则；保存后将调用 OCI 接口更新该安全列表的入站与出站规则。
            </p>
            {detailError && <ErrorBanner message={detailError} onClose={() => setDetailError('')} />}
            {securityListParseError && (
              <ErrorBanner
                message={`规则格式错误：${securityListParseError}`}
                onClose={() => setSecurityListParseError('')}
              />
            )}
            <Field label="入站规则（JSON 数组）">
              <textarea
                className="oci-input font-mono text-xs"
                rows={8}
                value={securityListForm.ingress}
                onChange={(e) => setSecurityListForm((f) => ({ ...f, ingress: e.target.value }))}
              />
            </Field>
            <Field label="出站规则（JSON 数组）">
              <textarea
                className="oci-input font-mono text-xs"
                rows={8}
                value={securityListForm.egress}
                onChange={(e) => setSecurityListForm((f) => ({ ...f, egress: e.target.value }))}
              />
            </Field>
          </div>
        </Modal>
      )}

      {/* 换 IP 第一级：二次确认 */}
      {rotateConfirm && (
        <Modal
          title="更换公网 IP"
          onClose={() => setRotateConfirm(null)}
          footer={
            <>
              <button type="button" className="oci-btn-secondary" onClick={() => setRotateConfirm(null)}>
                取消
              </button>
              <button type="button" className="oci-btn-danger" onClick={proceedRotate}>
                <Icon name="rotate" size={15} />
                仍要更换
              </button>
            </>
          }
        >
          <div className="space-y-4">
            <Notice tone="danger">
              <strong>请确认以下风险后再继续：</strong>
              <br />· DNS 解析记录不会自动更新，更换后请自行同步所有解析。
              <br />· 此操作不可逆，旧 IP 将立即释放且无法找回。
            </Notice>
            <div className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
              <div>
                <div className="text-xs text-oracle-muted">当前公网 IP</div>
                <div className="mt-1 font-mono">{rotateConfirm.ip_address || '—'}</div>
              </div>
              <div>
                <div className="text-xs text-oracle-muted">生命周期</div>
                <div className="mt-1">{rotateConfirm.lifetime || '—'}</div>
              </div>
              <div className="sm:col-span-2">
                <div className="text-xs text-oracle-muted">关联的专用 IP ID</div>
                <div className="mt-1 break-all font-mono text-xs">
                  {rotateConfirm.private_ip_id || '—'}
                </div>
              </div>
            </div>
            {rotation && (
              <p className="oci-inline-hint">
                今日剩余 {rotation.remaining_today} 次 / 每日上限 {rotation.daily_limit} 次，冷却时间{' '}
                {rotation.cooldown_sec} 秒。
              </p>
            )}
          </div>
        </Modal>
      )}

      {/* 换 IP 第二级：OTP 二次确认 */}
      {rotateOtp && (
        <OtpConfirm
          title="验证并更换公网 IP"
          operation="rotate_public_ip"
          summary={`更换公网 IP（当前 ${rotateOtp.ip_address}）`}
          confirmLabel="确认更换"
          onClose={() => setRotateOtp(null)}
          onConfirm={async (otp) => {
            if (!account || !detailTarget) return
            const res = await api.network.rotateIp(account.id, {
              private_ip_id: rotateOtp.private_ip_id,
              otp,
            })
            setRotateResult(res)
            await loadDetail(detailTarget.id)
          }}
        >
          <Notice tone="warning">
            DNS 解析记录不会自动更新；此操作不可逆，旧 IP 释放后无法找回。
          </Notice>
        </OtpConfirm>
      )}
    </>
  )
}
