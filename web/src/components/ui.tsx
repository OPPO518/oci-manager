import type { ReactNode } from 'react'

// 状态徽标：绿色代表运行中，灰色代表已停止，红色代表异常或已终止
const STATE_STYLES: Record<string, string> = {
  RUNNING: 'border-[#bbe3cb] bg-[#f0faf4] text-[#136437]',
  STOPPED: 'border-[#d9d6d2] bg-[#f5f4f2] text-[#5c5854]',
  STOPPING: 'border-[#f0d9b0] bg-[#fdf7ec] text-[#8a5200]',
  STARTING: 'border-[#f0d9b0] bg-[#fdf7ec] text-[#8a5200]',
  TERMINATED: 'border-[#f0c4bd] bg-[#fdf3f1] text-[#8e2d1e]',
  TERMINATING: 'border-[#f0c4bd] bg-[#fdf3f1] text-[#8e2d1e]',
  PROVISIONING: 'border-[#c9dced] bg-[#f2f8fd] text-[#155a8c]',
}

export function Badge({ children, tone }: { children: ReactNode; tone?: string }) {
  const style =
    tone ??
    STATE_STYLES[typeof children === 'string' ? children : ''] ??
    'border-oracle-border bg-oracle-surface text-oracle-muted'
  return <span className={`oci-badge ${style}`}>{children}</span>
}

const TASK_STATUS: Record<string, { label: string; tone: string }> = {
  idle: { label: '未启动', tone: 'border-[#d9d6d2] bg-[#f5f4f2] text-[#5c5854]' },
  running: { label: '运行中', tone: 'border-[#c9dced] bg-[#f2f8fd] text-[#155a8c]' },
  success: { label: '已成功', tone: 'border-[#bbe3cb] bg-[#f0faf4] text-[#136437]' },
  failed: { label: '已失败', tone: 'border-[#f0c4bd] bg-[#fdf3f1] text-[#8e2d1e]' },
  stopped: { label: '已停止', tone: 'border-[#f0d9b0] bg-[#fdf7ec] text-[#8a5200]' },
}

export function TaskStatusBadge({ status }: { status: string }) {
  const item = TASK_STATUS[status] ?? TASK_STATUS.idle
  return <Badge tone={item.tone}>{item.label}</Badge>
}

/** 容量状态徽标：AVAILABLE / OUT_OF_HOST_CAPACITY / HARDWARE_NOT_SUPPORTED */
const CAPACITY_STATUS: Record<string, { label: string; tone: string }> = {
  AVAILABLE: { label: '有库存', tone: 'border-[#bbe3cb] bg-[#f0faf4] text-[#136437]' },
  OUT_OF_HOST_CAPACITY: {
    label: '无库存',
    tone: 'border-[#f0c4bd] bg-[#fdf3f1] text-[#8e2d1e]',
  },
  HARDWARE_NOT_SUPPORTED: {
    label: '硬件未部署',
    tone: 'border-[#d9d6d2] bg-[#f5f4f2] text-[#5c5854]',
  },
  UNKNOWN: { label: '状态未知', tone: 'border-[#d9d6d2] bg-[#f5f4f2] text-[#5c5854]' },
}

export function CapacityBadge({ status }: { status: string }) {
  const item = CAPACITY_STATUS[status] ?? CAPACITY_STATUS.UNKNOWN
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
        <header className="oci-card-head">
          <h2>{title}</h2>
          {extra}
        </header>
      )}
      <div className="oci-card-body">{children}</div>
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
    <div className="oci-notice oci-notice-error mb-4 flex items-start justify-between gap-4">
      <span className="break-all">{message}</span>
      {onClose && (
        <button type="button" className="shrink-0 opacity-70 hover:opacity-100" onClick={onClose}>
          关闭
        </button>
      )}
    </div>
  )
}

export function SuccessBanner({ message, onClose }: { message: string; onClose?: () => void }) {
  return (
    <div className="oci-notice oci-notice-success mb-4 flex items-start justify-between gap-4">
      <span className="break-all">{message}</span>
      {onClose && (
        <button type="button" className="shrink-0 opacity-70 hover:opacity-100" onClick={onClose}>
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
      <div className={`w-full ${width} oci-card !overflow-visible shadow-xl`}>
        <header className="oci-card-head">
          <h2>{title}</h2>
          <button type="button" className="text-oracle-muted hover:text-oracle-ink" onClick={onClose}>
            关闭
          </button>
        </header>
        <div className="oci-card-body">{children}</div>
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
      {hint && <span className="oci-hint">{hint}</span>}
    </label>
  )
}

export function Stat({ label, value, unit }: { label: string; value: ReactNode; unit?: string }) {
  return (
    <div className="oci-stat">
      <p className="oci-stat-label">{label}</p>
      <p className="oci-stat-value">
        {value}
        {unit && <span className="ml-1 text-sm font-normal text-oracle-muted">{unit}</span>}
      </p>
    </div>
  )
}
