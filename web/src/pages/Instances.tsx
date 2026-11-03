import { useCallback, useEffect, useState } from 'react'
import { Badge, Card, EmptyState, ErrorBanner, Field, Loading, Modal } from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { formatDateTime } from '../lib/format'
import type { Compartment, Instance } from '../lib/types'

export default function Instances() {
  const { account, region } = useAccount()
  const effectiveRegion = region || undefined

  const [compartments, setCompartments] = useState<Compartment[]>([])
  const [compartmentId, setCompartmentId] = useState('')
  const [instances, setInstances] = useState<Instance[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [busyId, setBusyId] = useState('')

  const [shapeTarget, setShapeTarget] = useState<Instance | null>(null)
  const [shapeForm, setShapeForm] = useState({ ocpus: 4, memory_gbs: 24 })
  const [terminateTarget, setTerminateTarget] = useState<Instance | null>(null)
  const [preserveBootVolume, setPreserveBootVolume] = useState(true)

  const loadInstances = useCallback(async () => {
    if (!account) return
    setLoading(true)
    setError('')
    try {
      const list = await api.instances.list(account.id, {
        region: effectiveRegion,
        compartment_id: compartmentId || undefined,
      })
      setInstances(list)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }, [account, effectiveRegion, compartmentId])

  useEffect(() => {
    if (!account) return
    let cancelled = false
    api.resources
      .compartments(account.id, effectiveRegion)
      .then((list) => {
        if (cancelled) return
        setCompartments(list)
        setCompartmentId((prev) => (prev && list.some((c) => c.id === prev) ? prev : account.tenancy_ocid))
      })
      .catch(() => {
        if (!cancelled) setCompartmentId(account.tenancy_ocid)
      })
    return () => {
      cancelled = true
    }
  }, [account, effectiveRegion])

  useEffect(() => {
    void loadInstances()
  }, [loadInstances])

  async function runAction(instance: Instance, action: string) {
    if (!account) return
    setBusyId(instance.id)
    setError('')
    try {
      await api.instances.action(account.id, instance.id, action, effectiveRegion)
      await loadInstances()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusyId('')
    }
  }

  async function submitShape() {
    if (!account || !shapeTarget) return
    setBusyId(shapeTarget.id)
    setError('')
    try {
      await api.instances.updateShape(account.id, shapeTarget.id, shapeForm, effectiveRegion)
      setShapeTarget(null)
      await loadInstances()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusyId('')
    }
  }

  async function submitTerminate() {
    if (!account || !terminateTarget) return
    setBusyId(terminateTarget.id)
    setError('')
    try {
      await api.instances.terminate(account.id, terminateTarget.id, preserveBootVolume, effectiveRegion)
      setTerminateTarget(null)
      await loadInstances()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusyId('')
    }
  }

  if (!account) return null

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}

      <Card
        title="计算实例"
        extra={
          <div className="flex items-center gap-2">
            <select
              className="oci-input max-w-xs"
              value={compartmentId}
              onChange={(e) => setCompartmentId(e.target.value)}
            >
              {compartments.length === 0 && <option value={account.tenancy_ocid}>根区间</option>}
              {compartments.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
            <button type="button" className="oci-btn-secondary" onClick={() => void loadInstances()}>
              刷新
            </button>
          </div>
        }
      >
        {loading && instances.length === 0 ? (
          <Loading />
        ) : instances.length === 0 ? (
          <EmptyState text="该区间下没有实例" />
        ) : (
          <div className="overflow-x-auto">
            <table className="oci-table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>状态</th>
                  <th>规格</th>
                  <th>OCPU / 内存</th>
                  <th>可用域</th>
                  <th>创建时间</th>
                  <th className="text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {instances.map((inst) => (
                  <tr key={inst.id}>
                    <td>
                      <div className="font-medium">{inst.display_name || '(未命名)'}</div>
                      <div className="font-mono text-xs text-oracle-muted">{inst.id.slice(-16)}</div>
                    </td>
                    <td>
                      <Badge>{inst.state}</Badge>
                    </td>
                    <td className="font-mono text-xs">{inst.shape}</td>
                    <td>
                      {inst.ocpus ? `${inst.ocpus} / ${inst.memory_gbs} GB` : '—'}
                    </td>
                    <td className="text-xs text-oracle-muted">{inst.availability_domain}</td>
                    <td className="text-xs text-oracle-muted">{formatDateTime(inst.time_created)}</td>
                    <td>
                      <div className="flex justify-end gap-1">
                        <button
                          type="button"
                          className="oci-btn-secondary"
                          disabled={busyId === inst.id || inst.state === 'RUNNING'}
                          onClick={() => void runAction(inst, 'start')}
                        >
                          启动
                        </button>
                        <button
                          type="button"
                          className="oci-btn-secondary"
                          disabled={busyId === inst.id || inst.state !== 'RUNNING'}
                          onClick={() => void runAction(inst, 'stop')}
                        >
                          停止
                        </button>
                        <button
                          type="button"
                          className="oci-btn-secondary"
                          disabled={busyId === inst.id || inst.state !== 'RUNNING'}
                          onClick={() => void runAction(inst, 'softreset')}
                        >
                          重启
                        </button>
                        <button
                          type="button"
                          className="oci-btn-secondary"
                          disabled={busyId === inst.id}
                          onClick={() => {
                            setShapeForm({
                              ocpus: inst.ocpus || 4,
                              memory_gbs: inst.memory_gbs || 24,
                            })
                            setShapeTarget(inst)
                          }}
                        >
                          改规格
                        </button>
                        <button
                          type="button"
                          className="oci-btn-danger"
                          disabled={busyId === inst.id}
                          onClick={() => setTerminateTarget(inst)}
                        >
                          终止
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

      {shapeTarget && (
        <Modal title={`调整规格：${shapeTarget.display_name || shapeTarget.id}`} onClose={() => setShapeTarget(null)}>
          <div className="space-y-3">
            <p className="text-xs text-oracle-muted">
              仅 Flex 系列规格支持在线调整。Always Free 账号的 ARM 资源总额度受账号配额限制。
            </p>
            <Field label="OCPU 数量">
              <input
                type="number"
                min={1}
                className="oci-input"
                value={shapeForm.ocpus}
                onChange={(e) => setShapeForm((f) => ({ ...f, ocpus: Number(e.target.value) }))}
              />
            </Field>
            <Field label="内存（GB）">
              <input
                type="number"
                min={1}
                className="oci-input"
                value={shapeForm.memory_gbs}
                onChange={(e) => setShapeForm((f) => ({ ...f, memory_gbs: Number(e.target.value) }))}
              />
            </Field>
            <div className="flex justify-end gap-2">
              <button type="button" className="oci-btn-secondary" onClick={() => setShapeTarget(null)}>
                取消
              </button>
              <button type="button" className="oci-btn-primary" onClick={() => void submitShape()}>
                应用
              </button>
            </div>
          </div>
        </Modal>
      )}

      {terminateTarget && (
        <Modal
          title={`终止实例：${terminateTarget.display_name || terminateTarget.id}`}
          onClose={() => setTerminateTarget(null)}
        >
          <div className="space-y-3">
            <p className="text-sm">终止后实例将被销毁，且不可恢复。</p>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={preserveBootVolume}
                onChange={(e) => setPreserveBootVolume(e.target.checked)}
              />
              保留启动卷
            </label>
            <div className="flex justify-end gap-2">
              <button type="button" className="oci-btn-secondary" onClick={() => setTerminateTarget(null)}>
                取消
              </button>
              <button type="button" className="oci-btn-danger" onClick={() => void submitTerminate()}>
                确认终止
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}
