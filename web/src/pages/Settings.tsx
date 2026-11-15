import { useEffect, useState } from 'react'
import {
  Badge,
  Card,
  EmptyState,
  ErrorBanner,
  Field,
  Icon,
  Loading,
  Notice,
  SuccessBanner,
  Switch,
} from '../components/ui'
import { api } from '../lib/api'
import { useAuth } from '../lib/auth'
import { formatDateTime, truncate } from '../lib/format'
import type { AuditLog, IpRotationSettings, TelegramSettings, TLSSettings } from '../lib/types'

type Tab =
  | 'telegram'
  | 'grabber'
  | 'tls'
  | 'password'
  | 'security'
  | 'demo'
  | 'ipRotation'
  | 'audit'

const EMPTY_TG: TelegramSettings = {
  enabled: false,
  chat_id: '',
  has_token: false,
  login_otp: true,
  event_grab: true,
}

const EMPTY_TLS: TLSSettings = {
  enabled: false,
  domain: '',
  email: '',
  has_token: false,
  cert_issued: false,
  expires_at: '',
  last_error: '',
  staging: false,
  https_port: ':8443',
}

/* 审计日志后端存的是机器枚举，展示层统一翻译成中文 */
const CATEGORY_LABEL: Record<string, string> = {
  settings: '设置',
  iam: '身份与权限',
  compute: '计算',
  storage: '存储',
  network: '网络',
  auth: '登录',
  account: '账号',
  task: '抢购',
}

const OPERATION_LABEL: Record<string, string> = {
  demo_mode: '切换演示模式',
  ip_rotation_policy: '更新换 IP 保护策略',
  iam_settings: '更新 IAM 设置',
  iam_credential_import: '导入临时凭证',
  iam_credential_clear: '清除临时凭证',
  guard_block: '安全护栏拦截',
  iam_create_user: '创建 IAM 用户',
  iam_update_user: '更新 IAM 用户',
  iam_delete_user: '删除 IAM 用户',
  iam_reset_password: '重置控制台密码',
  iam_create_api_key: '上传 API Key',
  iam_delete_api_key: '删除 API Key',
  iam_add_member: '加入用户组',
  iam_remove_member: '移出用户组',
  rename_instance: '重命名实例',
  update_boot_volume: '更新引导卷',
  update_vnic: '更新 VNIC',
  update_security_list: '更新安全列表',
  rotate_public_ip: '更换公网 IP',
  power_task_create: '创建定时任务',
  power_task_update: '更新定时任务',
  power_task_delete: '删除定时任务',
  power_task_toggle: '启用/停用定时任务',
}

function humanOperation(op: string) {
  return OPERATION_LABEL[op] ?? op.replace(/_/g, ' ')
}

const RESULT_META: Record<string, { label: string; tone: 'success' | 'danger' | 'warning' | 'neutral' }> = {
  ok: { label: '成功', tone: 'success' },
  updated: { label: '已更新', tone: 'success' },
  true: { label: '已启用', tone: 'success' },
  false: { label: '已关闭', tone: 'neutral' },
  blocked: { label: '已拦截', tone: 'warning' },
  denied: { label: '已拒绝', tone: 'danger' },
  failed: { label: '失败', tone: 'danger' },
  error: { label: '错误', tone: 'danger' },
}

function humanResult(raw: string) {
  const key = raw.trim().toLowerCase()
  return (
    RESULT_META[key] ?? {
      label: raw || '—',
      tone: /fail|deny|error|block/i.test(key)
        ? ('danger' as const)
        : ('success' as const),
    }
  )
}

