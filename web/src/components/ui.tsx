import { useEffect, useRef, useState, type ReactNode } from 'react'

/* ------------------------------------------------------------------
   共用内联 SVG 图标（无第三方图标依赖）
   ------------------------------------------------------------------ */
export type IconName =
  | 'overview'
  | 'compute'
  | 'grabber'
  | 'capacity'
  | 'cost'
  | 'accounts'
  | 'settings'
  | 'menu'
  | 'search'
  | 'globe'
  | 'help'
  | 'bell'
  | 'chevron'
  | 'logout'
  | 'close'
  | 'more'
  | 'chevron-left'
  | 'chevron-right'
  | 'chevron-up'
  | 'chevron-down'
  | 'sort'
  | 'check'
  | 'warning'
  | 'error'
  | 'info'
  | 'success'
  | 'pin'
  | 'clock'
  | 'terminal'
  | 'announcement'
  | 'refresh'
  | 'download'
  | 'plus'
  | 'filter'
  | 'home'
  | 'external'
  | 'user'
  | 'shield'
  | 'database'
  | 'network'
  | 'storage'
  | 'eye'
  | 'sliders'

/** Redwood 线性图标，默认 16px。 */
export function Icon({ name, size = 16 }: { name: IconName; size?: number }) {
  const paths: Record<IconName, ReactNode> = {
    overview: (
      <>
        <rect x="2" y="2" width="5" height="5" rx="1" />
        <rect x="9" y="2" width="5" height="5" rx="1" />
        <rect x="2" y="9" width="5" height="5" rx="1" />
        <rect x="9" y="9" width="5" height="5" rx="1" />
      </>
    ),
    compute: (
      <>
        <rect x="2" y="3" width="12" height="4" rx="1" />
        <rect x="2" y="9" width="12" height="4" rx="1" />
        <circle cx="4.5" cy="5" r="0.7" fill="currentColor" stroke="none" />
        <circle cx="4.5" cy="11" r="0.7" fill="currentColor" stroke="none" />
      </>
    ),
    grabber: (
      <>
        <path d="M8 1.5v3M8 11.5v3M1.5 8h3M11.5 8h3" />
        <circle cx="8" cy="8" r="3" />
        <circle cx="8" cy="8" r="0.8" fill="currentColor" stroke="none" />
      </>
    ),
    capacity: (
      <>
        <path d="M2 12V7M6 12V4M10 12V9M14 12V2" />
        <path d="M1 14h14" />
      </>
    ),
    cost: (
      <>
        <circle cx="8" cy="8" r="6" />
        <path d="M10.5 5.5c-.6-.7-1.5-1-2.5-1-1.4 0-2.5.7-2.5 1.8 0 2.5 5 1.2 5 3.7 0 1.1-1.1 1.9-2.5 1.9-1 0-1.9-.3-2.5-1" />
        <path d="M8 3v10" />
      </>
    ),
    accounts: (
      <>
        <circle cx="8" cy="5.5" r="2.5" />
        <path d="M2.5 14c0-3 2.5-4.5 5.5-4.5s5.5 1.5 5.5 4.5" />
      </>
    ),
    settings: (
      <>
        <circle cx="8" cy="8" r="2.5" />
        <path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.4 1.4M11.2 11.2l1.4 1.4M12.6 3.4l-1.4 1.4M4.8 11.2l-1.4 1.4" />
      </>
    ),
    menu: <path d="M2 4h12M2 8h12M2 12h12" />,
    search: (
      <>
        <circle cx="7" cy="7" r="4.5" />
        <path d="M10.5 10.5L14 14" />
      </>
    ),
    globe: (
      <>
        <circle cx="8" cy="8" r="6" />
        <path d="M2 8h12M8 2c2 2 2 10 0 12M8 2c-2 2-2 10 0 12" />
      </>
    ),
    help: (
      <>
        <circle cx="8" cy="8" r="6" />
        <path d="M6.3 6.2a1.8 1.8 0 013.4.7c0 1.4-1.7 1.6-1.7 3" />
        <circle cx="8" cy="11.7" r="0.7" fill="currentColor" stroke="none" />
      </>
    ),
    bell: (
      <>
        <path d="M4 6a4 4 0 018 0c0 3 1 4 1 4H3s1-1 1-4" />
        <path d="M6.5 12.5a1.5 1.5 0 003 0" />
      </>
    ),
    chevron: <path d="M4 6l4 4 4-4" />,
    logout: (
      <>
        <path d="M6 2H3v12h3" />
        <path d="M9 5l3 3-3 3M12 8H6" />
      </>
    ),
    close: <path d="M4 4l8 8M12 4l-8 8" />,
    more: (
      <>
        <circle cx="3.2" cy="8" r="1.2" fill="currentColor" stroke="none" />
        <circle cx="8" cy="8" r="1.2" fill="currentColor" stroke="none" />
        <circle cx="12.8" cy="8" r="1.2" fill="currentColor" stroke="none" />
      </>
    ),
    'chevron-left': <path d="M10 4l-4 4 4 4" />,
    'chevron-right': <path d="M6 4l4 4-4 4" />,
    'chevron-up': <path d="M4 10l4-4 4 4" />,
    'chevron-down': <path d="M4 6l4 4 4-4" />,
    sort: <path d="M5 3v10M3 11l2 2 2-2M11 13V3M9 5l2-2 2 2" />,
    check: <path d="M3 8l3 3 7-7" />,
    warning: (
      <>
        <path d="M8 2l6 11H2L8 2z" />
        <path d="M8 6.5v3M8 11.5v.5" />
      </>
    ),
    error: (
      <>
        <circle cx="8" cy="8" r="6" />
        <path d="M8 5v4M8 11.5v.5" />
      </>
    ),
    info: (
      <>
        <circle cx="8" cy="8" r="6" />
        <path d="M8 7.5v4M8 5v.5" />
      </>
    ),
    success: (
      <>
        <circle cx="8" cy="8" r="6" />
        <path d="M5 8l2 2 4-4" />
      </>
    ),
    pin: <path d="M6 2h4M7 2v5l-2 3h6l-2-3V2M8 10v4" />,
    clock: (
      <>
        <circle cx="8" cy="8" r="6" />
        <path d="M8 4.5V8l2.5 1.5" />
      </>
    ),
    terminal: (
      <>
        <rect x="2" y="3" width="12" height="10" rx="1" />
        <path d="M4.5 6.5l2 1.5-2 1.5M8 10h3" />
      </>
    ),
    announcement: (
      <>
        <path d="M3 6.5h2.5L9 4v8L5.5 9.5H3V6.5z" />
        <path d="M11 6a3 3 0 010 4" />
      </>
    ),
    refresh: <path d="M13 8a5 5 0 11-1.5-3.5M13 3v3h-3" />,
    download: <path d="M8 2v8M5 7l3 3 3-3M3 13h10" />,
    plus: <path d="M8 3v10M3 8h10" />,
    filter: <path d="M2 4h12L9 9v4l-2 1V9L2 4z" />,
    home: (
      <>
        <path d="M2 7l6-5 6 5v7H2V7z" />
        <path d="M6 14V9h4v5" />
      </>
    ),
    external: <path d="M9 3h4v4M13 3L7 9M11 9v4H3V5h4" />,
    user: (
      <>
        <circle cx="8" cy="5.5" r="2.5" />
        <path d="M2.5 14c0-3 2.5-4.5 5.5-4.5s5.5 1.5 5.5 4.5" />
      </>
    ),
    shield: <path d="M8 1.5l5 2v4c0 3.5-2.5 5.5-5 6.5-2.5-1-5-3-5-6.5v-4l5-2z" />,
    database: (
      <>
        <ellipse cx="8" cy="4" rx="5" ry="2" />
        <path d="M3 4v8c0 1.1 2.2 2 5 2s5-.9 5-2V4M3 8c0 1.1 2.2 2 5 2s5-.9 5-2" />
      </>
    ),
    network: (
      <>
        <circle cx="8" cy="3" r="1.5" />
        <circle cx="3" cy="12" r="1.5" />
        <circle cx="13" cy="12" r="1.5" />
        <path d="M8 4.5L3 10.5M8 4.5l5 6M4.5 12h7" />
      </>
    ),
    storage: (
      <>
        <rect x="2" y="3" width="12" height="4" rx="1" />
        <rect x="2" y="9" width="12" height="4" rx="1" />
        <circle cx="4.5" cy="5" r="0.6" fill="currentColor" stroke="none" />
        <circle cx="4.5" cy="11" r="0.6" fill="currentColor" stroke="none" />
      </>
    ),
    eye: (
      <>
        <path d="M1.5 8s2.5-4.5 6.5-4.5S14.5 8 14.5 8 12 12.5 8 12.5 1.5 8 1.5 8z" />
        <circle cx="8" cy="8" r="2" />
      </>
    ),
    sliders: (
      <>
        <path d="M2 4h5M10 4h4M2 8h9M13 8h1M2 12h3M8 12h6" />
        <circle cx="8.5" cy="4" r="1.3" />
        <circle cx="12" cy="8" r="1.3" />
        <circle cx="6.5" cy="12" r="1.3" />
      </>
    ),
  }
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.3"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[name]}
    </svg>
  )
}

