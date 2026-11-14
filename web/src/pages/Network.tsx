import { useCallback, useEffect, useState } from 'react'
import { OtpConfirm } from '../components/OtpConfirm'
import { usePageAction } from '../components/Layout'
import {
  Badge,
  Card,
  EmptyState,
  ErrorBanner,
  Field,
  Icon,
  Loading,
  Modal,
  Notice,
  Stat,
  SuccessBanner,
  Switch,
} from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { formatDateTime, truncate } from '../lib/format'
import type {
  IpRotationSettings,
  PublicIp,
  RotateIpResult,
  SecurityList,
  SecurityRule,
  Vnic,
} from '../lib/types'

type Tab = 'vnics' | 'ips' | 'security'

const EMPTY_VNIC_FORM = { display_name: '', hostname_label: '' }

const EMPTY_RULE_FORM = {
  protocol: 'tcp',
  source: '0.0.0.0/0',
  source_type: 'CIDR_BLOCK',
  destination: '0.0.0.0/0',
  destination_type: 'CIDR_BLOCK',
  tcp_port_min: '80',
  tcp_port_max: '80',
  udp_port_min: '',
  udp_port_max: '',
  description: '',
  stateless: false,
}

type RuleForm = typeof EMPTY_RULE_FORM

function formatPorts(rule: SecurityRule): string {
  const parts: string[] = []
  if (rule.tcp_port_min !== undefined) {
    const min = rule.tcp_port_min
    const max = rule.tcp_port_max
    parts.push(`TCP ${min}${max !== undefined && max !== min ? `-${max}` : ''}`)
  }
  if (rule.udp_port_min !== undefined) {
    const min = rule.udp_port_min
    const max = rule.udp_port_max
    parts.push(`UDP ${min}${max !== undefined && max !== min ? `-${max}` : ''}`)
  }
  return parts.length > 0 ? parts.join(' / ') : '全部端口'
}

function RotationInfo({ rotation }: { rotation: IpRotationSettings | null }) {
  if (!rotation) return null
  return (
    <Card title="换 IP 保护策略" bodyClassName="oci-card-body--compact">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat label="今日剩余次数" value={rotation.remaining_today} accent="brand" />
        <Stat label="每日上限" value={rotation.daily_limit} />
        <Stat label="今日已用" value={rotation.used_today} />
        <Stat label="冷却时间" value={rotation.cooldown_sec} unit="秒" />
      </div>
      <p className="mt-3 text-xs text-oracle-muted">
        下次允许更换时间：{formatDateTime(rotation.next_allowed_at)}
      </p>
    </Card>
  )
}

