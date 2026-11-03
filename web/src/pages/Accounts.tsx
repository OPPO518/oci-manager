import { useEffect, useState } from 'react'
import { Badge, Card, ErrorBanner, Field, Loading, Modal } from '../components/ui'
import { api } from '../lib/api'
import { useAccount } from '../lib/context'
import { formatDateTime, truncate } from '../lib/format'
import type { Account, AccountInput, TestResult } from '../lib/types'

const EMPTY_FORM: AccountInput = {
  name: '',
  tenancy_ocid: '',
  user_ocid: '',
  fingerprint: '',
  region: 'us-ashburn-1',
  proxy_url: '',
  private_key_pem: '',
  passphrase: '',
}

export default function Accounts() {
  const { accounts, reload, account, selectAccount } = useAccount()
  const [form, setForm] = useState<AccountInput>(EMPTY_FORM)
  const [editing, setEditing] = useState<Account | null>(null)
  const [formOpen, setFormOpen] = useState(false)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState('')
  const [testResult, setTestResult] = useState<TestResult | null>(null)

  useEffect(() => {
    if (!account && accounts.length > 0) {
      selectAccount(accounts[0].id)
    }
  }, [account, accounts, selectAccount])

  function openCreate() {
    setEditing(null)
    setForm(EMPTY_FORM)
    setFormOpen(true)
  }

  function openEdit(item: Account) {
    setEditing(item)
    setForm({
      name: item.name,
      tenancy_ocid: item.tenancy_ocid,
      user_ocid: item.user_ocid,
      fingerprint: item.fingerprint,
      region: item.region,
      proxy_url: item.proxy_url,
      private_key_pem: '',
      passphrase: '',
    })
    setFormOpen(true)
  }

  async function submit() {
    setSaving(true)
    setError('')
    try {
      if (editing) {
        await api.accounts.update(editing.id, form)
      } else {
        await api.accounts.create(form)
      }
      setFormOpen(false)
      await reload()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  async function remove(item: Account) {
    setError('')
    try {
      await api.accounts.remove(item.id)
      await reload()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function test(item: Account) {
    setTesting(item.id)
    setTestResult(null)
    setError('')
    try {
      const result = await api.accounts.test(item.id)
      setTestResult(result)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setTesting('')
    }
  }

  return (
    <div className="space-y-4">
      {error && <ErrorBanner message={error} onClose={() => setError('')} />}

      {testResult && (
        <div className="rounded border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700">
          连接成功：租户「{testResult.tenancy_name}」，已订阅 {testResult.region_count} 个区域，
          当前区域 {testResult.region}
          {testResult.proxy_url ? `，出口代理 ${testResult.proxy_url}` : '，直连'}，
          耗时 {testResult.latency_ms} ms
        </div>
      )}

      <Card
        title="OCI 账号"
        extra={
          <button type="button" className="oci-btn-primary" onClick={openCreate}>
            添加账号
          </button>
        }
      >
        <p className="mb-3 text-xs text-oracle-muted">
          每个账号可以单独指定出口代理（http / https / socks5），避免多个账号共用同一 IP 触发风控。
        </p>

        {accounts.length === 0 ? (
          <p className="py-8 text-center text-sm text-oracle-muted">还没有账号</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="oci-table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>区域</th>
                  <th>出口代理</th>
                  <th>租户 OCID</th>
                  <th>指纹</th>
                  <th>添加时间</th>
                  <th className="text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {accounts.map((item) => (
                  <tr key={item.id}>
                    <td className="font-medium">{item.name}</td>
                    <td className="text-xs">{item.region}</td>
                    <td>
                      {item.proxy_url ? (
                        <Badge tone="border-blue-200 bg-blue-50 text-blue-700">
                          {truncate(item.proxy_url, 24, 0)}
                        </Badge>
                      ) : (
                        <span className="text-xs text-oracle-muted">直连</span>
                      )}
                    </td>
                    <td className="font-mono text-xs">{truncate(item.tenancy_ocid)}</td>
                    <td className="font-mono text-xs">{truncate(item.fingerprint, 14, 0)}</td>
                    <td className="text-xs text-oracle-muted">{formatDateTime(item.created_at)}</td>
                    <td>
                      <div className="flex justify-end gap-1">
                        <button
                          type="button"
                          className="oci-btn-secondary"
                          disabled={testing === item.id}
                          onClick={() => void test(item)}
                        >
                          {testing === item.id ? '测试中…' : '测试连接'}
                        </button>
                        <button type="button" className="oci-btn-secondary" onClick={() => openEdit(item)}>
                          编辑
                        </button>
                        <button type="button" className="oci-btn-danger" onClick={() => void remove(item)}>
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
        <Modal
          title={editing ? '编辑账号' : '添加账号'}
          onClose={() => setFormOpen(false)}
          width="max-w-2xl"
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label="账号名称">
              <input
                className="oci-input"
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              />
            </Field>
            <Field label="区域">
              <input
                className="oci-input"
                value={form.region}
                onChange={(e) => setForm((f) => ({ ...f, region: e.target.value }))}
              />
            </Field>
            <div className="sm:col-span-2">
              <Field label="租户 OCID（tenancy）">
                <input
                  className="oci-input font-mono text-xs"
                  value={form.tenancy_ocid}
                  onChange={(e) => setForm((f) => ({ ...f, tenancy_ocid: e.target.value }))}
                />
              </Field>
            </div>
            <div className="sm:col-span-2">
              <Field label="用户 OCID（user）">
                <input
                  className="oci-input font-mono text-xs"
                  value={form.user_ocid}
                  onChange={(e) => setForm((f) => ({ ...f, user_ocid: e.target.value }))}
                />
              </Field>
            </div>
            <div className="sm:col-span-2">
              <Field label="公钥指纹（fingerprint）">
                <input
                  className="oci-input font-mono text-xs"
                  value={form.fingerprint}
                  onChange={(e) => setForm((f) => ({ ...f, fingerprint: e.target.value }))}
                />
              </Field>
            </div>
            <div className="sm:col-span-2">
              <Field
                label="出口代理"
                hint="留空表示直连。示例：http://user:pass@1.2.3.4:8080 或 socks5://1.2.3.4:1080"
              >
                <input
                  className="oci-input"
                  value={form.proxy_url}
                  onChange={(e) => setForm((f) => ({ ...f, proxy_url: e.target.value }))}
                />
              </Field>
            </div>
            <div className="sm:col-span-2">
              <Field
                label="API 私钥（PEM 全文）"
                hint={editing ? '留空表示不修改已保存的私钥' : undefined}
              >
                <textarea
                  className="oci-input h-32 font-mono text-xs"
                  placeholder="-----BEGIN PRIVATE KEY-----"
                  value={form.private_key_pem}
                  onChange={(e) => setForm((f) => ({ ...f, private_key_pem: e.target.value }))}
                />
              </Field>
            </div>
            <div className="sm:col-span-2">
              <Field label="私钥口令（没有则留空）">
                <input
                  type="password"
                  className="oci-input"
                  value={form.passphrase}
                  onChange={(e) => setForm((f) => ({ ...f, passphrase: e.target.value }))}
                />
              </Field>
            </div>
          </div>

          <div className="mt-4 flex justify-end gap-2">
            <button type="button" className="oci-btn-secondary" onClick={() => setFormOpen(false)}>
              取消
            </button>
            <button type="button" className="oci-btn-primary" disabled={saving} onClick={() => void submit()}>
              {saving ? '保存中…' : '保存'}
            </button>
          </div>
        </Modal>
      )}

      {accounts.length === 0 && <Loading text="" />}
    </div>
  )
}