/* ------------------------------------------------------------------
   状态徽标：颜色统一收拢到 CSS 语义类 .oci-badge--*
   ------------------------------------------------------------------ */
export type BadgeTone = 'success' | 'warning' | 'danger' | 'info' | 'neutral'

const STATE_TONE: Record<string, BadgeTone> = {
  RUNNING: 'success',
  AVAILABLE: 'success',
  STOPPED: 'neutral',
  STOPPING: 'warning',
  STARTING: 'warning',
  PROVISIONING: 'warning',
  TERMINATED: 'danger',
  TERMINATING: 'danger',
}

export function Badge({ children, tone }: { children: ReactNode; tone?: BadgeTone }) {
  const resolved = tone ?? STATE_TONE[typeof children === 'string' ? children : ''] ?? 'neutral'
  return <span className={`oci-badge oci-badge--${resolved}`}>{children}</span>
}

const TASK_STATUS: Record<string, { label: string; tone: BadgeTone }> = {
  idle: { label: '未启动', tone: 'neutral' },
  running: { label: '运行中', tone: 'info' },
  success: { label: '已成功', tone: 'success' },
  failed: { label: '已失败', tone: 'danger' },
  stopped: { label: '已停止', tone: 'warning' },
}

export function TaskStatusBadge({ status }: { status: string }) {
  const item = TASK_STATUS[status] ?? TASK_STATUS.idle
  return <Badge tone={item.tone}>{item.label}</Badge>
}

