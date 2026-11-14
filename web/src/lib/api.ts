import type {
  Account,
  AccountInput,
  AuditLog,
  AuthStatus,
  BootVolume,
  CapacityQueryInput,
  CapacityResponse,
  Compartment,
  DemoSettings,
  GrabTask,
  GrabTaskInput,
  GrabTaskLog,
  IAMOverview,
  IamApiKeyResult,
  IamCredentialInput,
  IamMembershipResult,
  IamPasswordResult,
  IamSettings,
  IamUserResult,
  Image,
  Instance,
  InstanceDetail,
  IpRotationSettings,
  LoginResult,
  OtpPayload,
  OtpRequestResult,
  Overview,
  PublicIp,
  Region,
  RotateIpResult,
  SecurityList,
  SecurityRule,
  StatusResult,
  Subnet,
  TelegramSettings,
  TelegramSettingsInput,
  TLSSettings,
  TLSSettingsInput,
  TestResult,
  UsagePoint,
  Vnic,
} from './types'

const BASE = '/api'

// 任何接口返回 401 都说明会话失效，统一回到登录页
let unauthorizedHandler: (() => void) | null = null

export function setUnauthorizedHandler(fn: (() => void) | null) {
  unauthorizedHandler = fn
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  })
  if (res.status === 204) {
    return undefined as T
  }
  const text = await res.text()
  if (res.status === 401 && path !== '/auth/status' && unauthorizedHandler) {
    unauthorizedHandler()
  }
  if (!res.ok) {
    let message = text
    try {
      const parsed = JSON.parse(text) as { error?: string }
      message = parsed.error ?? text
    } catch {
      // 非 JSON 响应直接使用原始文本
    }
    throw new ApiError(message || `请求失败（HTTP ${res.status}）`, res.status)
  }
  return JSON.parse(text) as T
}

function query(params: Record<string, string | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value) search.set(key, value)
  }
  const qs = search.toString()
  return qs ? `?${qs}` : ''
}

export class ApiError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.status = status
    this.name = 'ApiError'
  }
}

/** 资源类接口通用的区域 / 区间过滤参数 */
type ResourceParams = { region?: string; compartment_id?: string }

