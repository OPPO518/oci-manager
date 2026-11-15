import { useEffect, useMemo, useRef, useState } from 'react'
import { Badge, Card, Icon } from '../components/ui'
import { api } from '../lib/api'
import { formatDateTime } from '../lib/format'
import type { LogEntry, LogLevel, LogSource } from '../lib/types'

const LEVELS: { key: LogLevel; label: string }[] = [
  { key: 'info', label: '信息' },
  { key: 'success', label: '成功' },
  { key: 'warn', label: '警告' },
  { key: 'error', label: '错误' },
]

const SOURCES: { key: LogSource; label: string }[] = [
  { key: 'startup', label: '启动' },
  { key: 'scheduler', label: '定时任务' },
  { key: 'grabber', label: '抢购' },
  { key: 'audit', label: '审计' },
  { key: 'system', label: '系统' },
]

const LEVEL_TONE: Record<LogLevel, 'neutral' | 'success' | 'warning' | 'danger'> = {
  info: 'neutral',
  success: 'success',
  warn: 'warning',
  error: 'danger',
}

export default function Logs() {
  const [entries, setEntries] = useState<LogEntry[]>([])
  const [connected, setConnected] = useState(false)
  const [autoScroll, setAutoScroll] = useState(true)
  const [levelFilter, setLevelFilter] = useState<Set<LogLevel>>(new Set())
  const [sourceFilter, setSourceFilter] = useState<Set<LogSource>>(new Set())
  const [paused, setPaused] = useState(false)
  const [buffer, setBuffer] = useState<LogEntry[]>([])
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const es = new EventSource(api.logs.streamUrl())
    es.onopen = () => setConnected(true)
    es.onerror = () => setConnected(false)
    es.onmessage = (ev) => {
      if (!ev.data || ev.data.startsWith(':')) return
      try {
        const entry = JSON.parse(ev.data) as LogEntry
        if (paused) {
          setBuffer((b) => trim([...b, entry]))
        } else {
          setEntries((prev) => trim([...prev, entry]))
        }
      } catch {
        // 忽略无法解析的心跳/脏数据
      }
    }
    return () => es.close()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (autoScroll && listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight
    }
  }, [entries, autoScroll])

  const visible = useMemo(() => {
    return entries.filter((e) => {
      if (levelFilter.size > 0 && !levelFilter.has(e.level)) return false
      if (sourceFilter.size > 0 && !sourceFilter.has(e.source)) return false
      return true
    })
  }, [entries, levelFilter, sourceFilter])

  const toggleLevel = (k: LogLevel) =>
    setLevelFilter((s) => {
      const next = new Set(s)
      next.has(k) ? next.delete(k) : next.add(k)
      return next
    })

  const toggleSource = (k: LogSource) =>
    setSourceFilter((s) => {
      const next = new Set(s)
      next.has(k) ? next.delete(k) : next.add(k)
      return next
    })

  const resume = () => {
    setEntries((prev) => trim([...prev, ...buffer]))
    setBuffer([])
    setPaused(false)
  }

  return (
    <div>
      <Card
        title="实时日志流"
        extra={
          <span className={`oci-conn ${connected ? 'is-on' : 'is-off'}`}>
            <Icon name="success" size={13} />
            {connected ? '已连接' : '连接中…'}
          </span>
        }
      >
        <div className="oci-log-toolbar">
          <div className="flex flex-wrap items-center gap-2">
            <span className="oci-inline-hint">级别</span>
            {LEVELS.map((l) => (
              <button
                key={l.key}
                type="button"
                className={`oci-chip ${levelFilter.has(l.key) ? 'is-active' : ''}`}
                onClick={() => toggleLevel(l.key)}
              >
                {l.label}
              </button>
            ))}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="oci-inline-hint">来源</span>
            {SOURCES.map((s) => (
              <button
                key={s.key}
                type="button"
                className={`oci-chip ${sourceFilter.has(s.key) ? 'is-active' : ''}`}
                onClick={() => toggleSource(s.key)}
              >
                {s.label}
              </button>
            ))}
          </div>
          <div className="flex items-center gap-3">
            <button type="button" className="oci-btn-text" onClick={() => setEntries([])}>
              清空显示
            </button>
            <label className="oci-checkbox">
              <input type="checkbox" checked={autoScroll} onChange={(e) => setAutoScroll(e.target.checked)} />
              <span className="oci-checkbox-box">
                <Icon name="check" size={11} />
              </span>
              <span className="oci-checkbox-label">自动滚动</span>
            </label>
            {paused ? (
              <button type="button" className="oci-btn-secondary" onClick={resume}>
                恢复并载入 {buffer.length} 条
              </button>
            ) : (
              <button type="button" className="oci-btn-secondary" onClick={() => setPaused(true)}>
                暂停
              </button>
            )}
          </div>
        </div>

        <div className="oci-log-panel" ref={listRef}>
          {visible.length === 0 ? (
            <div className="oci-empty">
              <Icon name="logs" size={28} />
              <p>暂无日志。执行开关机、抢购或敏感操作时会实时出现在这里。</p>
            </div>
          ) : (
            visible.map((e) => (
              <div key={e.id} className={`oci-log-row level-${e.level}`}>
                <span className="oci-log-time">{formatDateTime(e.timestamp)}</span>
                <Badge tone={LEVEL_TONE[e.level]}>{e.level}</Badge>
                <span className="oci-log-source">{e.source}</span>
                {e.account && <span className="oci-log-account">{e.account}</span>}
                <span className="oci-log-msg">{e.message}</span>
              </div>
            ))
          )}
        </div>
      </Card>
    </div>
  )
}

function trim(list: LogEntry[]): LogEntry[] {
  const MAX = 1000
  if (list.length > MAX) return list.slice(list.length - MAX)
  return list
}