/** 容量状态徽标：AVAILABLE / OUT_OF_HOST_CAPACITY / HARDWARE_NOT_SUPPORTED */
const CAPACITY_STATUS: Record<string, { label: string; tone: BadgeTone }> = {
  AVAILABLE: { label: '有库存', tone: 'success' },
  OUT_OF_HOST_CAPACITY: { label: '无库存', tone: 'danger' },
  HARDWARE_NOT_SUPPORTED: { label: '硬件未部署', tone: 'neutral' },
  UNKNOWN: { label: '状态未知', tone: 'neutral' },
}

export function CapacityBadge({ status }: { status: string }) {
  const item = CAPACITY_STATUS[status] ?? CAPACITY_STATUS.UNKNOWN
  return <Badge tone={item.tone}>{item.label}</Badge>
}

/* ------------------------------------------------------------------
   卡片
   ------------------------------------------------------------------ */
export function Card({
  title,
  extra,
  children,
  className = '',
  bodyClassName = '',
}: {
  title?: ReactNode
  extra?: ReactNode
  children: ReactNode
  className?: string
  bodyClassName?: string
}) {
  return (
    <section className={`oci-card ${className}`}>
      {(title || extra) && (
        <header className="oci-card-head">
          <h2>{title}</h2>
          {extra}
        </header>
      )}
      <div className={`oci-card-body ${bodyClassName}`}>{children}</div>
    </section>
  )
}

