import { useState, type ReactNode } from 'react'
import { ErrorBanner, Icon } from '../components/ui'
import { useAuth } from '../lib/auth'

/**
 * 统一认证卡：登录 / 二次验证 / 首次初始化共用同一套 Oracle 官网外观
 * （顶部彩色细条 + 深色 ORACLE Cloud 字标 + 黑色宽主按钮 + 蓝色次要链接）。
 */
function AuthCard({
  title,
  subtitle,
  account,
  children,
  links,
}: {
  title: string
  subtitle?: string
  account?: string
  children: ReactNode
  links?: ReactNode
}) {
  return (
    <div className="oci-auth-card">
      <div className="oci-auth-strip" />
      <div className="oci-auth-body">
        <div className="oci-auth-brand">
          <span className="oci-wordmark">ORACLE</span>
          <span className="oci-auth-brand-cloud">Cloud</span>
        </div>
        <h1 className="oci-auth-title">{title}</h1>
        {subtitle && <p className="oci-auth-sub">{subtitle}</p>}
        {account && (
          <div className="oci-auth-account">
            <Icon name="user" size={15} />
            管理员：{account}
          </div>
        )}
        {children}
        {links && <div className="oci-auth-links">{links}</div>}
      </div>
    </div>
  )
}

export default function Login() {
  const { status, setup, login, verifyOtp, pendingChallenge, expire } = useAuth()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [help, setHelp] = useState<'reset' | 'help' | null>(null)
  const [showMethods, setShowMethods] = useState(false)

  const needSetup = !status?.initialized

  async function submitSetup(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    if (password !== confirm) {
      setError('两次输入的密码不一致')
      return
    }
    setBusy(true)
    try {
      await setup(username, password)
      setPassword('')
      setConfirm('')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function submitLogin(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await login(username, password)
      setPassword('')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function submitOtp(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await verifyOtp(code.trim())
      setCode('')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="oci-login-shell">
      {/* 左：品牌区 */}
      <div className="oci-login-brand">
        <span className="oci-wordmark">ORACLE</span>
        <div>
          <div className="oci-login-brand-tagline">Oracle Cloud Infrastructure</div>
          <p className="oci-login-brand-copy mt-4">
            在一个控制台中管理计算实例、容量探测、抢购任务与成本用量。
            <br />
            以 Oracle Redwood 设计语言构建，安全、清晰地掌控你的云资源。
          </p>
        </div>
        <div className="flex items-center gap-3 text-[13px] text-white/60">
          <span className="inline-block h-1 w-8 rounded bg-oracle-brand" />
          <span>OCI 管理器 · 自托管面板</span>
        </div>
      </div>

      {/* 右：认证卡 */}
      <div className="oci-login-panel">
        <div className="oci-login-form">
          {error && (
            <div className="mb-4">
              <ErrorBanner message={error} onClose={() => setError('')} />
            </div>
          )}

          {pendingChallenge ? (
            /* ---------------- 二次验证 ---------------- */
            <AuthCard
              title="两步验证"
              subtitle="请使用已配置的验证方式完成本次登录。"
              account={username || '—'}
            >
              <div className="oci-auth-fido">
                <Icon name="key" size={34} />
              </div>
              <form onSubmit={submitOtp} className="oci-auth-form">
                <div className="oci-auth-field">
                  <label htmlFor="otp-code">Telegram 验证码</label>
                  <input
                    id="otp-code"
                    className="oci-input text-center font-mono text-lg tracking-[0.5em]"
                    value={code}
                    onChange={(e) => setCode(e.target.value)}
                    placeholder="000000"
                    maxLength={6}
                    autoFocus
                    inputMode="numeric"
                    autoComplete="one-time-code"
                  />
                  <span className="oci-inline-hint">
                    {status?.login_otp
                      ? '验证码已推送到 Telegram，5 分钟内有效。'
                      : '当前登录要求输入一次性验证码。'}
                  </span>
                </div>
                <button type="submit" className="oci-auth-submit" disabled={busy || code.trim().length < 6}>
                  {busy ? '验证中…' : '验证'}
                </button>
              </form>

              <button
                type="button"
                className="oci-auth-method-toggle"
                onClick={() => setShowMethods((v) => !v)}
                aria-expanded={showMethods}
              >
                {showMethods ? '隐藏备用登录方法' : '显示备用登录方法'}
              </button>
              {showMethods && (
                <div className="oci-auth-methods">
                  <div className="oci-auth-method-row">
                    <Icon name="lock" size={15} />
                    <span className="flex-1">密码</span>
                    <span className="text-xs text-oracle-muted">已启用</span>
                  </div>
                  <div className="oci-auth-method-row">
                    <Icon name="bell" size={15} />
                    <span className="flex-1">Telegram 验证码</span>
                    <span className="text-xs text-oracle-muted">
                      {status?.login_otp ? '已启用' : '未启用'}
                    </span>
                  </div>
                  <button
                    type="button"
                    className="oci-btn-secondary oci-btn-sm mt-2 w-full"
                    onClick={() => {
                      expire()
                      setCode('')
                      setError('')
                      setShowMethods(false)
                    }}
                  >
                    返回使用密码登录
                  </button>
                </div>
              )}
            </AuthCard>
          ) : needSetup ? (
            /* ---------------- 首次初始化 ---------------- */
            <AuthCard
              title="创建管理员账户"
              subtitle="首次使用需要设置管理员账号，设置后其他人才无法访问本面板。"
            >
              <form onSubmit={submitSetup} className="oci-auth-form">
                <div className="oci-auth-field">
                  <label htmlFor="setup-user">用户名</label>
                  <input
                    id="setup-user"
                    className="oci-input"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    autoFocus
                    autoComplete="username"
                    placeholder="3–32 个字符"
                  />
                </div>
                <div className="oci-auth-field">
                  <label htmlFor="setup-pass">密码</label>
                  <input
                    id="setup-pass"
                    type="password"
                    className="oci-input"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="new-password"
                    placeholder="至少 8 位"
                  />
                </div>
                <div className="oci-auth-field">
                  <label htmlFor="setup-confirm">确认密码</label>
                  <input
                    id="setup-confirm"
                    type="password"
                    className="oci-input"
                    value={confirm}
                    onChange={(e) => setConfirm(e.target.value)}
                    autoComplete="new-password"
                  />
                </div>
                <button type="submit" className="oci-auth-submit" disabled={busy || !username || !password}>
                  {busy ? '创建中…' : '创建账号'}
                </button>
              </form>
            </AuthCard>
          ) : (
            /* ---------------- 登录（首屏只有用户名和密码） ---------------- */
            <AuthCard
              title="Oracle Cloud 账户登录"
              subtitle="使用管理员账号登录 OCI 管理器。"
            >
              <form onSubmit={submitLogin} className="oci-auth-form">
                <div className="oci-auth-field">
                  <label htmlFor="login-user">用户名</label>
                  <input
                    id="login-user"
                    className="oci-input"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    placeholder="/用户名"
                    autoFocus
                    autoComplete="username"
                  />
                </div>
                <div className="oci-auth-field">
                  <label htmlFor="login-pass">密码</label>
                  <input
                    id="login-pass"
                    type="password"
                    className="oci-input"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="current-password"
                  />
                </div>
                <button type="submit" className="oci-auth-submit" disabled={busy || !username || !password}>
                  {busy ? '登录中…' : '登录'}
                </button>
              </form>
              <div className="oci-auth-links">
                <button type="button" onClick={() => setHelp(help === 'reset' ? null : 'reset')}>
                  忘记了密码?
                </button>
                <button type="button" onClick={() => setHelp(help === 'help' ? null : 'help')}>
                  需要登录方面的帮助?
                </button>
              </div>
              {help === 'reset' && (
                <div className="oci-auth-help">
                  本面板为单管理员自托管工具，不提供在线找回密码。请在服务器上停止容器后，
                  删除数据目录中 <code>admins</code> 表的记录并重启，再重新初始化管理员账号。
                  操作前请先备份数据库文件。
                </div>
              )}
              {help === 'help' && (
                <div className="oci-auth-help">
                  请使用首次部署时创建的管理员用户名与密码登录。若启用了 Telegram 两步验证，
                  密码验证通过后还需要输入一次性验证码。连续输错会触发登录限流，请稍后再试。
                </div>
              )}
            </AuthCard>
          )}
        </div>
      </div>
    </div>
  )
}
