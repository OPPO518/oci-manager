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
