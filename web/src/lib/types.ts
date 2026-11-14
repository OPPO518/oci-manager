export interface Account {
  id: string
  name: string
  tenancy_ocid: string
  user_ocid: string
  fingerprint: string
  region: string
  proxy_url: string
  created_at: string
  updated_at: string
  /** 后端自动提供的演示账号：所有接口返回模拟数据，不发真实 OCI 请求 */
  is_demo?: boolean
}

export interface AccountInput {
  name: string
  tenancy_ocid: string
  user_ocid: string
  fingerprint: string
  region: string
  proxy_url: string
  private_key_pem: string
  passphrase: string
}

export interface Instance {
  id: string
  display_name: string
  state: string
  shape: string
  ocpus: number
  memory_gbs: number
  availability_domain: string
  compartment_id: string
  region: string
  image_id: string
  time_created: string | null
}

export interface Region {
  key: string
  name: string
  status: string
  is_home_region: boolean
}

export interface Compartment {
  id: string
  name: string
  description: string
  inactive: boolean
}

export interface Image {
  id: string
  display_name: string
  operating_system: string
  operating_system_version: string
}

export interface Subnet {
  id: string
  display_name: string
  cidr_block: string
  vcn_id: string
  availability_domain: string
}

export interface UsagePoint {
  service: string
  resource_name: string
  region: string
  shape: string
  start_time: string
  end_time: string
  cost: number
  quantity: number
  currency: string
}

export type GrabTaskStatus = 'idle' | 'running' | 'success' | 'failed' | 'stopped'

export interface GrabTask {
  id: string
  account_id: string
  display_name: string
  region: string
  compartment_id: string
  ad_name: string
  shape: string
  ocpus: number
  memory_gbs: number
  image_id: string
  subnet_id: string
  ssh_public_key: string
  boot_volume_gbs: number
  instance_count: number
  interval_sec: number
  max_attempts: number
  status: GrabTaskStatus
  message: string
  attempts: number
  instance_ids: string[]
  last_attempt_at: string | null
  created_at: string
  updated_at: string
}

export interface GrabTaskInput {
  account_id: string
  display_name: string
  region: string
  compartment_id: string
  ad_name: string
  shape: string
  ocpus: number
  memory_gbs: number
  image_id: string
  subnet_id: string
  ssh_public_key: string
  boot_volume_gbs: number
  instance_count: number
  interval_sec: number
  max_attempts: number
}

export interface GrabTaskLog {
  id: number
  task_id: string
  level: string
  message: string
  created_at: string
}

export interface Overview {
  account_id: string
  region: string
  tenancy_ocid: string
  compartment_count: number
  instance_count: number
  running_count: number
  total_ocpus: number
  total_memory_gbs: number
  month_cost: number
  currency: string
  compartment_error?: string
  cost_error?: string
}

export interface TestResult {
  tenancy_name: string
  region: string
  proxy_url: string
  region_count: number
  latency_ms: number
}

/** 鉴权状态：initialized 为 false 时前端展示初始化表单 */
export interface AuthStatus {
  initialized: boolean
  authenticated: boolean
  username: string
  login_otp: boolean
  telegram_enabled: boolean
}

/** 登录结果：otp_required 为 true 时需要再输入 Telegram 验证码 */
export interface LoginResult {
  otp_required: boolean
  challenge: string
  username: string
}

export interface TelegramSettings {
  enabled: boolean
  chat_id: string
  has_token: boolean
  login_otp: boolean
  event_grab: boolean
}

export interface TelegramSettingsInput {
  enabled: boolean
  bot_token: string
  chat_id: string
  login_otp: boolean
  event_grab: boolean
}

/** 容量探测：单个可用域 / 故障域组合的结果 */
export interface CapacityResult {
  availability_domain: string
  fault_domain: string
  shape: string
  ocpus: number
  memory_gbs: number
  availability_status: string
  available: boolean
  available_count: number | null
  time_created: string
  error?: string
}

export interface CapacityQueryInput {
  region: string
  shape: string
  ocpus: number
  memory_gbs: number
  availability_domains: string[]
}

export interface CapacityResponse {
  region: string
  shape: string
  ocpus: number
  memory_gbs: number
  compartment_id: string
  items: CapacityResult[]
  checked_at: string
  unsupported: boolean
  hint: string
}