/** 部件右上角 "⋯"（展示性，不假装提供未实现的操作） */
export function CardDots() {
  const [open, setOpen] = useState(false)
  return (
    <div className="oci-actions">
      <button
        type="button"
        className="oci-btn-icon oci-widget-dots"
        aria-label="部件选项"
        title="部件选项"
        onClick={() => setOpen((v) => !v)}
      >
        <Icon name="more" />
      </button>
      {open && (
        <div className="oci-actions-menu" onMouseLeave={() => setOpen(false)}>
          <span className="oci-actions-note">部件选项（界面演示）</span>
        </div>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------
   加载 / 空态 / 提示条
   ------------------------------------------------------------------ */
export function Loading({ text = '加载中…' }: { text?: string }) {
  return (
    <div className="flex items-center justify-center py-10 text-sm text-oracle-muted">
      <span className="mr-2 inline-block h-4 w-4 animate-spin rounded-full border-2 border-oracle-border border-t-oracle-brand" />
      {text}
    </div>
  )
}

export function EmptyState({ text = 'No items found.', action }: { text?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-3 py-12 text-center">
      <p className="text-sm text-oracle-muted">{text}</p>
      {action}
    </div>
  )
}

export type NoticeTone = 'info' | 'success' | 'warning' | 'danger'

const NOTICE_CLASS: Record<NoticeTone, string> = {
  info: 'oci-notice-info',
  success: 'oci-notice-success',
  warning: 'oci-notice-warning',
  danger: 'oci-notice-error',
}

const NOTICE_ICON: Record<NoticeTone, IconName> = {
  info: 'info',
  success: 'success',
  warning: 'warning',
  danger: 'error',
}

export function Notice({
  tone = 'info',
  children,
  onClose,
  className = '',
}: {
  tone?: NoticeTone
  children: ReactNode
  onClose?: () => void
  className?: string
}) {
  return (
    <div className={`oci-notice ${NOTICE_CLASS[tone]} ${className}`}>
      <span className="oci-notice-icon">
        <Icon name={NOTICE_ICON[tone]} />
      </span>
      <div className="oci-notice-body">{children}</div>
      {onClose && (
        <button type="button" className="oci-notice-close" onClick={onClose}>
          关闭
        </button>
      )}
    </div>
  )
}

export function ErrorBanner({ message, onClose }: { message: string; onClose?: () => void }) {
  return (
    <Notice tone="danger" className="mb-4" onClose={onClose}>
      <span className="break-all">{message}</span>
    </Notice>
  )
}

export function SuccessBanner({ message, onClose }: { message: string; onClose?: () => void }) {
  return (
    <Notice tone="success" className="mb-4" onClose={onClose}>
      <span className="break-all">{message}</span>
    </Notice>
  )
}

/* ------------------------------------------------------------------
   Modal：× 图标按钮 + 右对齐 footer 动作区
   ------------------------------------------------------------------ */
export function Modal({
  title,
  onClose,
  children,
  footer,
  width = 'max-w-lg',
}: {
  title: string
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  width?: string
}) {
  return (
    <div className="oci-modal-overlay">
      <div className={`oci-modal ${width}`}>
        <header className="oci-modal-head">
          <h2>{title}</h2>
          <button
            type="button"
            className="oci-btn-icon"
            aria-label="关闭"
            title="关闭"
            onClick={onClose}
          >
            <Icon name="close" />
          </button>
        </header>
        <div className="oci-modal-body">{children}</div>
        {footer && <div className="oci-modal-footer">{footer}</div>}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------
   表单 / 统计
   ------------------------------------------------------------------ */
export function Field({
  label,
  hint,
  error,
  children,
}: {
  label: string
  hint?: string
  error?: string
  children: ReactNode
}) {
  return (
    <label className="block">
      <span className="oci-label">{label}</span>
      {children}
      {hint && <span className="oci-hint">{hint}</span>}
      {error && <span className="oci-error-text">{error}</span>}
    </label>
  )
}

export function Stat({
  label,
  value,
  unit,
  accent,
}: {
  label: string
  value: ReactNode
  unit?: string
  accent?: 'brand' | 'pine' | 'yellow'
}) {
  return (
    <div className={`oci-stat ${accent ? `oci-stat--${accent}` : ''}`}>
      <p className="oci-stat-label">{label}</p>
      <p className="oci-stat-value">
        {value}
        {unit && <span className="ml-1 text-sm font-normal text-oracle-muted">{unit}</span>}
      </p>
    </div>
  )
}

/* ------------------------------------------------------------------
   Checkbox / Switch（替换各页面的原生 <input type="checkbox">）
   ------------------------------------------------------------------ */
export function Checkbox({
  checked,
  onChange,
  label,
  disabled,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  label?: ReactNode
  disabled?: boolean
}) {
  return (
    <label className="oci-checkbox">
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="oci-checkbox-box">
        <Icon name="check" size={11} />
      </span>
      {label !== undefined && <span className="oci-checkbox-label">{label}</span>}
    </label>
  )
}

export function Switch({
  checked,
  onChange,
  label,
  disabled,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  label?: ReactNode
  disabled?: boolean
}) {
  return (
    <label className="oci-switch">
      <input
        type="checkbox"
        role="switch"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="oci-switch-track" />
      {label !== undefined && <span>{label}</span>}
    </label>
  )
}

/* ------------------------------------------------------------------
   分页：Showing 1–N of M + 每页条数 + 上/下页
   ------------------------------------------------------------------ */
export function Pager({
  total,
  page,
  pageSize,
  onPage,
  onPageSize,
}: {
  total: number
  page: number
  pageSize: number
  onPage: (page: number) => void
  onPageSize: (size: number) => void
}) {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1
  const to = Math.min(page * pageSize, total)
  return (
    <div className="oci-pager">
      <span className="oci-pager-info">
        Showing {from}–{to} of {total}
      </span>
      <div className="oci-pager-controls">
        <label>
          每页
          <select
            className="oci-select oci-pager-size"
            value={pageSize}
            onChange={(e) => onPageSize(Number(e.target.value))}
          >
            {[10, 20, 50, 100].map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
          条
        </label>
        <button
          type="button"
          className="oci-btn-icon"
          disabled={page <= 1}
          aria-label="上一页"
          title="上一页"
          onClick={() => onPage(page - 1)}
        >
          <Icon name="chevron-left" />
        </button>
        <span className="oci-pager-page">
          {page} / {pages}
        </span>
        <button
          type="button"
          className="oci-btn-icon"
          disabled={page >= pages}
          aria-label="下一页"
          title="下一页"
          onClick={() => onPage(page + 1)}
        >
          <Icon name="chevron-right" />
        </button>
      </div>
    </div>
  )
}

/** 返回当前页数据与可直接渲染的 <Pager/>，页面无需自行管理分页状态 */
export function usePager<T>(items: T[], defaultPageSize = 10) {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(defaultPageSize)
  const pages = Math.max(1, Math.ceil(items.length / pageSize))
  const safePage = Math.min(page, pages)
  const paged = items.slice((safePage - 1) * pageSize, safePage * pageSize)
  const pager = (
    <Pager
      total={items.length}
      page={safePage}
      pageSize={pageSize}
      onPage={setPage}
      onPageSize={(n) => {
        setPageSize(n)
        setPage(1)
      }}
    />
  )
  return { paged, pager }
}

/* ------------------------------------------------------------------
   行末 "⋯" 操作菜单（把原有多个行内按钮收纳进来，功能全部保留）
   ------------------------------------------------------------------ */
export type ActionItem = {
  label: ReactNode
  onSelect: () => void
  disabled?: boolean
  danger?: boolean
  separatorBefore?: boolean
}

export function ActionsMenu({ items, label = '操作' }: { items: ActionItem[]; label?: string }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [open])

  return (
    <div className="oci-actions" ref={ref}>
      <button
        type="button"
        className="oci-btn-icon"
        aria-label={label}
        title={label}
        onClick={() => setOpen((v) => !v)}
      >
        <Icon name="more" />
      </button>
      {open && (
        <div className="oci-actions-menu">
          {items.map((it, i) => (
            <div key={i}>
              {it.separatorBefore && <div className="oci-actions-sep" />}
              <button
                type="button"
                className={`oci-actions-item ${it.danger ? 'is-danger' : ''}`}
                disabled={it.disabled}
                onClick={() => {
                  setOpen(false)
                  it.onSelect()
                }}
              >
                {it.label}
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------
   可排序表头
   ------------------------------------------------------------------ */
export function SortableTh({
  label,
  sortKey,
  activeKey,
  dir,
  onSort,
  className = '',
}: {
  label: string
  sortKey: string
  activeKey: string
  dir: 'asc' | 'desc'
  onSort: (key: string) => void
  className?: string
}) {
  const active = activeKey === sortKey
  return (
    <th className={`oci-th-sort ${className}`}>
      <button type="button" onClick={() => onSort(sortKey)}>
        {label}
        {active && (
          <span className="oci-sort-icon">
            <Icon name={dir === 'asc' ? 'chevron-up' : 'chevron-down'} size={14} />
          </span>
        )}
      </button>
    </th>
  )
}
