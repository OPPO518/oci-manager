const CURRENCY_SYMBOLS: Record<string, string> = {
  USD: '$',
  CNY: '¥',
  EUR: '€',
  GBP: '£',
  JPY: '¥',
}

export function formatMoney(value: number, currency = ''): string {
  const symbol = CURRENCY_SYMBOLS[currency] ?? (currency ? `${currency} ` : '')
  const digits = value > 0 && value < 1 ? 4 : 2
  return `${symbol}${value.toFixed(digits)}`
}

export function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString('zh-CN', { hour12: false })
}

export function formatDate(value: string | null | undefined): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleDateString('zh-CN')
}

export function isoDate(date: Date): string {
  return date.toISOString().slice(0, 10)
}

export function daysAgo(days: number): string {
  const d = new Date()
  d.setDate(d.getDate() - days)
  return isoDate(d)
}

/** 截断 OCID 等长字符串，便于表格展示 */
export function truncate(value: string, head = 20, tail = 6): string {
  if (value.length <= head + tail + 3) return value
  return `${value.slice(0, head)}…${value.slice(-tail)}`
}
