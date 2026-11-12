import { useEffect, useState } from 'react'
import { Badge, Card, ErrorBanner, Field, SuccessBanner } from '../components/ui'
import { api } from '../lib/api'
import { useAuth } from '../lib/auth'
import { formatDateTime } from '../lib/format'
import type { TelegramSettings, TLSSettings } from '../lib/types'

type Tab = 'telegram' | 'grabber' | 'tls' | 'password'

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

export default function Settings() {
  const { username } = useAuth()
  const [tab, setTab] = useState<Tab>('telegram')

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

  useEffect(() => {
    let cancelled = false
    Promise.all([api.telegram.get(), api.precheck.get(), api.tls.get()])
      .then(([t, p, h]) => {
        if (cancelled) return
        setTg(t)
        setPrecheck(p.enabled)
        setTls(h)
        setTlsDomain(h.domain)
        setTlsEmail(h.email)
        setTlsStaging(h.staging)
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
      setNotice('已关闭 HTTPS，HTTP 访问始终保留')
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

  const TABS: { id: Tab; label: string }[] = [
    { id: 'telegram', label: 'Telegram 通知' },
    { id: 'grabber', label: '抢购策略' },
    { id: 'tls', label: 'HTTPS 证书' },
    { id: 'password', label: '修改密码' },
  ]

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
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={tg.enabled}
                onChange={(e) => setTg((s) => ({ ...s, enabled: e.target.checked }))}
              />
              启用 Telegram 通知
            </label>

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
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={tg.login_otp}
                  onChange={(e) => setTg((s) => ({ ...s, login_otp: e.target.checked }))}
                />
                登录时要求输入 Telegram 验证码（两步验证）
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={tg.event_grab}
                  onChange={(e) => setTg((s) => ({ ...s, event_grab: e.target.checked }))}
                />
                抢购任务成功或终止时推送通知
              </label>
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
              <span className="ml-1 text-xs text-oracle-muted">
                当前状态：
                {tg.enabled && tg.has_token && tg.chat_id ? (
                  <Badge tone="border-[#bbe3cb] bg-[#f0faf4] text-[#136437]">已配置</Badge>
                ) : (
                  <Badge tone="border-[#d9d6d2] bg-[#f5f4f2] text-[#5c5854]">未配置</Badge>
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
            <div className="oci-notice oci-notice-info">
              开启后，抢购任务每轮先调用 OCI <strong>Compute Capacity Report</strong> 只读接口判断是否有货，
              <strong>只有显示 AVAILABLE 才会发起 LaunchInstance</strong>。绝大多数轮询因此不再产生写调用，
              能显著降低高频创建请求触发风控的概率。
              <br />
              若该账户无权使用容量报告接口，任务会自动降级为直接创建，并在日志中标注，不会中断抢购。
            </div>

            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={precheck}
                disabled={savingPrecheck}
                onChange={(e) => void savePrecheck(e.target.checked)}
              />
              启用抢购前容量预检（推荐）
            </label>

            <dl className="grid grid-cols-1 gap-3 border-t border-oracle-border pt-4 text-sm sm:grid-cols-2">
              <div>
                <dt className="text-xs text-oracle-muted">当前状态</dt>
                <dd className="mt-1">
                  <Badge
                    tone={
                      precheck
                        ? 'border-[#bbe3cb] bg-[#f0faf4] text-[#136437]'
                        : 'border-[#f0d9b0] bg-[#fdf7ec] text-[#8a5200]'
                    }
                  >
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
                <Badge tone="border-[#bbe3cb] bg-[#f0faf4] text-[#136437]">证书已生效</Badge>
              ) : (
                <Badge tone="border-[#d9d6d2] bg-[#f5f4f2] text-[#5c5854]">未启用</Badge>
              )}
            </span>
          }
        >
          <div className="max-w-2xl space-y-4">
            <div className="oci-notice oci-notice-info">
              通过 Cloudflare API 自动添加 <code className="font-mono">_acme-challenge</code> TXT 记录，
              向 Let's Encrypt 申请证书。不需要开放 80 端口，也不需要提前把域名解析到本机。
              证书保存在数据卷中，剩余有效期不足 30 天时自动续期。
            </div>

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

            <label className="flex items-center gap-2 text-xs text-oracle-muted">
              <input
                type="checkbox"
                checked={tlsStaging}
                onChange={(e) => setTlsStaging(e.target.checked)}
              />
              使用 Let's Encrypt 测试环境（仅调试用，浏览器仍会提示证书不受信任）
            </label>

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
                disabled={disablingTLS || !tls.enabled}
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
              <div className="oci-notice oci-notice-error break-all">
                上次申请失败：{tls.last_error}
              </div>
            )}

            <p className="text-xs leading-relaxed text-oracle-muted">
              启用后访问 <code className="font-mono">https://{tlsDomain || '你的域名'}</code>。
              <strong>HTTP 端口始终保留</strong>，即使证书配置失败也能回到本页修改。
              请放行云厂商安全组的 443 端口。
            </p>
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
    </div>
  )
}
