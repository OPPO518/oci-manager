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
          <svg
            className="oci-auth-brand-logo"
            viewBox="0 0 24 24"
            aria-hidden="true"
            focusable="false"
          >
            <path
              d="M7.2 18.2a4 4 0 0 1 .4-7.97 6 6 0 0 1 11.5 1.27A3.5 3.5 0 0 1 18 18.2H7.2z"
              fill="#c74634"
            />
          </svg>
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

// 左侧黑栏的点阵球装饰：纯 DOM 圆点模拟点阵地球，不用任何图片素材。
// 距中心越近越亮，四角点缀 Oracle 红，营造科技感而不喧宾夺主。
const GLOBE_DOTS = (() => {
  const n = 9
  const c = (n - 1) / 2
  const accents = new Set(['2,2', '6,2', '2,6', '6,6'])
  const dots: { op: string; accent: boolean }[] = []
  for (let y = 0; y < n; y++) {
    for (let x = 0; x < n; x++) {
      const dx = x - c
      const dy = y - c
      const d = Math.sqrt(dx * dx + dy * dy)
      if (d > c + 0.35) continue
      const t = 1 - d / (c + 0.35)
      dots.push({ op: (0.22 + 0.78 * t).toFixed(2), accent: accents.has(`${x},${y}`) })
    }
  }
  return dots
})()