/** HTTPS 证书配置状态 */
export interface TLSSettings {
  enabled: boolean
  domain: string
  email: string
  has_token: boolean
  cert_issued: boolean
  expires_at: string
  last_error: string
  staging: boolean
  https_port: string
}

export interface TLSSettingsInput {
  enabled: boolean
  domain: string
  email: string
  cf_api_token: string
  staging: boolean
}

/* ------------------------------------------------------------------
   演示 / OTP / 审计日志
   ------------------------------------------------------------------ */
export interface DemoSettings {
  enabled: boolean
}

/** 业务写操作统一携带的 OTP 字段 */
export interface OtpPayload {
  challenge: string
  code: string
}

/** POST /otp/request 响应：演示账号下 demo_code 为 "123456" */
export interface OtpRequestResult {
  challenge: string
  demo_code?: string
}

export interface AuditLog {
  id: number
  created_at: string
  account_id: string
  category: string
  operation: string
  target: string
  result: string
  detail: string
  client_ip: string
}

/* ------------------------------------------------------------------
   资源编辑 / 实例详情
   ------------------------------------------------------------------ */
export interface BootVolume {
  id: string
  display_name: string
  size_gbs: number
  vpus_per_gb: number
  state: string
  availability_domain: string
  instance_id: string
  compartment_id: string
}

export interface Vnic {
  id: string
  display_name: string
  hostname_label: string
  private_ip: string
  public_ip: string
  subnet_id: string
  mac_address: string
  state: string
  instance_id: string
}

export interface PublicIp {
  id: string
  ip_address: string
  display_name: string
  lifetime: string
  private_ip_id: string
  state: string
  compartment_id: string
}

export interface SecurityRule {
  protocol: string
  source: string
  source_type: string
  destination: string
  destination_type: string
  tcp_port_min?: number
  tcp_port_max?: number
  udp_port_min?: number
  udp_port_max?: number
  description: string
  stateless: boolean
}

export interface SecurityList {
  id: string
  display_name: string
  vcn_id: string
  ingress_rules: SecurityRule[]
  egress_rules: SecurityRule[]
}

export interface InstanceDetail {
  instance: Instance
  boot_volumes: BootVolume[]
  vnics: Vnic[]
  public_ips: PublicIp[]
  security_lists: SecurityList[]
}

/* ------------------------------------------------------------------
   公网 IP 更换护栏
   ------------------------------------------------------------------ */
export interface IpRotationSettings {
  cooldown_sec: number
  daily_limit: number
  used_today: number
  remaining_today: number
  next_allowed_at: string
}

export interface RotateIpResult {
  status: string
  new_ip: PublicIp
  message: string
}

/* ------------------------------------------------------------------
   IAM（双钥匙 + 三道闸门）
   ------------------------------------------------------------------ */
export interface IamSettings {
  enabled: boolean
  mode: 'classic' | 'domains'
  home_region: string
  has_credential: boolean
  credential_expires_at: string
  telegram_ready: boolean
  demo: boolean
}

export interface IamCredentialInput {
  tenancy_ocid: string
  user_ocid: string
  fingerprint: string
  private_key_pem: string
  passphrase: string
  region: string
}

export interface IAMUser {
  id: string
  name: string
  description: string
  email: string
  state: string
  time_created: string
  can_use_console_password: boolean
  db_user_name: string
}

export interface IAMGroup {
  id: string
  name: string
  description: string
  state: string
  protected: boolean
}

export interface IAMMembership {
  id: string
  user_id: string
  group_id: string
}

export interface IAMApiKey {
  user_id: string
  fingerprint: string
  state: string
  time_created: string
  key_value?: string
}

export interface IAMLimit {
  name: string
  scope: string
  available: number
  used: number
  total: number
}

export interface IAMOverview {
  mode: string
  home_region: string
  users: IAMUser[]
  groups: IAMGroup[]
  memberships: IAMMembership[]
  api_keys: IAMApiKey[]
  limits: IAMLimit[]
  protected_user_ids: string[]
  read_only: boolean
  hint: string
}

export interface IamUserResult {
  user: IAMUser
}

export interface IamApiKeyResult {
  api_key: IAMApiKey
}

export interface IamMembershipResult {
  membership: IAMMembership
}

export interface IamPasswordResult {
  password: string
  hint: string
}

export interface StatusResult {
  status: string
}