export default function Settings() {
  const { username } = useAuth()
  const [tab, setTab] = useState<Tab>('telegram')
  const [access] = useState(() => ({
    host: window.location.hostname,
    secure: window.location.protocol === 'https:',
  }))

  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  // ---- Telegram ----
  const [tg, setTg] = useState<TelegramSettings>(EMPTY_TG)
  const [token, setToken] = useState('')
  const [savingTG, setSavingTG] = useState(false)
  const [testingTG, setTestingTG] = useState(false)

  // ---- 抢购预检 ----
  const [precheck, setPrecheck] = useState(true)
  const [savingPrecheck, setSavingPrecheck] = useState(false)

  // ---- HTTPS ----
  const [tls, setTls] = useState<TLSSettings>(EMPTY_TLS)
  const [tlsDomain, setTlsDomain] = useState('')
  const [tlsEmail, setTlsEmail] = useState('')
  const [cfToken, setCfToken] = useState('')
  const [tlsStaging, setTlsStaging] = useState(false)
  const [savingTLS, setSavingTLS] = useState(false)
  const [disablingTLS, setDisablingTLS] = useState(false)

  // ---- 密码 ----
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [savingPassword, setSavingPassword] = useState(false)

  // ---- 演示模式 ----
  const [demoEnabled, setDemoEnabled] = useState(false)
  const [savingDemo, setSavingDemo] = useState(false)

  // ---- 换 IP 保护策略 ----
  const [rotation, setRotation] = useState<IpRotationSettings | null>(null)
  const [rotationForm, setRotationForm] = useState({ cooldown_sec: 300, daily_limit: 5 })
  const [savingRotation, setSavingRotation] = useState(false)

  // ---- 审计日志 ----
  const [auditLogs, setAuditLogs] = useState<AuditLog[]>([])
  const [auditLoading, setAuditLoading] = useState(false)

  useEffect(() => {
    let cancelled = false
    Promise.all([
      api.telegram.get(),
      api.precheck.get(),
      api.tls.get(),
      api.demo.get(),
      api.ipRotation.get(),
      api.audit.list(100),
    ])
      .then(([t, p, h, d, r, a]) => {
        if (cancelled) return
        setTg(t)
        setPrecheck(p.enabled)
        setTls(h)
        setTlsDomain(h.domain)
        setTlsEmail(h.email)
        setTlsStaging(h.staging)
        setDemoEnabled(d.enabled)
        setRotation(r)
        setRotationForm({ cooldown_sec: r.cooldown_sec, daily_limit: r.daily_limit })
        setAuditLogs(a)
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message)
      })
    return () => {
      cancelled = true
    }
  }, [])

  async function saveTelegram() {
    setSavingTG(true)
    setError('')
    setNotice('')
    try {
      const saved = await api.telegram.save({
        enabled: tg.enabled,
        bot_token: token,
        chat_id: tg.chat_id,
        login_otp: tg.login_otp,
        event_grab: tg.event_grab,
      })
      setTg(saved)
      setToken('')
      setNotice('Telegram 配置已保存')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSavingTG(false)
    }
  }

  async function testTelegram() {
    setTestingTG(true)
    setError('')
    setNotice('')
    try {
      await api.telegram.test()
      setNotice('测试消息已发送，请检查 Telegram')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setTestingTG(false)
    }
  }

  async function savePrecheck(enabled: boolean) {
    setSavingPrecheck(true)
    setError('')
    setNotice('')
    try {
      const res = await api.precheck.save(enabled)
      setPrecheck(res.enabled)
      setNotice(res.enabled ? '已开启抢购前容量预检' : '已关闭容量预检，抢购将直接尝试创建实例')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSavingPrecheck(false)
    }
  }

  async function saveTLS() {
    setSavingTLS(true)
    setError('')
    setNotice('')
    try {
      const saved = await api.tls.save({
        enabled: true,
        domain: tlsDomain,
        email: tlsEmail,
        cf_api_token: cfToken,
        staging: tlsStaging,
      })
      setTls(saved)
      setCfToken('')
      setNotice('证书申请成功，HTTPS 已生效，无需重启容器')
    } catch (err) {
      setError((err as Error).message)
      const latest = await api.tls.get().catch(() => null)
      if (latest) setTls(latest)
    } finally {
      setSavingTLS(false)
    }
  }

  async function disableTLS() {
    setDisablingTLS(true)
    setError('')
    setNotice('')
    try {
      const saved = await api.tls.disable()
      setTls(saved)
      setNotice('已关闭 HTTPS。若当前只映射了 443，请重跑安装脚本恢复 HTTP 端口映射。')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setDisablingTLS(false)
    }
  }

  async function changePassword(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setNotice('')
    if (newPassword !== confirmPassword) {
      setError('两次输入的新密码不一致')
      return
    }
    setSavingPassword(true)
    try {
      await api.auth.changePassword(username, oldPassword, newPassword)
      setOldPassword('')
      setNewPassword('')
      setConfirmPassword('')
      setNotice('密码已修改')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSavingPassword(false)
    }
  }

  async function saveDemo(enabled: boolean) {
    setSavingDemo(true)
    setError('')
    setNotice('')
    try {
      const res = await api.demo.set(enabled)
      setDemoEnabled(res.enabled)
      setNotice(res.enabled ? '演示模式已开启' : '演示模式已关闭')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSavingDemo(false)
    }
  }

  async function saveRotation() {
    setSavingRotation(true)
    setError('')
    setNotice('')
    try {
      const res = await api.ipRotation.save({
        cooldown_sec: rotationForm.cooldown_sec,
        daily_limit: rotationForm.daily_limit,
      })
      setRotation(res)
      setNotice('换 IP 保护策略已保存')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSavingRotation(false)
    }
  }

  async function loadAudit() {
    setAuditLoading(true)
    setError('')
    try {
      setAuditLogs(await api.audit.list(100))
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setAuditLoading(false)
    }
  }

  const TABS: { id: Tab; label: string }[] = [
    { id: 'telegram', label: 'Telegram 通知' },
    { id: 'grabber', label: '抢购策略' },
    { id: 'tls', label: 'HTTPS 证书' },
    { id: 'password', label: '修改密码' },
    { id: 'security', label: '登录与安全' },
    { id: 'demo', label: '演示模式' },
    { id: 'ipRotation', label: '换 IP 保护' },
    { id: 'audit', label: '审计日志' },
  ]

  // Passkey 服务端硬门禁的展示侧：WebAuthn 要求安全上下文 + 域名 RP ID
  const isIpHost =
    /^[\d.]+$/.test(access.host) || access.host === '::1' || access.host.includes(':')
  const passkeyReason = !access.secure
    ? '当前为明文 HTTP 访问，需先启用 HTTPS'
    : isIpHost
      ? '当前通过 IP 访问（WebAuthn 的 RP ID 必须是域名）'
      : ''
  const passkeyReady = !passkeyReason && tls.cert_issued

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}
      {notice && <SuccessBanner message={notice} onClose={() => setNotice('')} />}

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

      {/* ---------------- Telegram ---------------- */}
      {tab === 'telegram' && (
        <Card title="Telegram 通知">
          <p className="mb-4 text-xs leading-relaxed text-oracle-muted">
            配置后可接收登录提醒、登录验证码与 ARM 抢购结果。Bot Token 以加密形式存库，接口不回传明文。
          </p>
          <div className="max-w-2xl space-y-3">
            <Switch
              checked={tg.enabled}
              onChange={(c) => setTg((s) => ({ ...s, enabled: c }))}
              label="启用 Telegram 通知"
            />

            <Field
              label="Bot Token"
              hint={
                tg.has_token
                  ? '已保存 Token，留空表示不修改。可随时粘贴新值覆盖。'
                  : '在 Telegram 找 @BotFather，发送 /newbot 创建机器人后获得。'
              }
            >
              <input
                type="password"
                className="oci-input font-mono text-xs"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder={tg.has_token ? '（已保存，留空不修改）' : '123456789:AAFxxxxxxxxxxxxxxxxxxxx'}
              />
            </Field>

            <Field
              label="Chat ID"
              hint="给机器人发一条消息，然后访问 https://api.telegram.org/bot<TOKEN>/getUpdates 查看 chat.id。"
            >
              <input
                className="oci-input font-mono text-xs"
                value={tg.chat_id}
                onChange={(e) => setTg((s) => ({ ...s, chat_id: e.target.value }))}
                placeholder="-1001234567890"
              />
            </Field>

            <div className="space-y-2 border-t border-oracle-border pt-3">
              <Switch
                checked={tg.login_otp}
                onChange={(c) => setTg((s) => ({ ...s, login_otp: c }))}
                label="登录时要求输入 Telegram 验证码（两步验证）"
              />
              <Switch
                checked={tg.event_grab}
                onChange={(c) => setTg((s) => ({ ...s, event_grab: c }))}
                label="抢购任务成功或终止时推送通知"
              />
            </div>

            <div className="flex flex-wrap items-center gap-2 pt-2">
              <button
                type="button"
                className="oci-btn-primary"
                disabled={savingTG}
                onClick={() => void saveTelegram()}
              >
                {savingTG ? '保存中…' : '保存'}
              </button>
              <button
                type="button"
                className="oci-btn-secondary"
                disabled={testingTG || !tg.enabled || tg.chat_id === ''}
                onClick={() => void testTelegram()}
              >
                {testingTG ? '发送中…' : '发送测试消息'}
              </button>
              <span className="ml-1 inline-flex items-center gap-2 text-xs text-oracle-muted">
                当前状态：
                {tg.enabled && tg.has_token && tg.chat_id ? (
                  <Badge tone="success">已配置</Badge>
                ) : (
                  <Badge tone="neutral">未配置</Badge>
                )}
              </span>
            </div>
          </div>
        </Card>
      )}

      {/* ---------------- 抢购策略 ---------------- */}
      {tab === 'grabber' && (
        <Card title="抢购前容量预检">
          <div className="max-w-3xl space-y-4">
            <Notice tone="info">
              开启后，抢购任务每轮先调用 OCI <strong>Compute Capacity Report</strong> 只读接口判断是否有货，
              <strong>只有显示 AVAILABLE 才会发起 LaunchInstance</strong>。绝大多数轮询因此不再产生写调用，
              能显著降低高频创建请求触发风控的概率。
              <br />
              若该账户无权使用容量报告接口，任务会自动降级为直接创建，并在日志中标注，不会中断抢购。
            </Notice>

            <Switch
              checked={precheck}
              disabled={savingPrecheck}
              onChange={(c) => void savePrecheck(c)}
              label="启用抢购前容量预检（推荐）"
            />

            <dl className="grid grid-cols-1 gap-3 border-t border-oracle-border pt-4 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-xs text-oracle-muted">当前状态</dt>
                <dd className="mt-1">
                  <Badge tone={precheck ? 'success' : 'warning'}>
                    {precheck ? '已开启' : '已关闭'}
                  </Badge>
                </dd>
              </div>
              <div>
                <dt className="text-xs text-oracle-muted">预检接口</dt>
                <dd className="mt-1 font-mono text-xs">POST /20160918/capacityReports</dd>
              </div>
            </dl>

            <p className="text-xs leading-relaxed text-oracle-muted">
              想单独查看某个机型在各可用域的余量，请到「容量探测」页面手动查询。
              预检与重试间隔共用任务的「重试间隔（秒）」设置，建议保持 60 秒以上。
            </p>
          </div>
        </Card>
      )}

      {/* ---------------- HTTPS ---------------- */}
      {tab === 'tls' && (
        <Card
          title="HTTPS 证书（Cloudflare DNS-01）"
          extra={
            <span className="text-xs text-oracle-muted">
              {tls.cert_issued ? (
                <Badge tone="success">证书已生效</Badge>
              ) : (
                <Badge tone="neutral">未启用</Badge>
              )}
            </span>
          }
        >
          <div className="max-w-2xl space-y-4">
            <Notice tone="info">
              通过 Cloudflare API 自动添加 <code className="font-mono">_acme-challenge</code> TXT 记录，
              向 Let's Encrypt 申请证书。不需要开放 80 端口，也不需要提前把域名解析到本机。
              证书保存在数据卷中，剩余有效期不足 30 天时自动续期。
            </Notice>

            <Field label="域名" hint="例如 oci.example.com，该域名的 DNS 必须托管在 Cloudflare">
              <input
                className="oci-input"
                value={tlsDomain}
                onChange={(e) => setTlsDomain(e.target.value)}
                placeholder="oci.example.com"
              />
            </Field>

            <Field label="邮箱" hint="用于接收 Let's Encrypt 的到期与异常通知">
              <input
                type="email"
                className="oci-input"
                value={tlsEmail}
                onChange={(e) => setTlsEmail(e.target.value)}
                placeholder="you@example.com"
              />
            </Field>

            <Field
              label="Cloudflare API Token"
              hint={
                tls.has_token
                  ? '已保存 Token，留空表示不修改。'
                  : '在 Cloudflare → My Profile → API Tokens 创建，需要 Zone:DNS:Edit 与 Zone:Zone:Read 权限。'
              }
            >
              <input
                type="password"
                className="oci-input font-mono text-xs"
                value={cfToken}
                onChange={(e) => setCfToken(e.target.value)}
                placeholder={tls.has_token ? '（已保存，留空不修改）' : '粘贴 CF API Token'}
              />
            </Field>

            <Switch
              checked={tlsStaging}
              onChange={setTlsStaging}
              label={
                <span className="text-xs text-oracle-muted">
                  使用 Let&apos;s Encrypt 测试环境（仅调试用，浏览器仍会提示证书不受信任）
                </span>
              }
            />

            <div className="flex flex-wrap items-center gap-2 border-t border-oracle-border pt-4">
              <button
                type="button"
                className="oci-btn-primary"
                disabled={savingTLS}
                onClick={() => void saveTLS()}
              >
                {savingTLS ? '申请中（约 1–3 分钟）…' : tls.cert_issued ? '重新申请 / 续期' : '启用 HTTPS'}
              </button>
              <button
                type="button"
                className="oci-btn-danger"
                disabled={disablingTLS || !tls.cert_issued}
                title={tls.cert_issued ? '关闭 HTTPS' : '尚未签发证书，无需关闭'}
                onClick={() => void disableTLS()}
              >
                {disablingTLS ? '处理中…' : '关闭 HTTPS'}
              </button>
            </div>

            <dl className="grid grid-cols-1 gap-3 border-t border-oracle-border pt-4 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-xs text-oracle-muted">HTTPS 监听</dt>
                <dd className="mt-1 font-mono text-xs">
                  容器 {tls.https_port || ':8443'} → 宿主机 443
                </dd>
              </div>
              <div>
                <dt className="text-xs text-oracle-muted">证书有效期至</dt>
                <dd className="mt-1 text-xs">
                  {tls.expires_at ? formatDateTime(tls.expires_at) : '—'}
                </dd>
              </div>
            </dl>

            {tls.last_error && (
              <Notice tone="danger" className="break-all">
                上次申请失败：{tls.last_error}
              </Notice>
            )}

            <p className="text-xs leading-relaxed text-oracle-muted">
              证书签发成功后访问 <code className="font-mono">https://{tlsDomain || '你的域名'}</code>。
              首次部署只映射 HTTP 端口，未开放 443；签发证书后<strong>重跑一次安装脚本</strong>，
              脚本会改为只映射 HTTPS 并关闭宿主机 HTTP 端口（容器内健康检查端口不受影响）。
              请放行云厂商安全组的 443 端口。纯 IP 证书不受支持，Passkey 也只在域名 + HTTPS 下可用。
            </p>
          </div>
        </Card>
      )}

      {/* ---------------- 登录与安全 ---------------- */}
      {tab === 'security' && (
        <Card title="登录与安全">
          <div className="max-w-3xl">
            <p className="oci-inline-hint mb-4">
              下列是当前面板真实启用的登录保护方式。新增 TOTP 或通行密钥后会自动出现在这里；
              未配置的方式不会伪装成可点控件。
            </p>
            <div className="grid grid-cols-1 gap-x-10 sm:grid-cols-2">
              <div className="oci-kv-row">
                <div className="oci-kv-label">管理员账号</div>
                <div className="oci-kv-value">{username || '—'}</div>
              </div>
              <div className="oci-kv-row">
                <div className="oci-kv-label">密码</div>
                <div className="oci-kv-value">
                  <Badge tone="success">已设置</Badge>
                </div>
              </div>
              <div className="oci-kv-row">
                <div className="oci-kv-label">Telegram 登录验证码</div>
                <div className="oci-kv-value">
                  <Badge tone={tg.login_otp ? 'success' : 'neutral'}>
                    {tg.login_otp ? '已启用' : '未启用'}
                  </Badge>
                </div>
              </div>
              <div className="oci-kv-row">
                <div className="oci-kv-label">Telegram 通知</div>
                <div className="oci-kv-value">
                  <Badge tone={tg.enabled && tg.has_token && tg.chat_id ? 'success' : 'warning'}>
                    {tg.enabled && tg.has_token && tg.chat_id ? '已就绪' : '未配置完成'}
                  </Badge>
                </div>
              </div>
              <div className="oci-kv-row">
                <div className="oci-kv-label">HTTPS 证书</div>
                <div className="oci-kv-value">
                  <Badge tone={tls.cert_issued ? 'success' : 'warning'}>
                    {tls.cert_issued ? `已生效 · ${tls.domain || '—'}` : '未启用'}
                  </Badge>
                </div>
              </div>
              <div className="oci-kv-row">
                <div className="oci-kv-label">当前访问</div>
                <div className="oci-kv-value">
                  {access.secure ? 'HTTPS' : 'HTTP（明文）'} · {access.host}
                </div>
              </div>
              <div className="oci-kv-row">
                <div className="oci-kv-label">两步验证覆盖范围</div>
                <div className="oci-kv-value">登录、IAM 与敏感资源操作</div>
              </div>
              <div className="oci-kv-row">
                <div className="oci-kv-label">备份码 / TOTP</div>
                <div className="oci-kv-value">尚未配置</div>
              </div>
            </div>

            <div className="mt-5 border-t border-oracle-border pt-3">
              <div className="oci-kv-title">通行密钥（Passkey）</div>
              <Notice tone={passkeyReady ? 'info' : 'warning'}>
                {passkeyReady ? (
                  <>
                    当前满足「域名 + HTTPS」条件。通行密钥的注册与登录将在下一批改动中提供，
                    届时需要你在真实浏览器上完成一次注册与登录验证；密码登录会保留作为恢复通道。
                  </>
                ) : (
                  <>
                    不可用：{passkeyReason || '证书尚未签发'}。WebAuthn 只在 HTTPS 安全上下文可用，
                    且 RP ID 必须绑定域名，纯 IP 证书无法使用。TOTP 与 Telegram 验证码不受此限制。
                  </>
                )}
              </Notice>
              <p className="oci-inline-hint mt-3">
                纯 IP 证书方案已按你的决定完全放弃；证书只支持 Cloudflare DNS-01 域名证书。
              </p>
            </div>
          </div>
        </Card>
      )}

      {/* ---------------- 密码 ---------------- */}
      {tab === 'password' && (
        <Card title="修改密码">
          <form onSubmit={changePassword} className="max-w-md space-y-3">
            <Field label="当前密码">
              <input
                type="password"
                className="oci-input"
                value={oldPassword}
                onChange={(e) => setOldPassword(e.target.value)}
                autoComplete="current-password"
              />
            </Field>
            <Field label="新密码" hint="至少 8 位">
              <input
                type="password"
                className="oci-input"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                autoComplete="new-password"
              />
            </Field>
            <Field label="确认新密码">
              <input
                type="password"
                className="oci-input"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                autoComplete="new-password"
              />
            </Field>
            <button type="submit" className="oci-btn-primary" disabled={savingPassword}>
              {savingPassword ? '保存中…' : '修改密码'}
            </button>
          </form>
        </Card>
      )}

      {/* ---------------- 演示模式 ---------------- */}
      {tab === 'demo' && (
        <Card title="演示模式">
          <div className="max-w-2xl space-y-4">
            <Notice tone="info">
              开启后，账号列表会自动提供演示账号（demo-account）：所有接口返回模拟数据，
              不会发出任何真实 OCI 请求，验证码固定为 123456，可放心试用全部页面与交互。
            </Notice>
            <Switch
              checked={demoEnabled}
              disabled={savingDemo}
              onChange={(c) => void saveDemo(c)}
              label="启用演示模式"
            />
            <dl className="grid grid-cols-1 gap-3 border-t border-oracle-border pt-4 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-xs text-oracle-muted">当前状态</dt>
                <dd className="mt-1">
                  <Badge tone={demoEnabled ? 'success' : 'neutral'}>
                    {demoEnabled ? '已开启' : '已关闭'}
                  </Badge>
                </dd>
              </div>
              <div>
                <dt className="text-xs text-oracle-muted">演示账号 ID</dt>
                <dd className="mt-1 font-mono text-xs">demo-account</dd>
              </div>
            </dl>
          </div>
        </Card>
      )}

      {/* ---------------- 换 IP 保护策略 ---------------- */}
      {tab === 'ipRotation' && (
        <Card title="换 IP 保护策略">
          <div className="max-w-2xl space-y-4">
            <Notice tone="info">
              更换公网 IP 受冷却时间与每日次数限制，且需要二次验证。此处调整的策略对所有账号生效。
            </Notice>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <Field label="冷却时间（秒）">
                <input
                  type="number"
                  min={0}
                  className="oci-input"
                  value={rotationForm.cooldown_sec}
                  onChange={(e) =>
                    setRotationForm((f) => ({ ...f, cooldown_sec: Number(e.target.value) }))
                  }
                />
              </Field>
              <Field label="每日次数上限">
                <input
                  type="number"
                  min={0}
                  className="oci-input"
                  value={rotationForm.daily_limit}
                  onChange={(e) =>
                    setRotationForm((f) => ({ ...f, daily_limit: Number(e.target.value) }))
                  }
                />
              </Field>
            </div>
            <button
              type="button"
              className="oci-btn-primary"
              disabled={savingRotation}
              onClick={() => void saveRotation()}
            >
              {savingRotation ? '保存中…' : '保存策略'}
            </button>
            {rotation && (
              <dl className="grid grid-cols-2 gap-3 border-t border-oracle-border pt-4 text-sm sm:grid-cols-4">
                <div>
                  <dt className="text-xs text-oracle-muted">今日已用</dt>
                  <dd className="mt-1">{rotation.used_today}</dd>
                </div>
                <div>
                  <dt className="text-xs text-oracle-muted">今日剩余</dt>
                  <dd className="mt-1">{rotation.remaining_today}</dd>
                </div>
                <div>
                  <dt className="text-xs text-oracle-muted">每日上限</dt>
                  <dd className="mt-1">{rotation.daily_limit}</dd>
                </div>
                <div>
                  <dt className="text-xs text-oracle-muted">下次允许时间</dt>
                  <dd className="mt-1 text-xs">{formatDateTime(rotation.next_allowed_at)}</dd>
                </div>
              </dl>
            )}
          </div>
        </Card>
      )}

      {/* ---------------- 审计日志 ---------------- */}
      {tab === 'audit' && (
        <Card
          title="审计日志（最近 50 条）"
          extra={
            <button
              type="button"
              className="oci-btn-secondary"
              disabled={auditLoading}
              onClick={() => void loadAudit()}
            >
              <Icon name="refresh" size={15} />
              刷新
            </button>
          }
          bodyClassName="oci-card-body--flush"
        >
          {auditLoading && auditLogs.length === 0 ? (
            <div className="p-6">
              <Loading />
            </div>
          ) : auditLogs.length === 0 ? (
            <EmptyState />
          ) : (
            <div className="oci-table-wrap">
              <table className="oci-table">
                <thead>
                  <tr>
                    <th>时间</th>
                    <th>分类</th>
                    <th>操作</th>
                    <th>目标</th>
                    <th>结果</th>
                    <th>详情</th>
                    <th>账号</th>
                    <th>客户端 IP</th>
                  </tr>
                </thead>
                <tbody>
                  {auditLogs.slice(0, 50).map((log) => (
                    <tr key={log.id}>
                      <td className="text-xs text-oracle-muted">{formatDateTime(log.created_at)}</td>
                      <td className="text-xs">{CATEGORY_LABEL[log.category] ?? log.category}</td>
                      <td className="text-xs" title={log.operation}>
                        {humanOperation(log.operation)}
                      </td>
                      <td className="font-mono text-xs" title={log.target}>
                        {truncate(log.target, 18, 0)}
                      </td>
                      <td>
                        <Badge tone={humanResult(log.result).tone}>
                          {humanResult(log.result).label}
                        </Badge>
                      </td>
                      <td className="text-xs">{log.detail || '—'}</td>
                      <td className="font-mono text-xs" title={log.account_id}>
                        {truncate(log.account_id, 14, 0)}
                      </td>
                      <td className="font-mono text-xs">{log.client_ip || '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}
    </div>
  )
}
