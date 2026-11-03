import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Card,
  EmptyState,
  ErrorBanner,
  Field,
  Loading,
  Modal,
  TaskStatusBadge,
} from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { formatDateTime } from '../lib/format'
import type { Compartment, GrabTask, GrabTaskInput, GrabTaskLog, Image, Subnet } from '../lib/types'

const DEFAULT_FORM: GrabTaskInput = {
  account_id: '',
  display_name: 'arm-instance',
  region: '',
  compartment_id: '',
  ad_name: '',
  shape: 'VM.Standard.A1.Flex',
  ocpus: 4,
  memory_gbs: 24,
  image_id: '',
  subnet_id: '',
  ssh_public_key: '',
  boot_volume_gbs: 50,
  instance_count: 1,
  interval_sec: 60,
  max_attempts: 0,
}

export default function Grabber() {
  const { account, region } = useAccount()
  const effectiveRegion = region || undefined

  const [tasks, setTasks] = useState<GrabTask[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [form, setForm] = useState<GrabTaskInput>(DEFAULT_FORM)
  const [submitting, setSubmitting] = useState(false)

  const [compartments, setCompartments] = useState<Compartment[]>([])
  const [ads, setAds] = useState<string[]>([])
  const [subnets, setSubnets] = useState<Subnet[]>([])
  const [images, setImages] = useState<Image[]>([])
  const [imageFilter, setImageFilter] = useState('')

  const [logTask, setLogTask] = useState<GrabTask | null>(null)
  const [logs, setLogs] = useState<GrabTaskLog[]>([])
  const lastLogId = useRef(0)

  const loadTasks = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const list = await api.grabTasks.list(account?.id)
      setTasks(list)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }, [account])

  useEffect(() => {
    void loadTasks()
  }, [loadTasks])

  // 任务运行中时自动刷新列表，让状态与尝试次数保持同步
  const hasRunning = tasks.some((t) => t.status === 'running')
  useEffect(() => {
    if (!hasRunning) return
    const timer = window.setInterval(() => void loadTasks(), 5000)
    return () => window.clearInterval(timer)
  }, [hasRunning, loadTasks])

  // 日志面板轮询，按 ID 增量拉取
  useEffect(() => {
    if (!logTask) return
    lastLogId.current = 0
    setLogs([])
    const fetchLogs = async () => {
      try {
        const batch = await api.grabTasks.logs(logTask.id, lastLogId.current)
        if (batch.length) {
          lastLogId.current = batch[batch.length - 1].id
          setLogs((prev) => [...prev, ...batch].slice(-300))
        }
      } catch {
        // 轮询失败忽略，下个周期继续
      }
    }
    void fetchLogs()
    const timer = window.setInterval(() => void fetchLogs(), 3000)
    return () => window.clearInterval(timer)
  }, [logTask])

  async function loadFormOptions() {
    if (!account) return
    const compartmentId = form.compartment_id || account.tenancy_ocid
    const [compList, adList, subnetList, imageList] = await Promise.all([
      api.resources.compartments(account.id, form.region || undefined),
      api.resources.availabilityDomains(account.id, {
        region: form.region || undefined,
        compartment_id: compartmentId,
      }),
      api.resources.subnets(account.id, {
        region: form.region || undefined,
        compartment_id: compartmentId,
      }),
      api.resources.images(account.id, {
        region: form.region || undefined,
        compartment_id: compartmentId,
      }),
    ])
    setCompartments(compList)
    setAds(adList)
    setSubnets(subnetList)
    setImages(imageList)
  }

  function openForm() {
    if (!account) return
    setForm({
      ...DEFAULT_FORM,
      account_id: account.id,
      region: effectiveRegion ?? account.region,
      compartment_id: account.tenancy_ocid,
    })
    setFormOpen(true)
    void loadFormOptions().catch((err: Error) => setError(err.message))
  }

  async function submitForm() {
    setSubmitting(true)
    setError('')
    try {
      await api.grabTasks.create(form)
      setFormOpen(false)
      await loadTasks()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  async function toggleTask(task: GrabTask) {
    setError('')
    try {
      if (task.status === 'running') {
        await api.grabTasks.stop(task.id)
      } else {
        await api.grabTasks.start(task.id)
      }
      await loadTasks()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function removeTask(task: GrabTask) {
    setError('')
    try {
      await api.grabTasks.remove(task.id)
      if (logTask?.id === task.id) setLogTask(null)
      await loadTasks()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  const filteredImages = images
    .filter((img) => {
      const keyword = imageFilter.trim().toLowerCase()
      if (!keyword) return true
      return (
        img.display_name.toLowerCase().includes(keyword) ||
        img.operating_system.toLowerCase().includes(keyword)
      )
    })
    .slice(0, 100)

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}

      <Card
        title="ARM 抢购任务"
        extra={
          <button type="button" className="oci-btn-primary" onClick={openForm}>
            新建任务
          </button>
        }
      >
        <p className="mb-3 text-xs text-oracle-muted">
          Always Free 的 Ampere A1 资源常因超售返回「Out of host capacity」。任务会在遇到该错误时按设定间隔持续重试，
          遇到其他错误（配额、权限、参数）则立即停止。
        </p>

        {loading && tasks.length === 0 ? (
          <Loading />
        ) : tasks.length === 0 ? (
          <EmptyState text="还没有抢购任务" />
        ) : (
          <div className="overflow-x-auto">
            <table className="oci-table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>状态</th>
                  <th>规格</th>
                  <th>可用域</th>
                  <th>尝试次数</th>
                  <th>最近尝试</th>
                  <th className="text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {tasks.map((task) => (
                  <tr key={task.id}>
                    <td>
                      <div className="font-medium">{task.display_name}</div>
                      {task.message && (
                        <div className="text-xs text-oracle-muted">{task.message}</div>
                      )}
                    </td>
                    <td>
                      <TaskStatusBadge status={task.status} />
                    </td>
                    <td className="text-xs">
                      {task.shape}
                      <br />
                      {task.ocpus} OCPU / {task.memory_gbs} GB × {task.instance_count}
                    </td>
                    <td className="text-xs text-oracle-muted">{task.ad_name}</td>
                    <td>{task.attempts}</td>
                    <td className="text-xs text-oracle-muted">{formatDateTime(task.last_attempt_at)}</td>
                    <td>
                      <div className="flex justify-end gap-1">
                        <button
                          type="button"
                          className="oci-btn-secondary"
                          disabled={task.status === 'success'}
                          onClick={() => void toggleTask(task)}
                        >
                          {task.status === 'running' ? '停止' : '启动'}
                        </button>
                        <button
                          type="button"
                          className="oci-btn-secondary"
                          onClick={() => setLogTask(task)}
                        >
                          日志
                        </button>
                        <button type="button" className="oci-btn-danger" onClick={() => void removeTask(task)}>
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {formOpen && (
        <Modal title="新建抢购任务" onClose={() => setFormOpen(false)} width="max-w-2xl">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label="任务名称">
              <input
                className="oci-input"
                value={form.display_name}
                onChange={(e) => setForm((f) => ({ ...f, display_name: e.target.value }))}
              />
            </Field>
            <Field label="区域">
              <input
                className="oci-input"
                value={form.region}
                onChange={(e) => setForm((f) => ({ ...f, region: e.target.value }))}
              />
            </Field>
            <Field label="区间">
              <select
                className="oci-input"
                value={form.compartment_id}
                onChange={(e) => {
                  setForm((f) => ({ ...f, compartment_id: e.target.value }))
                }}
              >
                {compartments.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="可用域">
              <select
                className="oci-input"
                value={form.ad_name}
                onChange={(e) => setForm((f) => ({ ...f, ad_name: e.target.value }))}
              >
                <option value="">请选择</option>
                {ads.map((ad) => (
                  <option key={ad} value={ad}>
                    {ad}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="规格">
              <input
                className="oci-input"
                value={form.shape}
                onChange={(e) => setForm((f) => ({ ...f, shape: e.target.value }))}
              />
            </Field>
            <Field label="子网">
              <select
                className="oci-input"
                value={form.subnet_id}
                onChange={(e) => setForm((f) => ({ ...f, subnet_id: e.target.value }))}
              >
                <option value="">请选择</option>
                {subnets.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.display_name}（{s.cidr_block}）
                  </option>
                ))}
              </select>
            </Field>
            <Field label="OCPU">
              <input
                type="number"
                min={1}
                className="oci-input"
                value={form.ocpus}
                onChange={(e) => setForm((f) => ({ ...f, ocpus: Number(e.target.value) }))}
              />
            </Field>
            <Field label="内存（GB）">
              <input
                type="number"
                min={1}
                className="oci-input"
                value={form.memory_gbs}
                onChange={(e) => setForm((f) => ({ ...f, memory_gbs: Number(e.target.value) }))}
              />
            </Field>
            <Field label="启动卷（GB）">
              <input
                type="number"
                min={47}
                className="oci-input"
                value={form.boot_volume_gbs}
                onChange={(e) => setForm((f) => ({ ...f, boot_volume_gbs: Number(e.target.value) }))}
              />
            </Field>
            <Field label="实例数量">
              <input
                type="number"
                min={1}
                className="oci-input"
                value={form.instance_count}
                onChange={(e) => setForm((f) => ({ ...f, instance_count: Number(e.target.value) }))}
              />
            </Field>
            <Field label="重试间隔（秒）">
              <input
                type="number"
                min={5}
                className="oci-input"
                value={form.interval_sec}
                onChange={(e) => setForm((f) => ({ ...f, interval_sec: Number(e.target.value) }))}
              />
            </Field>
            <Field label="最大尝试次数（0 表示不限）">
              <input
                type="number"
                min={0}
                className="oci-input"
                value={form.max_attempts}
                onChange={(e) => setForm((f) => ({ ...f, max_attempts: Number(e.target.value) }))}
              />
            </Field>

            <div className="sm:col-span-2">
              <Field label="镜像" hint="可输入关键字过滤，例如 Ubuntu、Oracle-Linux">
                <input
                  className="oci-input mb-1"
                  placeholder="过滤镜像…"
                  value={imageFilter}
                  onChange={(e) => setImageFilter(e.target.value)}
                />
                <select
                  className="oci-input"
                  value={form.image_id}
                  onChange={(e) => setForm((f) => ({ ...f, image_id: e.target.value }))}
                >
                  <option value="">请选择</option>
                  {filteredImages.map((img) => (
                    <option key={img.id} value={img.id}>
                      {img.display_name}
                    </option>
                  ))}
                </select>
              </Field>
            </div>

            <div className="sm:col-span-2">
              <Field label="SSH 公钥" hint="将写入实例的 ~/.ssh/authorized_keys">
                <textarea
                  className="oci-input h-20 font-mono text-xs"
                  value={form.ssh_public_key}
                  onChange={(e) => setForm((f) => ({ ...f, ssh_public_key: e.target.value }))}
                />
              </Field>
            </div>
          </div>

          <div className="mt-4 flex justify-end gap-2">
            <button type="button" className="oci-btn-secondary" onClick={() => setFormOpen(false)}>
              取消
            </button>
            <button
              type="button"
              className="oci-btn-primary"
              disabled={submitting}
              onClick={() => void submitForm()}
            >
              {submitting ? '创建中…' : '创建任务'}
            </button>
          </div>
        </Modal>
      )}

      {logTask && (
        <Modal title={`任务日志：${logTask.display_name}`} onClose={() => setLogTask(null)} width="max-w-3xl">
          <div className="max-h-96 overflow-y-auto rounded border border-oracle-border bg-oracle-ink p-3 font-mono text-xs leading-relaxed">
            {logs.length === 0 ? (
              <p className="text-white/50">暂无日志</p>
            ) : (
              logs.map((log) => (
                <p
                  key={log.id}
                  className={
                    log.level === 'error'
                      ? 'text-red-400'
                      : log.level === 'warn'
                        ? 'text-amber-400'
                        : log.level === 'success'
                          ? 'text-green-400'
                          : 'text-white/80'
                  }
                >
                  [{formatDateTime(log.created_at)}] {log.message}
                </p>
              ))
            )}
          </div>
          {logTask.instance_ids.length > 0 && (
            <p className="mt-3 break-all text-xs text-oracle-muted">
              已创建实例：{logTask.instance_ids.join(', ')}
            </p>
          )}
        </Modal>
      )}
    </div>
  )
}
