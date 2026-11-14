import { useEffect, useState, type ReactNode } from 'react'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import type { OtpPayload } from '../lib/types'
import { ErrorBanner, Field, Loading, Modal, Notice } from './ui'

export interface OtpConfirmProps {
  /** 弹窗标题 */
  title?: string
  /** 传给 /otp/request 的操作标识 */
  operation: string
  /** 传给 /otp/request 的操作摘要，同时展示给用户 */
  summary: string
  /** 关闭（取消 / 成功）回调 */
  onClose: () => void
  /** 调用方在此执行写操作并自行刷新数据；OTP 由本组件统一提供 */
  onConfirm: (otp: OtpPayload) => Promise<void>
  confirmLabel?: string
  /** 额外说明（如换 IP 的风险提示） */
  children?: ReactNode
}

/**
 * 统一封装业务写操作的二次验证：
 * 1. 挂载即调用 /otp/request 获取 challenge；
 * 2. 演示账号下后端返回 demo_code，自动填入并提示，不查收 Telegram；
 * 3. 用户输入验证码后，把 { challenge, code } 交给调用方；
 * 4. loading / 错误（ApiError.message）/ 成功后刷新全部由调用方与此处协作完成；
 * 5. 关闭或卸载时显式清空 challenge / code 等敏感 state。
 */
export function OtpConfirm({
  title = '验证并确认',
  operation,
  summary,
  onClose,
  onConfirm,
  confirmLabel = '确认',
  children,
}: OtpConfirmProps) {
  const { account } = useAccount()
  // 后端据此决定挑战类型：演示账号用固定码且不发 Telegram；
  // 真实账号的操作会校验挑战前缀，演示固定码无法蒙混过关。
  const demo = account?.is_demo ?? false
  const [challenge, setChallenge] = useState('')
  const [demoCode, setDemoCode] = useState('')
  const [code, setCode] = useState('')
  const [requesting, setRequesting] = useState(true)
  const [requestError, setRequestError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState('')

  useEffect(() => {
    let cancelled = false
    api.otp
      .request(operation, summary, demo)
      .then((res) => {
        if (cancelled) return
        setChallenge(res.challenge)
        if (res.demo_code) {
          setDemoCode(res.demo_code)
          setCode(res.demo_code)
        }
      })
      .catch((err: Error) => {
        if (!cancelled) setRequestError(err.message)
      })
      .finally(() => {
        if (!cancelled) setRequesting(false)
      })
    return () => {
      cancelled = true
      // 组件卸载时显式清空验证码等敏感 state
      setChallenge('')
      setDemoCode('')
      setCode('')
    }
  }, [operation, summary, demo])

  function clearSensitive() {
    setChallenge('')
    setDemoCode('')
    setCode('')
    setSubmitError('')
  }

  function handleClose() {
    clearSensitive()
    onClose()
  }

  async function submit() {
    if (!challenge || !code.trim()) return
    setSubmitting(true)
    setSubmitError('')
    try {
      await onConfirm({ challenge, code: code.trim() })
      clearSensitive()
      onClose()
    } catch (err) {
      setSubmitError((err as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={title}
      onClose={handleClose}
      footer={
        <>
          <button type="button" className="oci-btn-secondary" onClick={handleClose}>
            取消
          </button>
          <button
            type="button"
            className="oci-btn-primary"
            disabled={requesting || submitting || !challenge || !code.trim()}
            onClick={() => void submit()}
          >
            {submitting ? '提交中…' : confirmLabel}
          </button>
        </>
      }
    >
      <div className="space-y-4">
        {children}
        {requesting && <Loading text="正在申请验证码…" />}
        {requestError && <ErrorBanner message={requestError} onClose={() => setRequestError('')} />}
        {submitError && <ErrorBanner message={submitError} onClose={() => setSubmitError('')} />}
        {!requesting && !requestError && (
          <>
            {demoCode ? (
              <Notice tone="info">
                当前为演示账号，验证码 <strong className="font-mono">{demoCode}</strong> 已自动填入，
                无需查收 Telegram，也不会真的发送验证码。
              </Notice>
            ) : (
              <Notice tone="info">
                验证码已推送到 Telegram，请在 5 分钟内输入；真实账号需先在「设置 → Telegram 通知」启用 Telegram。
              </Notice>
            )}
            <Field label="验证码" hint={`操作：${summary}`}>
              <input
                className="oci-input text-center font-mono text-lg tracking-[0.5em]"
                value={code}
                onChange={(e) => setCode(e.target.value)}
                placeholder="000000"
                maxLength={6}
                inputMode="numeric"
                autoComplete="one-time-code"
                autoFocus
              />
            </Field>
          </>
        )}
      </div>
    </Modal>
  )
}
