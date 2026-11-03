import { useState } from 'react'
import { ErrorBanner, Field } from '../components/ui'
import { useAuth } from '../lib/auth'

export default function Login() {
  const { status, setup, login, verifyOtp, pendingChallenge } = useAuth()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

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
    <div className="flex min-h-full items-center justify-center bg-oracle-surface px-4 py-12">
      <div className="w-full max-w-md">
        <div className="mb-6 flex items-center justify-center gap-2 text-lg font-semibold">
          <span className="inline-block h-4 w-4 rounded-sm bg-oracle-red" />
          OCI 管理器
        </div>

        <div className="oci-card p-6">
          {error && <ErrorBanner message={error} onClose={() => setError('')} />}

          {pendingChallenge ? (
            <form onSubmit={submitOtp} className="space-y-4">
              <div>
                <h1 className="text-base font-semibold">两步验证</h1>
                <p className="mt-1 text-xs text-oracle-muted">
                  已向 Telegram 发送 6 位验证码，5 分钟内有效。
                </p>
              </div>
              <Field label="验证码">
                <input
                  className="oci-input text-center font-mono text-lg tracking-[0.5em]"
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  placeholder="000000"
                  maxLength={6}
                  autoFocus
                  inputMode="numeric"
                />
              </Field>
              <button type="submit" className="oci-btn-primary w-full" disabled={busy}>
                {busy ? '验证中…' : '验证并登录'}
              </button>
            </form>
          ) : needSetup ? (
            <form onSubmit={submitSetup} className="space-y-4">
              <div>
                <h1 className="text-base font-semibold">初始化设置</h1>
                <p className="mt-1 text-xs text-oracle-muted">
                  首次使用需要设置管理员账号，设置后其他人才无法访问本面板。
                </p>
              </div>
              <Field label="用户名" hint="3–32 个字符">
                <input
                  className="oci-input"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoFocus
                  autoComplete="username"
                />
              </Field>
              <Field label="密码" hint="至少 8 位">
                <input
                  type="password"
                  className="oci-input"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="new-password"
                />
              </Field>
              <Field label="确认密码">
                <input
                  type="password"
                  className="oci-input"
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  autoComplete="new-password"
                />
              </Field>
              <button type="submit" className="oci-btn-primary w-full" disabled={busy}>
                {busy ? '设置中…' : '创建账号'}
              </button>
            </form>
          ) : (
            <form onSubmit={submitLogin} className="space-y-4">
              <div>
                <h1 className="text-base font-semibold">登录</h1>
                <p className="mt-1 text-xs text-oracle-muted">
                  {status?.login_otp
                    ? '登录后会向 Telegram 发送验证码。'
                    : '输入管理员账号密码继续。'}
                </p>
              </div>
              <Field label="用户名">
                <input
                  className="oci-input"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoFocus
                  autoComplete="username"
                />
              </Field>
              <Field label="密码">
                <input
                  type="password"
                  className="oci-input"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="current-password"
                />
              </Field>
              <button type="submit" className="oci-btn-primary w-full" disabled={busy}>
                {busy ? '登录中…' : '登录'}
              </button>
            </form>
          )}
        </div>

        <p className="mt-4 text-center text-xs text-oracle-muted">
          忘记密码需要登录服务器删除数据库中的 admins 表后重新初始化
        </p>
      </div>
    </div>
  )
}
