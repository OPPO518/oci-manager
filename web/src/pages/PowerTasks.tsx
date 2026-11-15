import { useEffect, useMemo, useState } from 'react'
import { Badge, Card, ErrorBanner, Icon, Loading, Modal, Notice, Switch } from '../components/ui'
import { usePageAction } from '../components/Layout'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { formatDateTime } from '../lib/format'
import type { Instance, PowerAction, PowerTargetMode, PowerTask, PowerTaskInput } from '../lib/types'

const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

const ACTION_LABEL: Record<PowerAction, string> = {
  start: '启动',
  stop: '停止',
  reset: '重启',
}

function cronText(t: PowerTask): string {
  const dow = t.days_of_week === '*' || t.days_of_week === '' ? '每天' : t.days_of_week.split(',').map((d) => WEEKDAYS[Number(d)]).join('、')
  return `${dow} ${String(t.hour).padStart(2, '0')}:${String(t.minute).padStart(2, '0')}`
}

function emptyForm(): FormState {
  return {
    display_name: '',
    action: 'start',
    target_mode: 'all',
    instance_ids: [],
    days_of_week: [],
    hour: 9,
    minute: 0,
    enabled: true,
  }
}

type FormState = {
  display_name: string
  action: PowerAction
  target_mode: PowerTargetMode
  instance_ids: string[]
  days_of_week: number[]
  hour: number
  minute: number
  enabled: boolean
}

