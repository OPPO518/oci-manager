import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { OtpConfirm } from '../components/OtpConfirm'
import { usePageAction } from '../components/Layout'
import {
  ActionsMenu,
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
  IAMApiKey,
  IAMGroup,
  IAMMembership,
  IAMOverview,
  IAMUser,
  IamCredentialInput,
  IamSettings,
  OtpPayload,
} from '../lib/types'

type Tab = 'overview' | 'settings'

const EMPTY_CRED: IamCredentialInput = {
  tenancy_ocid: '',
  user_ocid: '',
  fingerprint: '',
  private_key_pem: '',
  passphrase: '',
  region: '',
}

const EMPTY_USER_CREATE = { name: '', description: '', email: '' }
const EMPTY_USER_EDIT = { description: '', email: '', disabled: false }
const EMPTY_API_KEY = { user_id: '', public_key_pem: '' }

interface OtpAction {
  title: string
  operation: string
  summary: string
  confirmLabel?: string
  run: (otp: OtpPayload) => Promise<void>
  children?: ReactNode
}

/* ------------------------------------------------------------------
   一次性控制台密码：默认遮盖、可复制、关闭/卸载时清空 state
   ------------------------------------------------------------------ */
function OneTimePasswordModal({
  password,
  hint,
  onClose,
}: {
  password: string
  hint: string
  onClose: () => void
}) {
  const [revealed, setRevealed] = useState(false)
  const [copyState, setCopyState] = useState<'idle' | 'ok' | 'fail'>('idle')

  useEffect(
    () => () => {
      // 卸载时显式清空显示状态
      setRevealed(false)
      setCopyState('idle')
    },
    [],
  )

  async function copyPassword() {
    try {
      if (!navigator.clipboard) throw new Error('clipboard unavailable')
      await navigator.clipboard.writeText(password)
      setCopyState('ok')
    } catch {
      setCopyState('fail')
    }
  }

  return (
    <Modal
      title="一次性控制台密码"
      onClose={onClose}
      footer={
        <>
          <button
            type="button"
            className="oci-btn-secondary"
            onClick={() => setRevealed((v) => !v)}
          >
            <Icon name={revealed ? 'eye-off' : 'eye'} size={15} />
            {revealed ? '隐藏' : '显示'}
          </button>
          <button type="button" className="oci-btn-primary" onClick={() => void copyPassword()}>
            <Icon name="copy" size={15} />
            复制
          </button>
          <button type="button" className="oci-btn-secondary" onClick={onClose}>
            关闭
          </button>
        </>
      }
    >
      <div className="space-y-4">
        <Notice tone="warning">
          此密码仅显示一次，关闭后将无法再次查看，请立即复制保存。
        </Notice>
        <div>
          <span className="oci-label">一次性密码</span>
          <div className="break-all rounded border border-oracle-border bg-oracle-surface2 px-4 py-3 font-mono text-base">
            {revealed ? password : '••••••••••••••••'}
          </div>
        </div>
        {copyState === 'ok' && <SuccessBanner message="已复制到剪贴板" />}
        {copyState === 'fail' && (
          <Notice tone="warning">
            自动复制失败，请点击「显示」后手动选中密码文本复制。
          </Notice>
        )}
        {hint && <p className="text-xs leading-relaxed text-oracle-muted">{hint}</p>}
      </div>
    </Modal>
  )
}

/* ------------------------------------------------------------------
   临时凭证导入：成功 / 关闭时显式清空私钥与口令 state
   ------------------------------------------------------------------ */
