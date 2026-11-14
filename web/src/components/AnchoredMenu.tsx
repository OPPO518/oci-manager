import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from 'react'
import { createPortal } from 'react-dom'

type Position = { left: number; top: number; maxHeight: number }

/**
 * 锚定式弹出菜单：渲染到 document.body，用 fixed 定位。
 *
 * 为什么不直接 position:absolute —— 页面里的 .oci-card(overflow:hidden) 与
 * .oci-table-wrap(overflow:auto) 都会创建裁剪上下文，absolute 菜单一旦越过容器
 * 边界就被切掉，z-index 再高也跨不出去。portal + fixed 是唯一稳妥的解法。
 */
export function AnchoredMenu({
  open,
  anchorRef,
  onClose,
  children,
  className = 'oci-actions-menu',
  align = 'end',
  minWidth = 180,
}: {
  open: boolean
  anchorRef: RefObject<HTMLElement | null>
  onClose: () => void
  children: ReactNode
  className?: string
  align?: 'start' | 'end'
  minWidth?: number
}) {
  const menuRef = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<Position | null>(null)

  const reposition = useCallback(() => {
    const anchor = anchorRef.current
    const menu = menuRef.current
    if (!anchor || !menu) return

    const r = anchor.getBoundingClientRect()
    const mw = menu.offsetWidth || minWidth
    const mh = menu.offsetHeight || 0
    const vw = window.innerWidth
    const vh = window.innerHeight
    const gap = 4
    const pad = 8

    let left = align === 'end' ? r.right - mw : r.left
    left = Math.max(pad, Math.min(left, vw - mw - pad))

    const spaceBelow = vh - r.bottom - gap - pad
    const spaceAbove = r.top - gap - pad
    // 下方空间不足且上方更宽裕时向上翻转
    const flip = mh > spaceBelow && spaceAbove > spaceBelow
    const available = Math.max(96, flip ? spaceAbove : spaceBelow)
    const height = Math.min(mh, available)
    const top = flip
      ? Math.max(pad, r.top - gap - height)
      : Math.min(r.bottom + gap, vh - height - pad)

    setPos({ left, top: Math.max(pad, top), maxHeight: height })
  }, [align, anchorRef, minWidth])

  useLayoutEffect(() => {
    if (!open) {
      setPos(null)
      return
    }
    reposition()

    window.addEventListener('scroll', reposition, true)
    window.addEventListener('resize', reposition)
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    const onDown = (e: MouseEvent) => {
      const target = e.target as Node
      if (anchorRef.current?.contains(target) || menuRef.current?.contains(target)) return
      onClose()
    }
    document.addEventListener('keydown', onKey)
    document.addEventListener('mousedown', onDown)

    let observer: ResizeObserver | null = null
    if (typeof ResizeObserver !== 'undefined' && menuRef.current) {
      observer = new ResizeObserver(reposition)
      observer.observe(menuRef.current)
    }

    return () => {
      window.removeEventListener('scroll', reposition, true)
      window.removeEventListener('resize', reposition)
      document.removeEventListener('keydown', onKey)
      document.removeEventListener('mousedown', onDown)
      observer?.disconnect()
    }
  }, [open, onClose, reposition])

  if (!open) return null

  return createPortal(
    <div
      ref={menuRef}
      className={className}
      role="menu"
      style={{
        position: 'fixed',
        left: pos?.left ?? -9999,
        top: pos?.top ?? -9999,
        maxHeight: pos?.maxHeight ?? 320,
        overflowY: 'auto',
        minWidth,
        visibility: pos ? 'visible' : 'hidden',
        zIndex: 1000,
      }}
    >
      {children}
    </div>,
    document.body,
  )
}