export default function Login() {
  const { status, setup, login, verifyOtp, pendingChallenge, expire } = useAuth()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [showMethods, setShowMethods] = useState(false)
  const [showPass, setShowPass] = useState(false)
  const [remember, setRemember] = useState(false)

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
      await login(username, password, remember)
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
      {/* 顶栏右上控件：语言 / 主题（深浅色系统尚未实现，先放占位） */}
      <div className="oci-login-topbar">
        <button type="button" className="oci-login-topbtn" title="多语言即将推出" aria-label="切换语言">
          <Icon name="globe" size={16} />
        </button>
        <button type="button" className="oci-login-topbtn" title="深色模式即将推出" aria-label="切换主题">
          <Icon name="moon" size={16} />
        </button>
      </div>

      {/* 左：品牌区（占窗口 1/3） */}
      <div className="oci-login-brand">
        <div className="oci-login-brand-top">
          <span className="oci-wordmark">ORACLE</span>
          <div className="oci-login-brand-tagline">Oracle Cloud Infrastructure</div>
        </div>
        <div className="oci-login-brand-globe" aria-hidden="true">
          {GLOBE_DOTS.map((d, i) => (
            <span
              key={i}
              style={{
                width: 9,
                height: 9,
                borderRadius: '50%',
                background: d.accent ? '#c74634' : `rgba(255,255,255,${d.op})`,
              }}
            />
          ))}
        </div>
        <div className="oci-login-brand-bottom">
          <p className="oci-login-brand-copy">
            在一个控制台中管理计算实例、容量探测、抢购任务与成本用量。
            以 Oracle Redwood 设计语言构建，安全、清晰地掌控你的云资源。
          </p>
          <div className="oci-login-brand-stat">管理多个 OCI 账号 · 免费 ARM 抢购一体</div>
        </div>
      </div>

      {/* 右：认证卡（占窗口 2/3） */}
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
                  <div className="oci-input-wrap">
                    <span className="oci-input-icon">
                      <Icon name="key" size={16} />
                    </span>
                    <input
                      id="otp-code"
                      className="oci-input oci-input-has-icon text-center font-mono text-lg tracking-[0.5em]"
                      value={code}
                      onChange={(e) => setCode(e.target.value)}
                      placeholder="000000"
                      maxLength={6}
                      autoFocus
                      inputMode="numeric"
                      autoComplete="one-time-code"
                    />
                  </div>
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
                  <div className="oci-input-wrap">
                    <span className="oci-input-icon">
                      <Icon name="user" size={16} />
                    </span>
                    <input
                      id="setup-user"
                      className="oci-input oci-input-has-icon"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      autoFocus
                      autoComplete="username"
                      placeholder="3–32 个字符"
                    />
                  </div>
                </div>
                <div className="oci-auth-field">
                  <label htmlFor="setup-pass">密码</label>
                  <div className="oci-input-wrap">
                    <span className="oci-input-icon">
                      <Icon name="lock" size={16} />
                    </span>
                    <input
                      id="setup-pass"
                      type={showPass ? 'text' : 'password'}
                      className="oci-input oci-input-has-icon oci-input-has-eye"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      autoComplete="new-password"
                      placeholder="至少 8 位"
                    />
                    <button
                      type="button"
                      className="oci-input-eye"
                      onClick={() => setShowPass((v) => !v)}
                      aria-label={showPass ? '隐藏密码' : '显示密码'}
                    >
                      <Icon name={showPass ? 'eye-off' : 'eye'} size={16} />
                    </button>
                  </div>
                </div>
                <div className="oci-auth-field">
                  <label htmlFor="setup-confirm">确认密码</label>
                  <div className="oci-input-wrap">
                    <span className="oci-input-icon">
                      <Icon name="lock" size={16} />
                    </span>
                    <input
                      id="setup-confirm"
                      type={showPass ? 'text' : 'password'}
                      className="oci-input oci-input-has-icon oci-input-has-eye"
                      value={confirm}
                      onChange={(e) => setConfirm(e.target.value)}
                      autoComplete="new-password"
                    />
                    <button
                      type="button"
                      className="oci-input-eye"
                      onClick={() => setShowPass((v) => !v)}
                      aria-label={showPass ? '隐藏密码' : '显示密码'}
                    >
                      <Icon name={showPass ? 'eye-off' : 'eye'} size={16} />
                    </button>
                  </div>
                </div>
                <button type="submit" className="oci-auth-submit" disabled={busy || !username || !password}>
                  {busy ? '创建中…' : '创建账号'}
                </button>
              </form>
            </AuthCard>
          ) : (
            /* ---------------- 登录（首屏只有用户名和密码） ---------------- */
            <AuthCard title="Manager">
              <form onSubmit={submitLogin} className="oci-auth-form">
                <div className="oci-auth-field">
                  <label htmlFor="login-user">用户名</label>
                  <div className="oci-input-wrap">
                    <span className="oci-input-icon">
                      <Icon name="user" size={16} />
                    </span>
                    <input
                      id="login-user"
                      className="oci-input oci-input-has-icon"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      placeholder="/用户名"
                      autoFocus
                      autoComplete="username"
                    />
                  </div>
                </div>
                <div className="oci-auth-field">
                  <label htmlFor="login-pass">密码</label>
                  <div className="oci-input-wrap">
                    <span className="oci-input-icon">
                      <Icon name="lock" size={16} />
                    </span>
                    <input
                      id="login-pass"
                      type={showPass ? 'text' : 'password'}
                      className="oci-input oci-input-has-icon oci-input-has-eye"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      autoComplete="current-password"
                    />
                    <button
                      type="button"
                      className="oci-input-eye"
                      onClick={() => setShowPass((v) => !v)}
                      aria-label={showPass ? '隐藏密码' : '显示密码'}
                    >
                      <Icon name={showPass ? 'eye-off' : 'eye'} size={16} />
                    </button>
                  </div>
                </div>
                <label className="oci-auth-remember">
                  <input
                    type="checkbox"
                    checked={remember}
                    onChange={(e) => setRemember(e.target.checked)}
                  />
                  记住我
                </label>
                <button type="submit" className="oci-auth-submit" disabled={busy || !username || !password}>
                  {busy ? '登录中…' : '登录'}
                </button>
                <div className="oci-auth-security">
                  <Icon name="shield" size={14} />
                  端到端加密 · 自托管安全保障
                </div>
              </form>
            </AuthCard>
          )}
        </div>
        <div className="oci-login-attrib">OCI Manager · 自托管面板</div>
      </div>
    </div>
  )
}
