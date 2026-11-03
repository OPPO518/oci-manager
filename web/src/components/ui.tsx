import type { ReactNode } from 'react'

// 状态徽标：绿色代表运行中，灰色代表已停止，红色代表异常或已终止
const STATE_STYLES: Record<string, string> = {
  RUNNING: 'border-green-200 bg-green-50 text-green-700',
  STOPPED: 'border-gray-200 bg-gray-50 text-gray-600',
  STOPPING: 'border-amber-200 bg-amber-50 text-amber-700',
  STARTING: 'border-amber-200 bg-amber-50 text-amber-700',
  TERMINATED: 'border-red-200 bg-red-50 text-red-700',
  TERMINATING: 'border-red-200 bg-red-50 text-red-700',
  PROVISIONING: 'border-blue-200 bg-blue-50 text-blue-700',
}

export function Badge({ children, tone }: { children: ReactNode; tone?: string }) {
  const style =
    tone ??
    STATE_STYLES[typeof children === 'string' ? children : ''] ??
    'border-oracle-border bg-oracle-surface text-oracle-muted'
  return (
    <span className={`inline-flex items-center rounded border px-2 py-0.5 text-xs font-medium ${style}`}>
      {children}
    </span>
  )
}

const TASK_STATUS: Record<string, { label: string; tone: string }> = {
  idle: { label: '未启动', tone: 'border-gray-200 bg-gray-50 text-gray-600' },
  running: { label: '运行中', tone: 'border-blue-200 bg-blue-50 text-blue-700' },
  success: { label: '已成功', tone: 'border-green-200 bg-green-50 text-green-700' },
  failed: { label: '已失败', tone: 'border-red-200 bg-red-50 text-red-700' },
  stopped: { label: '已停止', tone: 'border-amber-200 bg-amber-50 text-amber-700' },
}

export function TaskStatusBadge({ status }: { status: string }) {
  const item = TASK_STATUS[status] ?? TASK_STATUS.idle
  return <Badge tone={item.tone}>{item.label}</Badge>
}

export function Card({
  title,
  extra,
  children,
  className = '',
}: {
  title?: ReactNode
  extra?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={`oci-card ${className}`}>
      {(title || extra) && (
        <header className="flex items-center justify-between border-b border-oracle-border px-4 py-3">
          <h2 className="text-sm font-semibold">{title}</h2>
          {extra}
        </header>
      )}
      <div className="p-4">{children}</div>
    </section>
  )
}

export function Loading({ text = '加载中…' }: { text?: string }) {
  return (
    <div className="flex items-center justify-center py-10 text-sm text-oracle-muted">
      <span className="mr-2 inline-block h-3 w-3 animate-spin rounded-full border-2 border-oracle-border border-t-oracle-red" />
      {text}
    </div>
  )
}

export function EmptyState({ text, action }: { text: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-3 py-12 text-center">
      <p className="text-sm text-oracle-muted">{text}</p>
      {action}
    </div>
  )
}

export function ErrorBanner({ message, onClose }: { message: string; onClose?: () => void }) {
  return (
    <div className="mb-4 flex items-start justify-between gap-4 rounded border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
      <span className="break-all">{message}</span>
      {onClose && (
        <button type="button" className="shrink-0 text-red-500 hover:text-red-700" onClick={onClose}>
          关闭
        </button>
      )}
    </div>
  )
}

export function Modal({
  title,
  onClose,
  children,
  width = 'max-w-lg',
}: {
  title: string
  onClose: () => void
  children: ReactNode
  width?: string
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className={`w-full ${width} rounded bg-white shadow-lg`}>
        <header className="flex items-center justify-between border-b border-oracle-border px-4 py-3">
          <h3 className="text-sm font-semibold">{title}</h3>
          <button type="button" className="text-oracle-muted hover:text-oracle-ink" onClick={onClose}>
            关闭
          </button>
        </header>
        <div className="p-4">{children}</div>
      </div>
    </div>
  )
}

export function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <label className="block">
      <span className="oci-label">{label}</span>
      {children}
      {hint && <span className="mt-1 block text-xs text-oracle-muted">{hint}</span>}
    </label>
  )
}

export function Stat({ label, value, unit }: { label: string; value: ReactNode; unit?: string }) {
  return (
    <div className="oci-card p-4">
      <p className="text-xs text-oracle-muted">{label}</p>
      <p className="mt-1 text-2xl font-semibold">
        {value}
        {unit && <span className="ml-1 text-sm font-normal text-oracle-muted">{unit}</span>}
      </p>
    </div>
  )
}