export const api = {
  auth: {
    status: () => request<AuthStatus>('/auth/status'),
    setup: (username: string, password: string) =>
      request<{ status: string }>('/auth/setup', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      }),
    login: (username: string, password: string) =>
      request<LoginResult>('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      }),
    verifyOtp: (challenge: string, code: string) =>
      request<{ status: string; username: string }>('/auth/otp', {
        method: 'POST',
        body: JSON.stringify({ challenge, code }),
      }),
    logout: () => request<{ status: string }>('/auth/logout', { method: 'POST' }),
    changePassword: (username: string, oldPassword: string, newPassword: string) =>
      request<{ status: string }>('/auth/password', {
        method: 'POST',
        body: JSON.stringify({
          username,
          old_password: oldPassword,
          new_password: newPassword,
        }),
      }),
  },

  telegram: {
    get: () => request<TelegramSettings>('/settings/telegram'),
    save: (body: TelegramSettingsInput) =>
      request<TelegramSettings>('/settings/telegram', {
        method: 'PUT',
        body: JSON.stringify(body),
      }),
    test: () => request<{ status: string }>('/settings/telegram/test', { method: 'POST' }),
  },

  precheck: {
    get: () => request<{ enabled: boolean }>('/settings/precheck'),
    save: (enabled: boolean) =>
      request<{ enabled: boolean }>('/settings/precheck', {
        method: 'PUT',
        body: JSON.stringify({ enabled }),
      }),
  },

  tls: {
    get: () => request<TLSSettings>('/settings/tls'),
    save: (body: TLSSettingsInput) =>
      request<TLSSettings>('/settings/tls', {
        method: 'PUT',
        body: JSON.stringify(body),
      }),
    disable: () => request<TLSSettings>('/settings/tls/disable', { method: 'POST' }),
  },

  accounts: {
    list: () => request<Account[]>('/accounts'),
    get: (id: string) => request<Account>(`/accounts/${id}`),
    create: (body: AccountInput) =>
      request<Account>('/accounts', { method: 'POST', body: JSON.stringify(body) }),
    update: (id: string, body: Partial<AccountInput>) =>
      request<Account>(`/accounts/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
    remove: (id: string) => request<void>(`/accounts/${id}`, { method: 'DELETE' }),
    test: (id: string) => request<TestResult>(`/accounts/${id}/test`, { method: 'POST' }),
  },

  capacity: {
    check: (accountId: string, body: CapacityQueryInput) =>
      request<CapacityResponse>(`/accounts/${accountId}/capacity`, {
        method: 'POST',
        body: JSON.stringify(body),
      }),
  },

  resources: {
    regions: (accountId: string) => request<Region[]>(`/accounts/${accountId}/regions`),
    compartments: (accountId: string, region?: string) =>
      request<Compartment[]>(`/accounts/${accountId}/compartments${query({ region })}`),
    availabilityDomains: (accountId: string, params: { region?: string; compartment_id?: string }) =>
      request<string[]>(`/accounts/${accountId}/availability-domains${query(params)}`),
    subnets: (accountId: string, params: { region?: string; compartment_id?: string }) =>
      request<Subnet[]>(`/accounts/${accountId}/subnets${query(params)}`),
    images: (accountId: string, params: { region?: string; compartment_id?: string }) =>
      request<Image[]>(`/accounts/${accountId}/images${query(params)}`),
    /** 资源详情聚合视图：实例 + 引导卷 + VNIC + 公网 IP + 安全列表 */
    detail: (accountId: string, instanceId: string, params: ResourceParams = {}) =>
      request<InstanceDetail>(
        `/accounts/${accountId}/instances/${instanceId}/detail${query(params)}`,
      ),
  },

  instances: {
    list: (accountId: string, params: { region?: string; compartment_id?: string }) =>
      request<Instance[]>(`/accounts/${accountId}/instances${query(params)}`),
    action: (accountId: string, instanceId: string, action: string, region?: string) =>
      request<{ status: string }>(
        `/accounts/${accountId}/instances/${instanceId}/action${query({ region })}`,
        { method: 'POST', body: JSON.stringify({ action }) },
      ),
    updateShape: (
      accountId: string,
      instanceId: string,
      body: { ocpus: number; memory_gbs: number },
      region?: string,
    ) =>
      request<{ status: string }>(
        `/accounts/${accountId}/instances/${instanceId}/shape${query({ region })}`,
        { method: 'PUT', body: JSON.stringify(body) },
      ),
    terminate: (
      accountId: string,
      instanceId: string,
      preserveBootVolume: boolean,
      region?: string,
    ) =>
      request<{ status: string }>(
        `/accounts/${accountId}/instances/${instanceId}${query({
          region,
          preserve_boot_volume: String(preserveBootVolume),
        })}`,
        { method: 'DELETE' },
      ),
    rename: (accountId: string, instanceId: string, displayName: string, region?: string) =>
      request<StatusResult>(
        `/accounts/${accountId}/instances/${instanceId}/display-name${query({ region })}`,
        { method: 'PUT', body: JSON.stringify({ display_name: displayName }) },
      ),
    detail: (accountId: string, instanceId: string, params: ResourceParams = {}) =>
      request<InstanceDetail>(
        `/accounts/${accountId}/instances/${instanceId}/detail${query(params)}`,
      ),
  },

  cost: {
    query: (
      accountId: string,
      params: { start?: string; end?: string; granularity?: string; group_by?: string },
    ) => request<UsagePoint[]>(`/accounts/${accountId}/cost${query(params)}`),
    overview: (accountId: string, region?: string) =>
      request<Overview>(`/accounts/${accountId}/overview${query({ region })}`),
  },

  grabTasks: {
    list: (accountId?: string) => request<GrabTask[]>(`/grab-tasks${query({ account_id: accountId })}`),
    get: (id: string) => request<GrabTask>(`/grab-tasks/${id}`),
    create: (body: GrabTaskInput) =>
      request<GrabTask>('/grab-tasks', { method: 'POST', body: JSON.stringify(body) }),
    remove: (id: string) => request<void>(`/grab-tasks/${id}`, { method: 'DELETE' }),
    start: (id: string) => request<{ status: string }>(`/grab-tasks/${id}/start`, { method: 'POST' }),
    stop: (id: string) => request<{ status: string }>(`/grab-tasks/${id}/stop`, { method: 'POST' }),
    logs: (id: string, after?: number, limit?: number) =>
      request<GrabTaskLog[]>(
        `/grab-tasks/${id}/logs${query({ after: after?.toString(), limit: limit?.toString() })}`,
      ),
  },

  /* ----------------------------------------------------------------
     演示模式 / OTP / 审计日志
     ---------------------------------------------------------------- */
  demo: {
    get: () => request<DemoSettings>('/settings/demo'),
    set: (enabled: boolean) =>
      request<DemoSettings>('/settings/demo', {
        method: 'PUT',
        body: JSON.stringify({ enabled }),
      }),
  },

  otp: {
    request: (operation: string, summary: string, demo = false) =>
      request<OtpRequestResult>('/otp/request', {
        method: 'POST',
        body: JSON.stringify({ operation, summary, demo }),
      }),
  },

  audit: {
    list: (limit = 100) =>
      request<AuditLog[]>(`/audit-logs${query({ limit: String(limit) })}`),
  },

  /* ----------------------------------------------------------------
     引导卷
     ---------------------------------------------------------------- */
  storage: {
    bootVolumes: (accountId: string, params: ResourceParams = {}) =>
      request<BootVolume[]>(`/accounts/${accountId}/boot-volumes${query(params)}`),
    updateBootVolume: (
      accountId: string,
      volumeId: string,
      body: { display_name?: string; size_gbs?: number; vpus_per_gb?: number },
      params: ResourceParams = {},
    ) =>
      request<StatusResult>(
        `/accounts/${accountId}/boot-volumes/${volumeId}${query(params)}`,
        { method: 'PUT', body: JSON.stringify(body) },
      ),
  },

  /* ----------------------------------------------------------------
     网络：VNIC / 安全列表 / 公网 IP
     ---------------------------------------------------------------- */
  network: {
    vnics: (accountId: string, params: ResourceParams = {}) =>
      request<Vnic[]>(`/accounts/${accountId}/vnics${query(params)}`),
    updateVnic: (
      accountId: string,
      vnicId: string,
      body: { display_name?: string; hostname_label?: string },
      params: ResourceParams = {},
    ) =>
      request<StatusResult>(
        `/accounts/${accountId}/vnics/${vnicId}${query(params)}`,
        { method: 'PUT', body: JSON.stringify(body) },
      ),
    securityLists: (accountId: string, params: ResourceParams = {}) =>
      request<SecurityList[]>(`/accounts/${accountId}/security-lists${query(params)}`),
    updateSecurityList: (
      accountId: string,
      listId: string,
      body: { ingress_rules: SecurityRule[]; egress_rules: SecurityRule[] },
      params: ResourceParams = {},
    ) =>
      request<StatusResult>(
        `/accounts/${accountId}/security-lists/${listId}${query(params)}`,
        { method: 'PUT', body: JSON.stringify(body) },
      ),
    publicIps: (accountId: string) =>
      request<PublicIp[]>(`/accounts/${accountId}/public-ips`),
    rotateIp: (accountId: string, body: { private_ip_id: string; otp: OtpPayload }) =>
      request<RotateIpResult>(`/accounts/${accountId}/public-ips/rotate`, {
        method: 'POST',
        body: JSON.stringify(body),
      }),
  },

  /* ----------------------------------------------------------------
     换 IP 保护策略
     ---------------------------------------------------------------- */
  ipRotation: {
    get: () => request<IpRotationSettings>('/settings/ip-rotation'),
    save: (body: { cooldown_sec: number; daily_limit: number }) =>
      request<IpRotationSettings>('/settings/ip-rotation', {
        method: 'PUT',
        body: JSON.stringify(body),
      }),
  },

  /* ----------------------------------------------------------------
     IAM（双钥匙 + 三道闸门）
     ---------------------------------------------------------------- */
  iam: {
    settings: () => request<IamSettings>('/settings/iam'),
    saveSettings: (body: { enabled: boolean; mode?: string; home_region?: string }) =>
      request<IamSettings>('/settings/iam', {
        method: 'PUT',
        body: JSON.stringify(body),
      }),
    credential: (body: IamCredentialInput) =>
      request<IamSettings>('/settings/iam/credential', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    deleteCredential: () =>
      request<StatusResult>('/settings/iam/credential', { method: 'DELETE' }),
    overview: (accountId: string) =>
      request<IAMOverview>(`/accounts/${accountId}/iam/overview`),
    createUser: (
      accountId: string,
      body: { name: string; description: string; email: string; otp: OtpPayload },
    ) =>
      request<IamUserResult>(`/accounts/${accountId}/iam/users`, {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    updateUser: (
      accountId: string,
      userId: string,
      body: { description?: string; email?: string; disabled?: boolean; otp: OtpPayload },
    ) =>
      request<IamUserResult>(`/accounts/${accountId}/iam/users/${userId}`, {
        method: 'PUT',
        body: JSON.stringify(body),
      }),
    deleteUser: (accountId: string, userId: string, otp: OtpPayload) =>
      request<StatusResult>(`/accounts/${accountId}/iam/users/${userId}`, {
        method: 'DELETE',
        body: JSON.stringify({ otp }),
      }),
    resetPassword: (accountId: string, userId: string, otp: OtpPayload) =>
      request<IamPasswordResult>(`/accounts/${accountId}/iam/users/${userId}/password`, {
        method: 'POST',
        body: JSON.stringify({ otp }),
      }),
    createApiKey: (
      accountId: string,
      userId: string,
      body: { public_key_pem: string; otp: OtpPayload },
    ) =>
      request<IamApiKeyResult>(`/accounts/${accountId}/iam/users/${userId}/api-keys`, {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    deleteApiKey: (accountId: string, userId: string, fingerprint: string, otp: OtpPayload) =>
      request<StatusResult>(
        `/accounts/${accountId}/iam/users/${userId}/api-keys/${fingerprint}`,
        { method: 'DELETE', body: JSON.stringify({ otp }) },
      ),
    addMember: (
      accountId: string,
      groupId: string,
      body: { user_id: string; otp: OtpPayload },
    ) =>
      request<IamMembershipResult>(`/accounts/${accountId}/iam/groups/${groupId}/members`, {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    removeMember: (accountId: string, groupId: string, userId: string, otp: OtpPayload) =>
      request<StatusResult>(
        `/accounts/${accountId}/iam/groups/${groupId}/members/${userId}`,
        { method: 'DELETE', body: JSON.stringify({ otp }) },
      ),
  },
}
