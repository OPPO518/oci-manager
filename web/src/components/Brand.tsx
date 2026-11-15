import type { CSSProperties } from 'react'

/**
 * OCI Manager 自绘品牌标识。
 *
 * 用一组叠加的圆 + 圆角矩形拼出「云」的形状，再套 Oracle Redwood 风格的
 * 红→黄→青渐变方块底，作为我们自托管产品的 logo。刻意不使用 Oracle 的
 * 官方字标，避免第三方工具冒充商标；产品名统一为「OCI Manager」。
 */
export function BrandMark({ size = 40, style }: { size?: number; style?: CSSProperties }) {
  const gid = `ocim-g-${size}`
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 48 48"
      role="img"
      aria-label="OCI Manager"
      style={style}
    >
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#c74634" />
          <stop offset="0.5" stopColor="#f0cc72" />
          <stop offset="1" stopColor="#227e9e" />
        </linearGradient>
      </defs>
      <rect width="48" height="48" rx="12" fill={`url(#${gid})`} />
      <g fill="#ffffff">
        <circle cx="19" cy="25" r="7" />
        <circle cx="29" cy="21.5" r="9" />
        <circle cx="32" cy="28" r="6" />
        <rect x="13" y="27" width="24" height="9" rx="4.5" />
      </g>
    </svg>
  )
}