export default function PowerTasks() {
  const { account, accounts } = useAccount()
  const [tasks, setTasks] = useState<PowerTask[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<PowerTask | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm())
  const [saving, setSaving] = useState(false)
  const [instances, setInstances] = useState<Instance[]>([])

  const reload = () => {
    if (!account) return
    setLoading(true)
    setError('')
    api.powerTasks
      .list(account.id)
      .then(setTasks)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(reload, [account?.id])

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm())
    setModalOpen(true)
  }

  const openEdit = (t: PowerTask) => {
    setEditing(t)
    setForm({
      display_name: t.display_name,
      action: t.action,
      target_mode: t.target_mode,
      instance_ids: t.instance_ids ? t.instance_ids.split(',').filter(Boolean) : [],
      days_of_week: t.days_of_week === '*' || t.days_of_week === '' ? [] : t.days_of_week.split(',').map(Number),
      hour: t.hour,
      minute: t.minute,
      enabled: t.enabled,
    })
    setModalOpen(true)
  }

  useEffect(() => {
    if (modalOpen && account && (form.target_mode === 'selected' || editing)) {
      api.instances
        .list(account.id)
        .then(setInstances)
        .catch(() => setInstances([]))
    }
  }, [modalOpen, account?.id, form.target_mode, editing])

  const toggleEnabled = (t: PowerTask, enabled: boolean) => {
    api.powerTasks
      .toggle(String(t.id), enabled)
      .then((updated) => setTasks((prev) => prev.map((x) => (x.id === updated.id ? updated : x))))
      .catch((e: Error) => setError(e.message))
  }

  const remove = (t: PowerTask) => {
    if (!confirm(`确认删除定时任务「${t.display_name}」？`)) return
    api.powerTasks
      .remove(String(t.id))
      .then(() => setTasks((prev) => prev.filter((x) => x.id !== t.id)))
      .catch((e: Error) => setError(e.message))
  }

  const save = () => {
    if (!account) return
    if (!form.display_name.trim()) {
      setError('请填写任务名称')
      return
    }
    setSaving(true)
    setError('')
    const body: PowerTaskInput = {
      account_id: account.id,
      display_name: form.display_name.trim(),
      action: form.action,
      target_mode: form.target_mode,
      instance_ids: form.target_mode === 'selected' ? form.instance_ids : [],
      days_of_week: form.days_of_week,
      hour: form.hour,
      minute: form.minute,
      enabled: form.enabled,
    }
    const req = editing
      ? api.powerTasks.update(String(editing.id), body)
      : api.powerTasks.create(body)
    req
      .then(() => {
        setModalOpen(false)
        reload()
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setSaving(false))
  }

  usePageAction(
    <button type="button" className="oci-btn-primary" onClick={openCreate}>
      <Icon name="plus" size={15} />
      新建任务
    </button>,
  )

  const toggleDay = (d: number) => {
    setForm((f) => {
      const has = f.days_of_week.includes(d)
      return {
        ...f,
        days_of_week: has ? f.days_of_week.filter((x) => x !== d) : [...f.days_of_week, d].sort((a, b) => a - b),
      }
    })
  }

  const toggleInstance = (id: string) => {
    setForm((f) => {
      const has = f.instance_ids.includes(id)
      return { ...f, instance_ids: has ? f.instance_ids.filter((x) => x !== id) : [...f.instance_ids, id] }
    })
  }

  if (!account) {
    return (
      <Notice tone="info">请先在右上角选择一个 OCI 账号，再配置定时开关机任务。</Notice>
    )
  }

  return (
    <div>
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}

      {loading ? (
        <Loading text="正在加载定时任务…" />
      ) : (
        <Card
          title="定时开关机任务"
          extra={
            <span className="oci-inline-hint">
              按服务器本地时间执行 · {tasks.length} 条任务
            </span>
          }
        >
          {tasks.length === 0 ? (
            <div className="oci-empty">
              <Icon name="power" size={28} />
              <p>还没有定时任务。点击右上角「新建任务」按计划自动启停实例。</p>
            </div>
          ) : (
            <div className="oci-table-wrap">
              <table className="oci-table">
                <thead>
                  <tr>
                    <th>名称</th>
                    <th>动作</th>
                    <th>目标</th>
                    <th>计划</th>
                    <th>启用</th>
                    <th>上次运行</th>
                    <th>结果</th>
                    <th className="text-right">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {tasks.map((t) => (
                    <tr key={t.id}>
                      <td className="font-medium">{t.display_name}</td>
                      <td>
                        <Badge tone={t.action === 'stop' ? 'danger' : t.action === 'reset' ? 'warning' : 'success'}>
                          {ACTION_LABEL[t.action]}
                        </Badge>
                      </td>
                      <td className="text-rw-muted">
                        {t.target_mode === 'all' ? '全部实例' : `${t.instance_ids.split(',').filter(Boolean).length} 台指定`}
                      </td>
                      <td className="font-mono text-sm">{cronText(t)}</td>
                      <td>
                        <Switch checked={t.enabled} onChange={(v) => toggleEnabled(t, v)} />
                      </td>
                      <td className="text-sm text-rw-muted">{t.last_run ? formatDateTime(t.last_run) : '从未'}</td>
                      <td className="text-sm text-rw-muted max-w-[220px] truncate" title={t.last_result}>
                        {t.last_result || '—'}
                      </td>
                      <td className="text-right whitespace-nowrap">
                        <button type="button" className="oci-btn-text" onClick={() => openEdit(t)}>
                          编辑
                        </button>
                        <button type="button" className="oci-link-danger ml-2" onClick={() => remove(t)}>
                          删除
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {modalOpen && (
        <Modal
          title={editing ? '编辑定时任务' : '新建定时任务'}
          onClose={() => setModalOpen(false)}
          footer={
            <>
              <button type="button" className="oci-btn-secondary" onClick={() => setModalOpen(false)}>
                取消
              </button>
              <button type="button" className="oci-btn-primary" onClick={save} disabled={saving}>
                {saving ? '保存中…' : '保存'}
              </button>
            </>
          }
        >
          <div className="space-y-4">
            <label className="block">
              <span className="oci-label">任务名称</span>
              <input
                className="oci-input"
                value={form.display_name}
                placeholder="例如：工作日早九点启动"
                onChange={(e) => setForm((f) => ({ ...f, display_name: e.target.value }))}
              />
            </label>

            <div className="grid grid-cols-2 gap-4">
              <label className="block">
                <span className="oci-label">动作</span>
                <select
                  className="oci-input"
                  value={form.action}
                  onChange={(e) => setForm((f) => ({ ...f, action: e.target.value as PowerAction }))}
                >
                  <option value="start">启动</option>
                  <option value="stop">停止</option>
                  <option value="reset">重启</option>
                </select>
              </label>
              <label className="block">
                <span className="oci-label">目标范围</span>
                <select
                  className="oci-input"
                  value={form.target_mode}
                  onChange={(e) => setForm((f) => ({ ...f, target_mode: e.target.value as PowerTargetMode }))}
                >
                  <option value="all">全部实例</option>
                  <option value="selected">指定实例</option>
                </select>
              </label>
            </div>

            {form.target_mode === 'selected' && (
              <div>
                <span className="oci-label">选择实例</span>
                <div className="oci-checkbox-group max-h-44 overflow-auto rounded border border-oracle-border p-2">
                  {instances.length === 0 ? (
                    <p className="oci-inline-hint">该账号暂无实例。</p>
                  ) : (
                    instances.map((inst) => (
                      <label key={inst.id} className="oci-checkbox mb-1">
                        <input
                          type="checkbox"
                          checked={form.instance_ids.includes(inst.id)}
                          onChange={() => toggleInstance(inst.id)}
                        />
                        <span className="oci-checkbox-box">
                          <Icon name="check" size={11} />
                        </span>
                        <span className="oci-checkbox-label">
                          {inst.display_name} · <span className="font-mono text-xs">{inst.state}</span>
                        </span>
                      </label>
                    ))
                  )}
                </div>
              </div>
            )}

            <div>
              <span className="oci-label">执行时间</span>
              <div className="flex flex-wrap items-center gap-2">
                {WEEKDAYS.map((label, d) => {
                  const active = form.days_of_week.length === 0 || form.days_of_week.includes(d)
                  return (
                    <button
                      key={d}
                      type="button"
                      className={`oci-chip ${active ? 'is-active' : ''}`}
                      onClick={() => toggleDay(d)}
                    >
                      {label}
                    </button>
                  )
                })}
                <span className="oci-inline-hint">（全部不选 = 每天）</span>
              </div>
              <div className="mt-2 flex items-center gap-2">
                <input
                  type="number"
                  min={0}
                  max={23}
                  className="oci-input w-20"
                  value={form.hour}
                  onChange={(e) => setForm((f) => ({ ...f, hour: clampInt(e.target.value, 0, 23) }))}
                />
                <span className="text-rw-muted">时</span>
                <input
                  type="number"
                  min={0}
                  max={59}
                  className="oci-input w-20"
                  value={form.minute}
                  onChange={(e) => setForm((f) => ({ ...f, minute: clampInt(e.target.value, 0, 59) }))}
                />
                <span className="text-rw-muted">分</span>
              </div>
            </div>

            <div className="flex items-center justify-between rounded border border-oracle-border px-3 py-2">
              <span className="text-sm">启用此任务</span>
              <Switch checked={form.enabled} onChange={(v) => setForm((f) => ({ ...f, enabled: v }))} />
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}

function clampInt(v: string, min: number, max: number): number {
  const n = Math.floor(Number(v))
  if (Number.isNaN(n)) return min
  return Math.max(min, Math.min(max, n))
}