function CredentialModal({
  defaultRegion,
  onClose,
  onSaved,
}: {
  defaultRegion: string
  onClose: () => void
  onSaved: () => void
}) {
  const [form, setForm] = useState<IamCredentialInput>({ ...EMPTY_CRED, region: defaultRegion })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  function clearSensitive() {
    // 显式清空含 private_key_pem / passphrase 的 state
    setForm({ ...EMPTY_CRED, region: defaultRegion })
    setError('')
  }

  function handleClose() {
    clearSensitive()
    onClose()
  }

  async function submit() {
    setSaving(true)
    setError('')
    try {
      await api.iam.credential(form)
      clearSensitive()
      onSaved()
      onClose()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title="导入临时 IAM 凭证"
      onClose={handleClose}
      width="max-w-2xl"
      footer={
        <>
          <button type="button" className="oci-btn-secondary" onClick={handleClose}>
            取消
          </button>
          <button
            type="button"
            className="oci-btn-primary"
            disabled={saving}
            onClick={() => void submit()}
          >
            {saving ? '导入中…' : '导入'}
          </button>
        </>
      }
    >
      <div className="space-y-3">
        <Notice tone="warning">
          凭证仅驻留内存 10 分钟、不落盘、不回显；到期后自动失效，需重新导入。
        </Notice>
        {error && <ErrorBanner message={error} onClose={() => setError('')} />}
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <Field label="租户 OCID">
              <input
                className="oci-input font-mono text-xs"
                value={form.tenancy_ocid}
                onChange={(e) => setForm((f) => ({ ...f, tenancy_ocid: e.target.value }))}
              />
            </Field>
          </div>
          <div className="sm:col-span-2">
            <Field label="用户 OCID">
              <input
                className="oci-input font-mono text-xs"
                value={form.user_ocid}
                onChange={(e) => setForm((f) => ({ ...f, user_ocid: e.target.value }))}
              />
            </Field>
          </div>
          <div className="sm:col-span-2">
            <Field label="公钥指纹">
              <input
                className="oci-input font-mono text-xs"
                value={form.fingerprint}
                onChange={(e) => setForm((f) => ({ ...f, fingerprint: e.target.value }))}
              />
            </Field>
          </div>
          <Field label="区域">
            <input
              className="oci-input"
              value={form.region}
              onChange={(e) => setForm((f) => ({ ...f, region: e.target.value }))}
            />
          </Field>
          <Field label="私钥口令（没有则留空）">
            <input
              type="password"
              className="oci-input"
              autoComplete="off"
              value={form.passphrase}
              onChange={(e) => setForm((f) => ({ ...f, passphrase: e.target.value }))}
            />
          </Field>
          <div className="sm:col-span-2">
            <Field label="API 私钥（PEM 全文）">
              <textarea
                className="oci-input h-32 font-mono text-xs"
                placeholder="-----BEGIN PRIVATE KEY-----"
                autoComplete="off"
                value={form.private_key_pem}
                onChange={(e) => setForm((f) => ({ ...f, private_key_pem: e.target.value }))}
              />
            </Field>
          </div>
        </div>
      </div>
    </Modal>
  )
}

export default function Identity() {
  const { account } = useAccount()

  const [tab, setTab] = useState<Tab>('overview')
  const [settings, setSettings] = useState<IamSettings | null>(null)
  const [overview, setOverview] = useState<IAMOverview | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  // IAM 设置区的本地表单
  const [moduleMode, setModuleMode] = useState('classic')
  const [homeRegion, setHomeRegion] = useState('')
  const [credOpen, setCredOpen] = useState(false)
  /** 设置区写操作的 loading 标识：'' | 'enabled' | 'module' | 'credential' */
  const [settingsBusy, setSettingsBusy] = useState('')

  // 统一的 OTP 写操作
  const [otpAction, setOtpAction] = useState<OtpAction | null>(null)

  // 一次性密码
  const [passwordReveal, setPasswordReveal] = useState<{ password: string; hint: string } | null>(null)

  // 用户表单
  const [userCreateOpen, setUserCreateOpen] = useState(false)
  const [userCreateForm, setUserCreateForm] = useState(EMPTY_USER_CREATE)
  const [userEdit, setUserEdit] = useState<IAMUser | null>(null)
  const [userEditForm, setUserEditForm] = useState(EMPTY_USER_EDIT)

  // API Key 表单
  const [apiKeyOpen, setApiKeyOpen] = useState(false)
  const [apiKeyForm, setApiKeyForm] = useState(EMPTY_API_KEY)

  // 组成员表单
  const [addMemberGroup, setAddMemberGroup] = useState<IAMGroup | null>(null)
  const [addMemberUserId, setAddMemberUserId] = useState('')

  usePageAction(
    <button type="button" className="oci-btn-secondary" onClick={() => void refresh(false)}>
      <Icon name="refresh" size={15} />
      刷新
    </button>,
  )

  const refresh = useCallback(
    async (showLoading = true) => {
      if (showLoading) setLoading(true)
      setError('')
      try {
        const s = await api.iam.settings()
        setSettings(s)
        setModuleMode(s.mode)
        setHomeRegion(s.home_region)
        if (account) {
          try {
            setOverview(await api.iam.overview(account.id))
          } catch (err) {
            setOverview(null)
            setError((err as Error).message)
          }
        } else {
          setOverview(null)
        }
      } catch (err) {
        setError((err as Error).message)
      } finally {
        setLoading(false)
      }
    },
    [account],
  )

  useEffect(() => {
    void refresh(true)
  }, [refresh])

  const protectedIds = overview?.protected_user_ids ?? []
  const domainsMode = settings?.mode === 'domains' || overview?.mode === 'domains'
  // 模块开关关闭、无概览数据、只读或 Identity Domains 模式下，一律禁用写操作
  const writeDisabled =
    !account || !settings?.enabled || !overview || overview.read_only || domainsMode

  const userName = (id: string) =>
    overview?.users.find((u) => u.id === id)?.name ?? truncate(id, 16, 0)
  const groupName = (id: string) =>
    overview?.groups.find((g) => g.id === id)?.name ?? truncate(id, 16, 0)
  const groupProtected = (id: string) =>
    overview?.groups.find((g) => g.id === id)?.protected ?? false

  /* ------------------------- 设置区写操作 ------------------------- */
  async function saveEnabled(enabled: boolean) {
    setSettingsBusy('enabled')
    setError('')
    setSuccess('')
    try {
      const s = await api.iam.saveSettings({ enabled })
      setSettings(s)
      setSuccess(enabled ? 'IAM 模块已启用' : 'IAM 模块已关闭')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSettingsBusy('')
    }
  }

  async function saveModuleSettings() {
    setSettingsBusy('module')
    setError('')
    setSuccess('')
    try {
      const s = await api.iam.saveSettings({
        enabled: settings?.enabled ?? false,
        mode: moduleMode,
        home_region: homeRegion,
      })
      setSettings(s)
      setSuccess('IAM 模块配置已保存')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSettingsBusy('')
    }
  }

  async function clearCredential() {
    setSettingsBusy('credential')
    setError('')
    setSuccess('')
    try {
      await api.iam.deleteCredential()
      setSuccess('临时凭证已清除')
      await refresh(false)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSettingsBusy('')
    }
  }

  /* ------------------------- OTP 写操作 ------------------------- */
  function submitUserCreate() {
    const form = { ...userCreateForm }
    setUserCreateOpen(false)
    setOtpAction({
      title: '创建 IAM 用户',
      operation: 'iam_create_user',
      summary: `创建用户 ${form.name}`,
      confirmLabel: '确认创建',
      run: async (otp) => {
        if (!account) return
        await api.iam.createUser(account.id, { ...form, otp })
        setUserCreateForm(EMPTY_USER_CREATE)
        setSuccess(`用户「${form.name}」已创建`)
        await refresh(false)
      },
    })
  }

  function openUserEdit(u: IAMUser) {
    setUserEditForm({
      description: u.description,
      email: u.email,
      disabled: u.state.toUpperCase() !== 'ACTIVE',
    })
    setUserEdit(u)
  }

  function submitUserEdit() {
    const target = userEdit
    const form = { ...userEditForm }
    setUserEdit(null)
    if (!target) return
    setOtpAction({
      title: `编辑用户：${target.name}`,
      operation: 'iam_update_user',
      summary: `修改用户 ${target.name} 的资料 / 启停状态`,
      run: async (otp) => {
        if (!account) return
        await api.iam.updateUser(account.id, target.id, { ...form, otp })
        setSuccess(`用户「${target.name}」已更新`)
        await refresh(false)
      },
    })
  }

  function requestDeleteUser(u: IAMUser) {
    setOtpAction({
      title: '删除 IAM 用户',
      operation: 'iam_delete_user',
      summary: `删除用户 ${u.name}`,
      confirmLabel: '确认删除',
      children: <Notice tone="danger">删除后该用户及其成员关系不可恢复。</Notice>,
      run: async (otp) => {
        if (!account) return
        await api.iam.deleteUser(account.id, u.id, otp)
        setSuccess(`用户「${u.name}」已删除`)
        await refresh(false)
      },
    })
  }

  function requestPassword(u: IAMUser) {
    setOtpAction({
      title: '重置控制台密码',
      operation: 'iam_reset_password',
      summary: `重置用户 ${u.name} 的控制台密码`,
      confirmLabel: '生成一次性密码',
      run: async (otp) => {
        if (!account) return
        const res = await api.iam.resetPassword(account.id, u.id, otp)
        setPasswordReveal({ password: res.password, hint: res.hint })
        await refresh(false)
      },
    })
  }

  function openApiKey(userId = '') {
    setApiKeyForm({ user_id: userId, public_key_pem: '' })
    setApiKeyOpen(true)
  }

  function submitApiKey() {
    const form = { ...apiKeyForm }
    setApiKeyOpen(false)
    setOtpAction({
      title: '上传 API Key',
      operation: 'iam_create_api_key',
      summary: `为用户 ${userName(form.user_id)} 上传 API Key`,
      confirmLabel: '确认上传',
      run: async (otp) => {
        if (!account) return
        await api.iam.createApiKey(account.id, form.user_id, {
          public_key_pem: form.public_key_pem,
          otp,
        })
        setApiKeyForm(EMPTY_API_KEY)
        setSuccess('API Key 已上传')
        await refresh(false)
      },
    })
  }

  function requestDeleteApiKey(key: IAMApiKey) {
    setOtpAction({
      title: '删除 API Key',
      operation: 'iam_delete_api_key',
      summary: `删除指纹 ${key.fingerprint} 的 API Key`,
      confirmLabel: '确认删除',
      run: async (otp) => {
        if (!account) return
        await api.iam.deleteApiKey(account.id, key.user_id, key.fingerprint, otp)
        setSuccess('API Key 已删除')
        await refresh(false)
      },
    })
  }

  function openAddMember(g: IAMGroup) {
    setAddMemberUserId('')
    setAddMemberGroup(g)
  }

  function submitAddMember() {
    const group = addMemberGroup
    const userId = addMemberUserId
    setAddMemberGroup(null)
    setAddMemberUserId('')
    if (!group || !userId) return
    setOtpAction({
      title: '加入组',
      operation: 'iam_add_member',
      summary: `将用户 ${userName(userId)} 加入组 ${group.name}`,
      confirmLabel: '确认加入',
      run: async (otp) => {
        if (!account) return
        await api.iam.addMember(account.id, group.id, { user_id: userId, otp })
        setSuccess('成员已加入')
        await refresh(false)
      },
    })
  }

  function requestRemoveMember(m: IAMMembership) {
    setOtpAction({
      title: '移除组成员',
      operation: 'iam_remove_member',
      summary: `将用户 ${userName(m.user_id)} 移出组 ${groupName(m.group_id)}`,
      confirmLabel: '确认移除',
      run: async (otp) => {
        if (!account) return
        await api.iam.removeMember(account.id, m.group_id, m.user_id, otp)
        setSuccess('成员已移除')
        await refresh(false)
      },
    })
  }

  const TABS: { id: Tab; label: string }[] = [
    { id: 'overview', label: 'IAM 资源' },
    { id: 'settings', label: 'IAM 设置' },
  ]

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}
      {success && <SuccessBanner message={success} onClose={() => setSuccess('')} />}

      {/* 我的概要信息：官网详情页标题 + 双栏键值明细 */}
      <div className="oci-detail-head">
        <div>
          <div className="oci-detail-title">{account?.name ?? '未选择账号'}</div>
          <div className="oci-detail-sub">我的概要信息 · 身份与安全</div>
        </div>
        <Badge tone={settings?.enabled ? 'success' : 'neutral'}>
          {settings?.enabled ? '模块已启用' : '模块未启用'}
        </Badge>
      </div>
      <Card title="详细信息" bodyClassName="oci-card-body--compact">
        <div className="grid grid-cols-1 gap-x-10 sm:grid-cols-2">
          {[
            {
              label: '模块状态',
              value: (
                <Badge tone={settings?.enabled ? 'success' : 'neutral'}>
                  {settings?.enabled ? '已启用' : '未启用'}
                </Badge>
              ),
            },
            {
              label: '临时凭证',
              value: (
                <Badge tone={settings?.has_credential ? 'success' : 'warning'}>
                  {settings?.has_credential ? '已导入' : '未导入'}
                </Badge>
              ),
            },
            { label: '凭证到期时间', value: formatDateTime(settings?.credential_expires_at) },
            { label: '主区域（Home Region）', value: settings?.home_region || '—' },
            {
              label: 'Telegram 验证码',
              value: (
                <Badge tone={settings?.telegram_ready ? 'success' : 'warning'}>
                  {settings?.telegram_ready ? '已就绪' : '未就绪'}
                </Badge>
              ),
            },
            {
              label: '模式 / 数据',
              value: `${settings?.mode === 'domains' ? 'Identity Domains' : '经典 IAM'} · ${settings?.demo ? '演示' : '真实'}`,
            },
          ].map((row) => (
            <div className="oci-kv-row" key={row.label}>
              <div className="oci-kv-label">{row.label}</div>
              <div className="oci-kv-value">{row.value}</div>
            </div>
          ))}
        </div>
      </Card>

      {domainsMode && (
        <Notice tone="warning">
          当前为 <strong>Identity Domains（domains）</strong> 模式，本面板仅支持经典 IAM 模式的管理，
          相关写操作不可用。请在本页「IAM 设置」页签中切换回经典 IAM 模式。
        </Notice>
      )}

      <div className="oci-page-tabs">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            className={`oci-page-tab ${tab === t.id ? 'active' : ''}`}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {loading ? (
        <Loading text="正在加载 IAM 数据…" />
      ) : (
        <>
          {/* ------------------------- IAM 概览 ------------------------- */}
          {tab === 'overview' && (
            <div className="space-y-4">
              {overview?.read_only && (
                <Notice tone="warning">
                  当前为只读模式，所有写操作已禁用。{overview.hint}
                </Notice>
              )}
              {!account && (
                <Notice tone="info">
                  尚未选择 OCI 账号。可先在「账号」页添加账号，或切换到本页的「IAM 设置」页签完成模块配置。
                </Notice>
              )}
              {!overview && account && (
                <Notice tone="info">IAM 概览暂不可用，请确认模块已启用且已导入临时凭证。</Notice>
              )}

              {overview && (
                <>
                  <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
                    <Stat label="用户数" value={overview.users.length} accent="brand" />
                    <Stat label="组数" value={overview.groups.length} accent="pine" />
                    <Stat label="成员关系" value={overview.memberships.length} />
                    <Stat label="API Key" value={overview.api_keys.length} accent="yellow" />
                  </div>

                  {/* 用户表 */}
                  <Card
                    title="用户"
                    extra={
                      <button
                        type="button"
                        className="oci-btn-primary"
                        disabled={writeDisabled}
                        onClick={() => {
                          setUserCreateForm(EMPTY_USER_CREATE)
                          setUserCreateOpen(true)
                        }}
                      >
                        <Icon name="plus" size={15} />
                        创建用户
                      </button>
                    }
                    bodyClassName="oci-card-body--flush"
                  >
                    {overview.users.length === 0 ? (
                      <EmptyState />
                    ) : (
                      <div className="oci-table-wrap">
                        <table className="oci-table">
                          <thead>
                            <tr>
                              <th>名称</th>
                              <th>描述</th>
                              <th>邮箱</th>
                              <th>状态</th>
                              <th>创建时间</th>
                              <th>数据库用户名</th>
                              <th className="oci-cell-actions">操作</th>
                            </tr>
                          </thead>
                          <tbody>
                            {overview.users.map((u) => {
                              const isProtected = protectedIds.includes(u.id)
                              return (
                                <tr key={u.id}>
                                  <td className="font-medium">{u.name}</td>
                                  <td className="text-xs">{u.description || '—'}</td>
                                  <td className="text-xs">{u.email || '—'}</td>
                                  <td>
                                    <Badge tone={u.state.toUpperCase() === 'ACTIVE' ? 'success' : 'neutral'}>
                                      {u.state}
                                    </Badge>
                                  </td>
                                  <td className="text-xs text-oracle-muted">{formatDateTime(u.time_created)}</td>
                                  <td className="font-mono text-xs">{u.db_user_name || '—'}</td>
                                  <td className="oci-cell-actions">
                                    {isProtected ? (
                                      <span className="text-xs text-oracle-muted">
                                        面板自用 API 用户，禁止操作
                                      </span>
                                    ) : (
                                      <div className="flex justify-end">
                                        <ActionsMenu
                                          label="用户操作"
                                          items={[
                                            {
                                              label: '编辑资料 / 启停',
                                              disabled: writeDisabled,
                                              onSelect: () => openUserEdit(u),
                                            },
                                            {
                                              label: '重置控制台密码',
                                              disabled: writeDisabled || !u.can_use_console_password,
                                              onSelect: () => requestPassword(u),
                                            },
                                            {
                                              label: '上传 API Key',
                                              disabled: writeDisabled,
                                              onSelect: () => openApiKey(u.id),
                                            },
                                            {
                                              label: '删除用户',
                                              disabled: writeDisabled,
                                              danger: true,
                                              separatorBefore: true,
                                              onSelect: () => requestDeleteUser(u),
                                            },
                                          ]}
                                        />
                                      </div>
                                    )}
                                  </td>
                                </tr>
                              )
                            })}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </Card>

                  {/* 组表 */}
                  <Card title="组" bodyClassName="oci-card-body--flush">
                    {overview.groups.length === 0 ? (
                      <EmptyState />
                    ) : (
                      <div className="oci-table-wrap">
                        <table className="oci-table">
                          <thead>
                            <tr>
                              <th>名称</th>
                              <th>描述</th>
                              <th>状态</th>
                              <th>保护</th>
                              <th className="oci-cell-actions">操作</th>
                            </tr>
                          </thead>
                          <tbody>
                            {overview.groups.map((g) => (
                              <tr key={g.id}>
                                <td className="font-medium">{g.name}</td>
                                <td className="text-xs">{g.description || '—'}</td>
                                <td><Badge>{g.state}</Badge></td>
                                <td>
                                  {g.protected ? (
                                    <Badge tone="warning">受保护</Badge>
                                  ) : (
                                    <Badge tone="neutral">普通</Badge>
                                  )}
                                </td>
                                <td className="oci-cell-actions">
                                  {g.protected ? (
                                    <span className="text-xs text-oracle-muted">
                                      Administrators 等受保护组禁止成员变更
                                    </span>
                                  ) : (
                                    <div className="flex justify-end">
                                      <button
                                        type="button"
                                        className="oci-btn-secondary"
                                        disabled={writeDisabled}
                                        onClick={() => openAddMember(g)}
                                      >
                                        <Icon name="plus" size={15} />
                                        添加成员
                                      </button>
                                    </div>
                                  )}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </Card>

                  {/* 成员关系 */}
                  <Card title="成员关系" bodyClassName="oci-card-body--flush">
                    {overview.memberships.length === 0 ? (
                      <EmptyState />
                    ) : (
                      <div className="oci-table-wrap">
                        <table className="oci-table">
                          <thead>
                            <tr>
                              <th>用户</th>
                              <th>组</th>
                              <th>关系 ID</th>
                              <th className="oci-cell-actions">操作</th>
                            </tr>
                          </thead>
                          <tbody>
                            {overview.memberships.map((m) => (
                              <tr key={m.id}>
                                <td>{userName(m.user_id)}</td>
                                <td>{groupName(m.group_id)}</td>
                                <td className="font-mono text-xs">{truncate(m.id, 16, 0)}</td>
                                <td className="oci-cell-actions">
                                  {groupProtected(m.group_id) ? (
                                    <span className="text-xs text-oracle-muted">受保护组，禁止变更</span>
                                  ) : (
                                    <div className="flex justify-end">
                                      <button
                                        type="button"
                                        className="oci-btn-danger"
                                        disabled={writeDisabled}
                                        onClick={() => requestRemoveMember(m)}
                                      >
                                        移除
                                      </button>
                                    </div>
                                  )}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </Card>

                  {/* API Key 列表 */}
                  <Card
                    title="API Key"
                    extra={
                      <button
                        type="button"
                        className="oci-btn-secondary"
                        disabled={writeDisabled || overview.users.length === 0}
                        onClick={() => openApiKey()}
                      >
                        <Icon name="key" size={15} />
                        上传 API Key
                      </button>
                    }
                    bodyClassName="oci-card-body--flush"
                  >
                    {overview.api_keys.length === 0 ? (
                      <EmptyState />
                    ) : (
                      <div className="oci-table-wrap">
                        <table className="oci-table">
                          <thead>
                            <tr>
                              <th>所属用户</th>
                              <th>指纹</th>
                              <th>状态</th>
                              <th>创建时间</th>
                              <th className="oci-cell-actions">操作</th>
                            </tr>
                          </thead>
                          <tbody>
                            {overview.api_keys.map((k) => (
                              <tr key={`${k.user_id}-${k.fingerprint}`}>
                                <td>{userName(k.user_id)}</td>
                                <td className="font-mono text-xs">{k.fingerprint || '—'}</td>
                                <td><Badge>{k.state}</Badge></td>
                                <td className="text-xs text-oracle-muted">{formatDateTime(k.time_created)}</td>
                                <td className="oci-cell-actions">
                                  {protectedIds.includes(k.user_id) ? (
                                    <span className="text-xs text-oracle-muted">受保护用户，禁止操作</span>
                                  ) : (
                                    <div className="flex justify-end">
                                      <button
                                        type="button"
                                        className="oci-btn-icon"
                                        title="删除 API Key"
                                        aria-label="删除 API Key"
                                        disabled={writeDisabled}
                                        onClick={() => requestDeleteApiKey(k)}
                                      >
                                        <Icon name="trash" size={15} />
                                      </button>
                                    </div>
                                  )}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </Card>

                  {/* 限额卡片 */}
                  <Card title="限额（Limits）">
                    {overview.limits.length === 0 ? (
                      <EmptyState />
                    ) : (
                      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
                        {overview.limits.map((l) => (
                          <div className="oci-stat" key={`${l.name}-${l.scope}`}>
                            <p className="oci-stat-label">{l.name} · {l.scope}</p>
                            <p className="oci-stat-value">
                              {l.available}
                              <span className="ml-1 text-sm font-normal text-oracle-muted">可用</span>
                            </p>
                            <div className="oci-progress-track mt-3">
                              <div
                                className="oci-progress-fill"
                                style={{
                                  width: `${l.total > 0 ? Math.min(100, (l.used / l.total) * 100) : 0}%`,
                                }}
                              />
                            </div>
                            <p className="oci-hint">已用 {l.used} / 共 {l.total}</p>
                          </div>
                        ))}
                      </div>
                    )}
                  </Card>
                </>
              )}
            </div>
          )}

          {/* ------------------------- IAM 设置 ------------------------- */}
          {tab === 'settings' && (
            <div className="space-y-4">
              <Card title="IAM 模块">
                <div className="max-w-2xl space-y-4">
                  <Switch
                    checked={settings?.enabled ?? false}
                    disabled={settingsBusy !== ''}
                    onChange={(c) => void saveEnabled(c)}
                    label="启用 IAM 管理模块"
                  />
                  <Notice tone="info">
                    本面板仅支持<strong>经典 IAM（classic）</strong>模式；Identity Domains 模式下的写操作不可用。
                    模块开关、模式与主区域都在本页的「IAM 设置」页签；真实账号的写操作还需在「设置 → Telegram 通知」启用验证码。
                  </Notice>
                  <div className="grid grid-cols-1 gap-3 border-t border-oracle-border pt-4 sm:grid-cols-2">
                    <Field label="模式（mode）">
                      <select
                        className="oci-select"
                        value={moduleMode}
                        onChange={(e) => setModuleMode(e.target.value)}
                      >
                        <option value="classic">经典 IAM（classic）</option>
                        <option value="domains">Identity Domains（domains）</option>
                      </select>
                    </Field>
                    <Field label="主区域（Home Region）">
                      <input
                        className="oci-input"
                        value={homeRegion}
                        onChange={(e) => setHomeRegion(e.target.value)}
                      />
                    </Field>
                  </div>
                  <button
                    type="button"
                    className="oci-btn-primary"
                    disabled={settingsBusy !== ''}
                    onClick={() => void saveModuleSettings()}
                  >
                    {settingsBusy === 'module' ? '保存中…' : '保存模块配置'}
                  </button>
                </div>
              </Card>

              <Card title="临时凭证">
                <div className="max-w-2xl space-y-4">
                  <Notice tone="warning">
                    凭证仅驻留内存 10 分钟、不落盘、不回显；接口响应永不回显私钥，到期后需重新导入。
                    演示账号无需导入任何真实凭证即可试用全部功能。
                  </Notice>
                  <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
                    <div>
                      <dt className="text-xs text-oracle-muted">当前状态</dt>
                      <dd className="mt-1">
                        <Badge tone={settings?.has_credential ? 'success' : 'neutral'}>
                          {settings?.has_credential ? '已导入' : '未导入'}
                        </Badge>
                      </dd>
                    </div>
                    <div>
                      <dt className="text-xs text-oracle-muted">到期时间</dt>
                      <dd className="mt-1 text-xs">{formatDateTime(settings?.credential_expires_at)}</dd>
                    </div>
                  </dl>
                  <div className="flex flex-wrap items-center gap-2 border-t border-oracle-border pt-4">
                    <button
                      type="button"
                      className="oci-btn-primary"
                      onClick={() => setCredOpen(true)}
                    >
                      <Icon name="key" size={15} />
                      导入临时凭证
                    </button>
                    <button
                      type="button"
                      className="oci-btn-danger"
                      disabled={!settings?.has_credential || settingsBusy !== ''}
                      onClick={() => void clearCredential()}
                    >
                      {settingsBusy === 'credential' ? '清除中…' : '清除凭证'}
                    </button>
                  </div>
                </div>
              </Card>
            </div>
          )}
        </>
      )}

      {/* 统一 OTP 确认 */}
      {otpAction && (
        <OtpConfirm
          title={otpAction.title}
          operation={otpAction.operation}
          summary={otpAction.summary}
          confirmLabel={otpAction.confirmLabel}
          onClose={() => setOtpAction(null)}
          onConfirm={otpAction.run}
        >
          {otpAction.children}
        </OtpConfirm>
      )}

      {/* 一次性密码：关闭时显式清空 */}
      {passwordReveal && (
        <OneTimePasswordModal
          password={passwordReveal.password}
          hint={passwordReveal.hint}
          onClose={() => setPasswordReveal(null)}
        />
      )}

      {/* 临时凭证导入 */}
      {credOpen && (
        <CredentialModal
          defaultRegion={homeRegion || account?.region || ''}
          onClose={() => setCredOpen(false)}
          onSaved={() => {
            setSuccess('临时凭证已导入')
            void refresh(false)
          }}
        />
      )}

      {/* 创建用户 */}
      {userCreateOpen && (
        <Modal
          title="创建 IAM 用户"
          onClose={() => {
            setUserCreateOpen(false)
            setUserCreateForm(EMPTY_USER_CREATE)
          }}
          footer={
            <>
              <button
                type="button"
                className="oci-btn-secondary"
                onClick={() => {
                  setUserCreateOpen(false)
                  setUserCreateForm(EMPTY_USER_CREATE)
                }}
              >
                取消
              </button>
              <button
                type="button"
                className="oci-btn-primary"
                disabled={!userCreateForm.name.trim()}
                onClick={submitUserCreate}
              >
                下一步
              </button>
            </>
          }
        >
          <div className="space-y-3">
            <p className="text-xs text-oracle-muted">填写资料后，下一步需要输入验证码确认。</p>
            <Field label="用户名">
              <input
                className="oci-input"
                value={userCreateForm.name}
                onChange={(e) => setUserCreateForm((f) => ({ ...f, name: e.target.value }))}
              />
            </Field>
            <Field label="描述">
              <input
                className="oci-input"
                value={userCreateForm.description}
                onChange={(e) => setUserCreateForm((f) => ({ ...f, description: e.target.value }))}
              />
            </Field>
            <Field label="邮箱">
              <input
                type="email"
                className="oci-input"
                value={userCreateForm.email}
                onChange={(e) => setUserCreateForm((f) => ({ ...f, email: e.target.value }))}
              />
            </Field>
          </div>
        </Modal>
      )}

      {/* 编辑用户 */}
      {userEdit && (
        <Modal
          title={`编辑用户：${userEdit.name}`}
          onClose={() => setUserEdit(null)}
          footer={
            <>
              <button type="button" className="oci-btn-secondary" onClick={() => setUserEdit(null)}>
                取消
              </button>
              <button type="button" className="oci-btn-primary" onClick={submitUserEdit}>
                下一步
              </button>
            </>
          }
        >
          <div className="space-y-3">
            <Field label="描述">
              <input
                className="oci-input"
                value={userEditForm.description}
                onChange={(e) => setUserEditForm((f) => ({ ...f, description: e.target.value }))}
              />
            </Field>
            <Field label="邮箱">
              <input
                type="email"
                className="oci-input"
                value={userEditForm.email}
                onChange={(e) => setUserEditForm((f) => ({ ...f, email: e.target.value }))}
              />
            </Field>
            <Switch
              checked={userEditForm.disabled}
              onChange={(c) => setUserEditForm((f) => ({ ...f, disabled: c }))}
              label="禁用该用户"
            />
          </div>
        </Modal>
      )}

      {/* 上传 API Key */}
      {apiKeyOpen && (
        <Modal
          title="上传 API Key"
          onClose={() => {
            setApiKeyOpen(false)
            setApiKeyForm(EMPTY_API_KEY)
          }}
          footer={
            <>
              <button
                type="button"
                className="oci-btn-secondary"
                onClick={() => {
                  setApiKeyOpen(false)
                  setApiKeyForm(EMPTY_API_KEY)
                }}
              >
                取消
              </button>
              <button
                type="button"
                className="oci-btn-primary"
                disabled={!apiKeyForm.user_id || !apiKeyForm.public_key_pem.trim()}
                onClick={submitApiKey}
              >
                下一步
              </button>
            </>
          }
        >
          <div className="space-y-3">
            <Field label="所属用户">
              <select
                className="oci-select"
                value={apiKeyForm.user_id}
                onChange={(e) => setApiKeyForm((f) => ({ ...f, user_id: e.target.value }))}
              >
                <option value="">请选择用户</option>
                {overview?.users
                  .filter((u) => !protectedIds.includes(u.id))
                  .map((u) => (
                    <option key={u.id} value={u.id}>
                      {u.name}
                    </option>
                  ))}
              </select>
            </Field>
            <Field label="公钥（PEM）" hint="仅上传公钥，私钥始终由用户自行保管">
              <textarea
                className="oci-input h-28 font-mono text-xs"
                placeholder="-----BEGIN PUBLIC KEY-----"
                autoComplete="off"
                value={apiKeyForm.public_key_pem}
                onChange={(e) => setApiKeyForm((f) => ({ ...f, public_key_pem: e.target.value }))}
              />
            </Field>
          </div>
        </Modal>
      )}

      {/* 加入组 */}
      {addMemberGroup && (
        <Modal
          title={`加入组：${addMemberGroup.name}`}
          onClose={() => {
            setAddMemberGroup(null)
            setAddMemberUserId('')
          }}
          footer={
            <>
              <button
                type="button"
                className="oci-btn-secondary"
                onClick={() => {
                  setAddMemberGroup(null)
                  setAddMemberUserId('')
                }}
              >
                取消
              </button>
              <button
                type="button"
                className="oci-btn-primary"
                disabled={!addMemberUserId}
                onClick={submitAddMember}
              >
                下一步
              </button>
            </>
          }
        >
          <Field label="选择用户">
            <select
              className="oci-select"
              value={addMemberUserId}
              onChange={(e) => setAddMemberUserId(e.target.value)}
            >
              <option value="">请选择用户</option>
              {overview?.users
                .filter((u) => !protectedIds.includes(u.id))
                .map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name}
                  </option>
                ))}
            </select>
          </Field>
        </Modal>
      )}
    </div>
  )
}
