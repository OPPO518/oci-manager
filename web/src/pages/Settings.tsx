import { useEffect, useState } from 'react'
import { Badge, Card, ErrorBanner, Field } from '../components/ui'
import { api } from '../lib/api'
import { useAuth } from '../lib/auth'
import type { TelegramSettings } from '../lib/types'

const EMPTY: TelegramSettings = {
  enabled: false,
  chat_id: '',
  has_token: false,
  login_otp: true,
  event_grab: true,
}

export default function Settings() {
  const { username } = useAuth()
  const [tg, setTg] = useState<TelegramSettings>(EMPTY)
  const [token, setToken] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  // 密码修改
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [savingPassword, setSavingPassword] = useState(false)

  useEffect(() => {
    let cancelled = false
    api.telegram
      .get()
      .then((cfg) => {
        if (!cancelled) setTg(cfg)
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  async function saveTelegram() {
    setSaving(true)
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
      setNotice('已保存')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  async function testTelegram() {
    setTesting(true)
    setError('')
    setNotice('')
    try {
      await api.telegram.test()
      setNotice('测试消息已发送，请检查 Telegram')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setTesting(false)
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

  if (loading) {
    return (
      <div className="py-10 text-center text-sm text-oracle-muted">加载中…</div>
    )
  }

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}
      {notice && (
        <div className="rounded border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700">
          {notice}
        </div>
      )}

      <Card title="Telegram 通知">
        <p className="mb-4 text-xs text-oracle-muted">
          配置后可接收登录提醒、登录验证码与 ARM 抢购结果。Bot Token 会以加密形式存库，接口不回传明文。
        </p>

        <div className="space-y-3">
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
            hint="给机器人发一条消息，然后访问 https://api.telegram.org/bot<TOKEN>/getUpdates 查看 chat.id。频道可用 @channelname。"
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

          <div className="flex items-center gap-2 pt-2">
            <button type="button" className="oci-btn-primary" disabled={saving} onClick={() => void saveTelegram()}>
              {saving ? '保存中…' : '保存'}
            </button>
            <button
              type="button"
              className="oci-btn-secondary"
              disabled={testing || !tg.enabled || tg.chat_id === ''}
              onClick={() => void testTelegram()}
            >
              {testing ? '发送中…' : '发送测试消息'}
            </button>
            <span className="text-xs text-oracle-muted">
              当前状态：
              {tg.enabled && tg.has_token && tg.chat_id ? (
                <Badge tone="border-green-200 bg-green-50 text-green-700">已配置</Badge>
              ) : (
                <Badge tone="border-gray-200 bg-gray-50 text-gray-600">未配置</Badge>
              )}
            </span>
          </div>
        </div>
      </Card>

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
    </div>
  )
}