function RuleTable({
  title,
  rules,
  onAdd,
  onDelete,
  disabled,
}: {
  title: string
  rules: SecurityRule[]
  onAdd: () => void
  onDelete: (index: number) => void
  disabled: boolean
}) {
  return (
    <Card
      title={title}
      extra={
        <button
          type="button"
          className="oci-btn-secondary"
          onClick={onAdd}
          disabled={disabled}
        >
          <Icon name="plus" size={15} />
          添加规则
        </button>
      }
      bodyClassName="oci-card-body--flush"
    >
      {rules.length === 0 ? (
        <EmptyState />
      ) : (
        <div className="oci-table-wrap">
          <table className="oci-table">
            <thead>
              <tr>
                <th>协议</th>
                <th>源</th>
                <th>源类型</th>
                <th>目的</th>
                <th>目的类型</th>
                <th>端口范围</th>
                <th>描述</th>
                <th>无状态</th>
                <th className="oci-cell-actions">操作</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((rule, index) => (
                <tr key={index}>
                  <td className="font-mono text-xs">{rule.protocol}</td>
                  <td className="font-mono text-xs">{rule.source || '—'}</td>
                  <td className="text-xs">{rule.source_type || '—'}</td>
                  <td className="font-mono text-xs">{rule.destination || '—'}</td>
                  <td className="text-xs">{rule.destination_type || '—'}</td>
                  <td className="text-xs">{formatPorts(rule)}</td>
                  <td className="text-xs">{rule.description || '—'}</td>
                  <td>
                    <Badge tone={rule.stateless ? 'warning' : 'neutral'}>
                      {rule.stateless ? '是' : '否'}
                    </Badge>
                  </td>
                  <td className="oci-cell-actions">
                    <div className="flex justify-end">
                      <button
                        type="button"
                        className="oci-btn-icon"
                        title="删除规则"
                        aria-label="删除规则"
                        disabled={disabled}
                        onClick={() => onDelete(index)}
                      >
                        <Icon name="trash" size={15} />
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
  )
}

export default function Network() {
  const { account, region } = useAccount()
  const effectiveRegion = region || undefined

  const [tab, setTab] = useState<Tab>('vnics')
  const [vnics, setVnics] = useState<Vnic[]>([])
  const [ips, setIps] = useState<PublicIp[]>([])
  const [lists, setLists] = useState<SecurityList[]>([])
  const [rotation, setRotation] = useState<IpRotationSettings | null>(null)
  const [loading, setLoading] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')
  const [busy, setBusy] = useState('')

  // VNIC 改名
  const [vnicEdit, setVnicEdit] = useState<Vnic | null>(null)
  const [vnicForm, setVnicForm] = useState(EMPTY_VNIC_FORM)

  // 换 IP：二次确认 → OTP → 结果
  const [rotateConfirm, setRotateConfirm] = useState<PublicIp | null>(null)
  const [rotateOtp, setRotateOtp] = useState<PublicIp | null>(null)
  const [rotateResult, setRotateResult] = useState<RotateIpResult | null>(null)

  // 安全列表规则草稿
  const [selectedListId, setSelectedListId] = useState('')
  const [draftIngress, setDraftIngress] = useState<SecurityRule[]>([])
  const [draftEgress, setDraftEgress] = useState<SecurityRule[]>([])
  const [dirty, setDirty] = useState(false)
  const [savingRules, setSavingRules] = useState(false)
  const [ruleModal, setRuleModal] = useState<'ingress' | 'egress' | null>(null)
  const [ruleForm, setRuleForm] = useState<RuleForm>(EMPTY_RULE_FORM)

  usePageAction(
    <button type="button" className="oci-btn-secondary" onClick={() => void load()}>
      <Icon name="refresh" size={15} />
      刷新
    </button>,
  )

  const load = useCallback(async () => {
    if (!account) return
    setLoading(true)
    setError('')
    try {
      const [v, i, s, r] = await Promise.all([
        api.network.vnics(account.id, { region: effectiveRegion }),
        api.network.publicIps(account.id),
        api.network.securityLists(account.id, { region: effectiveRegion }),
        api.ipRotation.get(),
      ])
      setVnics(v)
      setIps(i)
      setLists(s)
      setRotation(r)
      setLoaded(true)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }, [account, effectiveRegion])

  useEffect(() => {
    void load()
  }, [load])

  // 选中安全列表变化时同步本地规则草稿
  useEffect(() => {
    if (lists.length === 0) {
      setSelectedListId('')
      setDraftIngress([])
      setDraftEgress([])
      setDirty(false)
      return
    }
    const id = lists.some((l) => l.id === selectedListId) ? selectedListId : lists[0].id
    if (id !== selectedListId) setSelectedListId(id)
    const list = lists.find((l) => l.id === id)
    if (list) {
      setDraftIngress(list.ingress_rules.map((r) => ({ ...r })))
      setDraftEgress(list.egress_rules.map((r) => ({ ...r })))
      setDirty(false)
    }
  }, [lists, selectedListId])

  const selectedList = lists.find((l) => l.id === selectedListId) ?? null

  async function saveVnic() {
    if (!account || !vnicEdit) return
    setBusy(vnicEdit.id)
    setError('')
    setSuccess('')
    try {
      await api.network.updateVnic(
        account.id,
        vnicEdit.id,
        { display_name: vnicForm.display_name, hostname_label: vnicForm.hostname_label },
        { region: effectiveRegion },
      )
      setVnicEdit(null)
      setSuccess('VNIC 信息已更新')
      await load()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy('')
    }
  }

  function openVnicEdit(item: Vnic) {
    setVnicForm({
      display_name: item.display_name,
      hostname_label: item.hostname_label,
    })
    setVnicEdit(item)
  }

  function proceedRotate() {
    const ip = rotateConfirm
    setRotateConfirm(null)
    setRotateOtp(ip)
  }

  function addRule() {
    if (!ruleModal) return
    const rule: SecurityRule = {
      protocol: ruleForm.protocol,
      source: ruleForm.source,
      source_type: ruleForm.source_type,
      destination: ruleForm.destination,
      destination_type: ruleForm.destination_type,
      description: ruleForm.description,
      stateless: ruleForm.stateless,
    }
    const toNum = (value: string): number | undefined => {
      if (value === '') return undefined
      const n = Number(value)
      return Number.isFinite(n) ? n : undefined
    }
    if (ruleForm.protocol === 'tcp' || ruleForm.protocol === 'all') {
      const min = toNum(ruleForm.tcp_port_min)
      const max = toNum(ruleForm.tcp_port_max)
      if (min !== undefined) rule.tcp_port_min = min
      if (max !== undefined) rule.tcp_port_max = max
    }
    if (ruleForm.protocol === 'udp' || ruleForm.protocol === 'all') {
      const min = toNum(ruleForm.udp_port_min)
      const max = toNum(ruleForm.udp_port_max)
      if (min !== undefined) rule.udp_port_min = min
      if (max !== undefined) rule.udp_port_max = max
    }
    if (ruleModal === 'ingress') setDraftIngress((prev) => [...prev, rule])
    else setDraftEgress((prev) => [...prev, rule])
    setDirty(true)
    setRuleModal(null)
    setRuleForm(EMPTY_RULE_FORM)
  }

  async function saveRules() {
    if (!account || !selectedListId) return
    setSavingRules(true)
    setError('')
    setSuccess('')
    try {
      await api.network.updateSecurityList(
        account.id,
        selectedListId,
        { ingress_rules: draftIngress, egress_rules: draftEgress },
        { region: effectiveRegion },
      )
      setDirty(false)
      setSuccess('安全列表规则已保存')
      await load()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSavingRules(false)
    }
  }

  function resetRules() {
    if (!selectedList) return
    setDraftIngress(selectedList.ingress_rules.map((r) => ({ ...r })))
    setDraftEgress(selectedList.egress_rules.map((r) => ({ ...r })))
    setDirty(false)
  }

  if (!account) return null

  const TABS: { id: Tab; label: string }[] = [
    { id: 'vnics', label: 'VNIC' },
    { id: 'ips', label: '公网 IP' },
    { id: 'security', label: '安全列表' },
  ]

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}
      {success && <SuccessBanner message={success} onClose={() => setSuccess('')} />}
      {rotateResult && (
        <SuccessBanner
          message={`公网 IP 已更换：${rotateResult.new_ip.ip_address}。${rotateResult.message}`}
          onClose={() => setRotateResult(null)}
        />
      )}

      <div className="oci-tabs">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            className={`oci-tab ${tab === t.id ? 'active' : ''}`}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {loading && !loaded ? (
        <Loading text="正在加载网络资源…" />
      ) : (
        <>
          {/* ---------------- VNIC ---------------- */}
          {tab === 'vnics' && (
            <Card
              title="虚拟网卡（VNIC）"
              extra={<span className="text-[13px] text-oracle-muted">{vnics.length} 个 VNIC</span>}
              bodyClassName="oci-card-body--flush"
            >
              {vnics.length === 0 ? (
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
                        <th>MAC 地址</th>
                        <th>状态</th>
                        <th>所属实例</th>
                        <th className="oci-cell-actions">操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {vnics.map((item) => (
                        <tr key={item.id}>
                          <td className="font-medium">{item.display_name || '(未命名)'}</td>
                          <td className="font-mono text-xs">{item.hostname_label || '—'}</td>
                          <td className="font-mono text-xs">{item.private_ip || '—'}</td>
                          <td className="font-mono text-xs">{item.public_ip || '—'}</td>
                          <td className="font-mono text-xs">{item.mac_address || '—'}</td>
                          <td><Badge>{item.state}</Badge></td>
                          <td className="font-mono text-xs">{truncate(item.instance_id, 16, 0)}</td>
                          <td className="oci-cell-actions">
                            <div className="flex justify-end">
                              <button
                                type="button"
                                className="oci-btn-secondary"
                                disabled={busy === item.id}
                                onClick={() => openVnicEdit(item)}
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
          )}

          {/* ---------------- 公网 IP ---------------- */}
          {tab === 'ips' && (
            <div className="space-y-4">
              <RotationInfo rotation={rotation} />
              <Card
                title="公网 IP"
                extra={<span className="text-[13px] text-oracle-muted">{ips.length} 个地址</span>}
                bodyClassName="oci-card-body--flush"
              >
                <p className="px-6 pt-4 text-xs text-oracle-muted">
                  更换公网 IP 受冷却时间与每日次数限制，并需要二次验证。
                </p>
                {ips.length === 0 ? (
                  <EmptyState />
                ) : (
                  <div className="oci-table-wrap">
                    <table className="oci-table">
                      <thead>
                        <tr>
                          <th>IP 地址</th>
                          <th>名称</th>
                          <th>生命周期</th>
                          <th>私有 IP ID</th>
                          <th>状态</th>
                          <th className="oci-cell-actions">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {ips.map((item) => (
                          <tr key={item.id}>
                            <td className="font-mono font-medium">{item.ip_address}</td>
                            <td>{item.display_name || '—'}</td>
                            <td className="text-xs"><Badge tone="info">{item.lifetime}</Badge></td>
                            <td className="font-mono text-xs" title={item.private_ip_id}>
                              {truncate(item.private_ip_id, 18, 0)}
                            </td>
                            <td><Badge>{item.state}</Badge></td>
                            <td className="oci-cell-actions">
                              <div className="flex justify-end">
                                <button
                                  type="button"
                                  className="oci-btn-secondary"
                                  disabled={!item.private_ip_id}
                                  onClick={() => setRotateConfirm(item)}
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
            </div>
          )}

          {/* ---------------- 安全列表 ---------------- */}
          {tab === 'security' && (
            <div className="space-y-4">
              <Card title="安全列表规则">
                <div className="flex flex-wrap items-end gap-3">
                  <div className="min-w-[240px] flex-1">
                    <Field label="选择安全列表">
                      <select
                        className="oci-select"
                        value={selectedListId}
                        onChange={(e) => setSelectedListId(e.target.value)}
                      >
                        {lists.map((l) => (
                          <option key={l.id} value={l.id}>
                            {l.display_name}
                          </option>
                        ))}
                      </select>
                    </Field>
                  </div>
                  <button
                    type="button"
                    className="oci-btn-secondary"
                    onClick={resetRules}
                    disabled={!dirty || savingRules}
                  >
                    放弃更改
                  </button>
                  <button
                    type="button"
                    className="oci-btn-primary"
                    onClick={() => void saveRules()}
                    disabled={!dirty || savingRules || !selectedList}
                  >
                    {savingRules ? '保存中…' : '保存规则'}
                  </button>
                </div>
                {selectedList && (
                  <p className="mt-3 text-xs text-oracle-muted">
                    VCN：{truncate(selectedList.vcn_id, 24, 0)} · 入站 {draftIngress.length} 条 · 出站 {draftEgress.length} 条
                  </p>
                )}
              </Card>

              {lists.length === 0 ? (
                <Card bodyClassName="oci-card-body--flush"><EmptyState /></Card>
              ) : (
                <div className="space-y-4">
                  <RuleTable
                    title="入站规则（Ingress）"
                    rules={draftIngress}
                    onAdd={() => setRuleModal('ingress')}
                    onDelete={(i) => {
                      setDraftIngress((prev) => prev.filter((_, idx) => idx !== i))
                      setDirty(true)
                    }}
                    disabled={savingRules}
                  />
                  <RuleTable
                    title="出站规则（Egress）"
                    rules={draftEgress}
                    onAdd={() => setRuleModal('egress')}
                    onDelete={(i) => {
                      setDraftEgress((prev) => prev.filter((_, idx) => idx !== i))
                      setDirty(true)
                    }}
                    disabled={savingRules}
                  />
                </div>
              )}
            </div>
          )}
        </>
      )}

      {/* VNIC 改名 */}
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
                disabled={busy === vnicEdit.id}
                onClick={() => void saveVnic()}
              >
                {busy === vnicEdit.id ? '保存中…' : '保存'}
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
            <Field label="主机名标签" hint="将作为实例内网 DNS 名称的一部分">
              <input
                className="oci-input"
                value={vnicForm.hostname_label}
                onChange={(e) => setVnicForm((f) => ({ ...f, hostname_label: e.target.value }))}
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
              <br />· Cloudflare DNS 记录不会自动更新，更换后请自行同步所有解析记录。
              <br />· 此操作不可逆，旧 IP 将立即释放且无法找回。
            </Notice>
            <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-xs text-oracle-muted">当前公网 IP</dt>
                <dd className="mt-1 font-mono">{rotateConfirm.ip_address}</dd>
              </div>
              <div>
                <dt className="text-xs text-oracle-muted">生命周期</dt>
                <dd className="mt-1">{rotateConfirm.lifetime}</dd>
              </div>
              <div className="sm:col-span-2">
                <dt className="text-xs text-oracle-muted">关联的私有 IP ID</dt>
                <dd className="mt-1 break-all font-mono text-xs">{rotateConfirm.private_ip_id || '—'}</dd>
              </div>
            </dl>
            <RotationInfo rotation={rotation} />
          </div>
        </Modal>
      )}

      {/* 换 IP 第二级：OTP 验证 */}
      {rotateOtp && (
        <OtpConfirm
          title="验证并更换公网 IP"
          operation="rotate_public_ip"
          summary={`更换公网 IP（当前 ${rotateOtp.ip_address}）`}
          confirmLabel="确认更换"
          onClose={() => setRotateOtp(null)}
          onConfirm={async (otp) => {
            if (!account) return
            const res = await api.network.rotateIp(account.id, {
              private_ip_id: rotateOtp.private_ip_id,
              otp,
            })
            setRotateResult(res)
            await load()
          }}
        >
          <Notice tone="warning">
            Cloudflare DNS 记录不会自动更新；此操作不可逆，旧 IP 释放后无法找回。
          </Notice>
        </OtpConfirm>
      )}

      {/* 添加安全规则 */}
      {ruleModal && (
        <Modal
          title={`添加${ruleModal === 'ingress' ? '入站' : '出站'}规则`}
          onClose={() => {
            setRuleModal(null)
            setRuleForm(EMPTY_RULE_FORM)
          }}
          footer={
            <>
              <button
                type="button"
                className="oci-btn-secondary"
                onClick={() => {
                  setRuleModal(null)
                  setRuleForm(EMPTY_RULE_FORM)
                }}
              >
                取消
              </button>
              <button type="button" className="oci-btn-primary" onClick={addRule}>
                添加
              </button>
            </>
          }
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label="协议">
              <select
                className="oci-select"
                value={ruleForm.protocol}
                onChange={(e) => setRuleForm((f) => ({ ...f, protocol: e.target.value }))}
              >
                <option value="all">全部</option>
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
                <option value="icmp">ICMP</option>
              </select>
            </Field>
            <Field label="源类型">
              <select
                className="oci-select"
                value={ruleForm.source_type}
                onChange={(e) => setRuleForm((f) => ({ ...f, source_type: e.target.value }))}
              >
                <option value="CIDR_BLOCK">CIDR_BLOCK</option>
                <option value="SERVICE_CIDR_BLOCK">SERVICE_CIDR_BLOCK</option>
              </select>
            </Field>
            <Field label="源 CIDR">
              <input
                className="oci-input font-mono text-xs"
                value={ruleForm.source}
                onChange={(e) => setRuleForm((f) => ({ ...f, source: e.target.value }))}
              />
            </Field>
            <Field label="目的类型">
              <select
                className="oci-select"
                value={ruleForm.destination_type}
                onChange={(e) => setRuleForm((f) => ({ ...f, destination_type: e.target.value }))}
              >
                <option value="CIDR_BLOCK">CIDR_BLOCK</option>
                <option value="SERVICE_CIDR_BLOCK">SERVICE_CIDR_BLOCK</option>
              </select>
            </Field>
            <Field label="目的 CIDR">
              <input
                className="oci-input font-mono text-xs"
                value={ruleForm.destination}
                onChange={(e) => setRuleForm((f) => ({ ...f, destination: e.target.value }))}
              />
            </Field>
            {(ruleForm.protocol === 'tcp' || ruleForm.protocol === 'all') && (
              <>
                <Field label="TCP 起始端口">
                  <input
                    type="number"
                    className="oci-input"
                    value={ruleForm.tcp_port_min}
                    onChange={(e) => setRuleForm((f) => ({ ...f, tcp_port_min: e.target.value }))}
                  />
                </Field>
                <Field label="TCP 结束端口">
                  <input
                    type="number"
                    className="oci-input"
                    value={ruleForm.tcp_port_max}
                    onChange={(e) => setRuleForm((f) => ({ ...f, tcp_port_max: e.target.value }))}
                  />
                </Field>
              </>
            )}
            {(ruleForm.protocol === 'udp' || ruleForm.protocol === 'all') && (
              <>
                <Field label="UDP 起始端口">
                  <input
                    type="number"
                    className="oci-input"
                    value={ruleForm.udp_port_min}
                    onChange={(e) => setRuleForm((f) => ({ ...f, udp_port_min: e.target.value }))}
                  />
                </Field>
                <Field label="UDP 结束端口">
                  <input
                    type="number"
                    className="oci-input"
                    value={ruleForm.udp_port_max}
                    onChange={(e) => setRuleForm((f) => ({ ...f, udp_port_max: e.target.value }))}
                  />
                </Field>
              </>
            )}
            <div className="sm:col-span-2">
              <Field label="描述">
                <input
                  className="oci-input"
                  value={ruleForm.description}
                  onChange={(e) => setRuleForm((f) => ({ ...f, description: e.target.value }))}
                />
              </Field>
            </div>
            <div className="sm:col-span-2">
              <Switch
                checked={ruleForm.stateless}
                onChange={(c) => setRuleForm((f) => ({ ...f, stateless: c }))}
                label="无状态规则（stateless）"
              />
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}
