import { useEffect, useState } from 'react'
import {
  ActionsMenu,
  Badge,
  Card,
  EmptyState,
  ErrorBanner,
  Field,
  Loading,
  Modal,
  Notice,
  usePager,
} from '../components/ui'
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

  const { paged, pager } = usePager(accounts)

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

  /** 关闭或保存后显式清空含 private_key_pem / passphrase 的敏感表单状态 */
  function closeForm() {
    setFormOpen(false)
    setForm(EMPTY_FORM)
    setEditing(null)
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
      closeForm()
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
        <Notice tone="success" className="mb-4">
          连接成功：租户「{testResult.tenancy_name}」，已订阅 {testResult.region_count} 个区域，
          当前区域 {testResult.region}
          {testResult.proxy_url ? `，出口代理 ${testResult.proxy_url}` : '，直连'}，
          耗时 {testResult.latency_ms} ms
        </Notice>
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
        <Notice tone="warning" className="mb-3">
          启用 HTTPS 前不要录入真实凭证；演示账号由系统提供，不会发出任何真实 OCI 请求，可放心试用全部功能。
        </Notice>

        {accounts.length === 0 ? (
          <EmptyState />
        ) : (
          <>
            <div className="oci-table-wrap">
              <table className="oci-table">
                <thead>
                  <tr>
                    <th>名称</th>
                    <th>区域</th>
                    <th>出口代理</th>
                    <th>租户 OCID</th>
                    <th>指纹</th>
                    <th>添加时间</th>
                    <th className="oci-cell-actions">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {paged.map((item) => (
                    <tr key={item.id}>
                      <td className="font-medium">
                        <div className="flex items-center gap-2">
                          {item.name}
                          {item.is_demo && <Badge tone="info">演示</Badge>}
                        </div>
                        {item.is_demo && (
                          <div className="mt-1 text-xs font-normal text-oracle-muted">
                            不发真实 OCI 请求，可放心试用
                          </div>
                        )}
                      </td>
                      <td className="text-xs">{item.region}</td>
                      <td>
                        {item.proxy_url ? (
                          <Badge tone="info">{truncate(item.proxy_url, 24, 0)}</Badge>
                        ) : (
                          <span className="text-xs text-oracle-muted">直连</span>
                        )}
                      </td>
                      <td className="font-mono text-xs">{truncate(item.tenancy_ocid)}</td>
                      <td className="font-mono text-xs">{truncate(item.fingerprint, 14, 0)}</td>
                      <td className="text-xs text-oracle-muted">{formatDateTime(item.created_at)}</td>
                      <td className="oci-cell-actions">
                        <div className="flex justify-end">
                          <ActionsMenu
                            label="账号操作"
                            items={[
                              {
                                label: testing === item.id ? '测试中…' : '测试连接',
                                disabled: testing === item.id || item.is_demo === true,
                                onSelect: () => void test(item),
                              },
                              {
                                label: '编辑',
                                disabled: item.is_demo === true,
                                onSelect: () => openEdit(item),
                              },
                              {
                                label: '删除',
                                danger: true,
                                disabled: item.is_demo === true,
                                separatorBefore: true,
                                onSelect: () => void remove(item),
                              },
                            ]}
                          />
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {pager}
          </>
        )}
      </Card>

      {formOpen && (
        <Modal
          title={editing ? '编辑账号' : '添加账号'}
          onClose={closeForm}
          width="max-w-2xl"
          footer={
            <>
              <button type="button" className="oci-btn-secondary" onClick={closeForm}>
                取消
              </button>
              <button type="button" className="oci-btn-primary" disabled={saving} onClick={() => void submit()}>
                {saving ? '保存中…' : '保存'}
              </button>
            </>
          }
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
                  autoComplete="off"
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
                  autoComplete="off"
                  value={form.passphrase}
                  onChange={(e) => setForm((f) => ({ ...f, passphrase: e.target.value }))}
                />
              </Field>
            </div>
          </div>
        </Modal>
      )}

      {accounts.length === 0 && <Loading text="" />}
    </div>
  )
}
