# OCI 管理器

用 Go 编写的 Oracle Cloud Infrastructure 个人账户管理器。通过 OCI 官方 API 完成资源盘点、
实例控制、ARM 资源抢占与成本监控，界面仿照 Oracle Cloud 控制台。

支持多个 OCI 账号，且**每个账号可以配置独立的出口代理**，避免多账号共用同一 IP 触发风控。

## 功能

| 模块 | 能力 |
| --- | --- |
| **登录鉴权** | 首次访问强制设置管理员账号；密码 bcrypt 加盐存储；会话 HttpOnly Cookie，7 天有效；失败 10 次锁 15 分钟 |
| **两步验证** | 开启后登录需输入 Telegram 收到的 6 位验证码（5 分钟有效，错 5 次作废） |
| **Telegram 通知** | 登录提醒、登录验证码、ARM 抢购成功/终止推送；Bot Token 加密存库 |
| **容量探测** | 用 OCI Compute Capacity Report 只读接口提前查询各可用域/故障域的机型余量，不创建实例、不触发风控 |
| **HTTPS 证书** | 后台配置 Cloudflare API Token，通过 DNS-01 自动申请/续期 Let's Encrypt 证书，热加载无需重启 |
| 资源总览 | 实例数量、运行中 OCPU / 内存、区间数量、本月消费 |
| 实例管理 | 列表、启动 / 停止 / 重启、调整 Flex 规格、终止（可选保留启动卷） |
| ARM 抢购 | 先做容量预检、仅在有货时发起创建请求，实时日志；接口不可用时自动降级 |
| 成本监控 | 按服务 / 区域聚合消费，支持按天、按月、合计三种粒度 |
| 多账号 | 凭证加密存入 SQLite，每个账号独立代理出口 |
| **演示模式** | 内置演示账号，数据全部内存模拟，**绝不发出真实 OCI 请求**，未录入凭证也能完整试用全部页面与交互 |
| 资源编辑 | 实例 / 引导卷 / VNIC 改名、引导卷容量与 VPU、安全列表规则可视化编辑、实例详情抽屉 |
| 公网 IP | 更换临时公网 IP，带冷却时间、每日上限、Telegram 验证码确认与审计日志 |
| **IAM 管理** | 用户 / 组 / 成员 / API Key 列表与写操作；高权限凭证仅驻留内存 10 分钟、不落盘；每次写操作 OTP 确认 + 服务端禁止清单 + 追加式审计 |
| 界面 | 仿 Oracle Cloud 控制台（Redwood）：深色全局顶栏、图标侧栏、面包屑、卡片与状态徽标 |

## 演示模式（零风险试用）

**还没录入任何 OCI 凭证，也能把整站走一遍。** 账号列表里第一条是固定的「演示账号」，
它是一份内存模拟数据（3 台实例、3 个可用域、引导卷 / VNIC / 公网 IP / 安全列表、IAM 用户组等），
所有读写操作都只改这份内存状态：

- 演示客户端**不持有任何 OCI SDK 客户端**，任何漏短路只会导致报错，绝不会发出网络请求；
  因此不存在「试用时误触风控」的可能。
- 改名、改规格、换 IP、创建 IAM 用户等写操作会立刻反映在后续读取上，可以完整体验交互。
- 敏感操作的验证码固定为 `123456`，自动填入、不会发送 Telegram。
- 换 IP 的冷却与每日次数上限对演示账号不生效（方便反复试用），真实账号一步不少。
- IAM 模块默认关闭（保护真实账号）。演示账号无需导入任何凭证：在「身份与权限」或
  「设置 → IAM」点一下开关启用后，即可试用全部 IAM 功能。
- 演示账号的凭证不可编辑 / 删除 / 测试连接；不想显示它可在「设置 → 演示模式」关闭，
  重启后仍会自动补回。

> 演示固定验证码只对演示挑战生效：挑战类型在服务端区分，真实账号的写操作不可能用
> `123456` 蒙混过关；真实账号必须先在设置里启用 Telegram 才能收到验证码。

## 技术栈

- 后端：Go 1.27 + [oci-go-sdk](https://github.com/oracle/oci-go-sdk) + Gin + SQLite（modernc.org/sqlite，无 CGO）
- 前端：React 18 + TypeScript + Vite + Tailwind CSS，构建产物由 `embed` 打进二进制
- 部署：单容器，单二进制，无外部依赖

## 快速开始

### 1. 准备 OCI API 凭证

1. 登录 [Oracle Cloud 控制台](https://cloud.oracle.com/)
2. 右上角头像 → **My profile**
3. 左侧 **Resources** → **API keys** → **Add API key**
4. 选择 **Generate API key pair**，下载私钥（`.pem`），点击 **Add**
5. 复制生成的配置片段，其中包含 `user`、`tenancy`、`fingerprint`、`region`

> 私钥只保存在本地，服务端会用 AES-256-GCM 加密后写入数据库，接口永不返回明文。

### 2. 一键部署到 VPS（推荐）

在 VPS 上执行一条命令即可，脚本会自动装 Docker、生成加密密钥、拉预构建镜像、修好目录权限并启动：

```bash
curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh | sudo bash
```

重复执行同一条命令即为升级。完整说明见 [DEPLOY.md](DEPLOY.md)。

### 2b. 手动 / 本地构建（可选）

```bash
cp .env.example .env
echo "OCI_MANAGER_ENCRYPTION_KEY=$(openssl rand -hex 32)" > .env

docker compose up -d --build
```

### 3. 初始化管理员账号

打开 http://localhost:8080 ，**首次访问会要求设置用户名和密码**（3–32 位用户名、至少 8 位密码）。
设置完成后所有接口都需要登录才能访问。

> 忘记密码：登录服务器删除数据库中的 `admins` 表后重新初始化即可。

### 4. 填入 OCI 凭证

进入「账号」页面填入第 1 步拿到的凭证。

### 5. 配置 Telegram 通知（可选但推荐）

1. Telegram 搜索 `@BotFather`，发送 `/newbot`，按提示创建机器人，得到 **Bot Token**
2. 给你创建的机器人**发一条任意消息**（否则机器人无法主动给你发消息）
3. 浏览器打开 `https://api.telegram.org/bot<你的TOKEN>/getUpdates`，从返回的 JSON 里找到 `chat.id`
4. 在面板「设置」页填入 Token 和 Chat ID，勾选需要的通知项，点「发送测试消息」验证

勾选「登录时要求输入 Telegram 验证码」后，登录流程变为：密码校验通过 → Telegram 收到 6 位验证码 → 输入验证码才能进入。
**验证码发送失败时会直接拒绝登录**，不会降级成仅密码登录。

### 6. 配置 HTTPS（可选）

「设置 → HTTPS 证书」填写域名、邮箱与 Cloudflare API Token，点「启用 HTTPS」。
Token 需要 `Zone:DNS:Edit` 与 `Zone:Zone:Read` 权限，域名 DNS 必须托管在 Cloudflare。
证书通过 DNS-01 申请，热加载生效、无需重启，剩余 30 天内自动续期。
容器内 HTTPS 监听 `8443`，由 compose 映射到宿主机 `443`。证书签发后，HTTP 端口
只保留本机健康检查（`/api/health`），外部访问会 308 跳转到 HTTPS；若证书配错被锁在外面，
临时设 `OCI_MANAGER_ALLOW_HTTP=true` 可关闭跳转。详见 [DEPLOY.md](DEPLOY.md)。

> 未启用 HTTPS 时页面顶部会显示明文警告，**请勿在此时录入真实 OCI API 凭证**。

> 该密钥用于加密 OCI 私钥，录入账号后**不可再更改**，否则已存账号将无法解密。

完整部署步骤（推 GitHub、VPS 构建、HTTPS 与鉴权加固、备份）见 [DEPLOY.md](DEPLOY.md)。

### 7. 本地开发

```bash
make deps          # 安装前端依赖
make web           # 构建前端到 internal/web/dist
make dev           # 启动后端（:8080）
cd web && npm run dev   # 前端热更新（:5173，已配置代理到后端）
```

## 配置

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `OCI_MANAGER_ADDR` | `:8080` | HTTP 监听地址；证书签发后仅本机健康检查可直连 |
| `OCI_MANAGER_HTTPS_ADDR` | `:8443` | HTTPS 监听地址；与 HTTP 共用同一个宿主机端口，由部署脚本映射到该端口 |
| `OCI_MANAGER_DB` | `./data/oci-manager.db` | SQLite 数据库路径（权限自动收紧为 0600） |
| `OCI_MANAGER_ENCRYPTION_KEY` | **无，必填** | 加密 OCI 私钥；缺失或少于 32 字符会拒绝启动，绝不使用默认值 |
| `OCI_MANAGER_ALLOW_HTTP` | `false` | 排障时允许明文 HTTP（会降低安全性） |
| `OCI_MANAGER_WEB_DIR` | 空 | 指定外部前端目录，为空则使用内嵌资源 |

生成加密密钥：

```bash
openssl rand -hex 32
```

## 代理配置

每个账号可以单独设置出口代理，支持三种协议：

```
http://user:pass@1.2.3.4:8080
https://user:pass@1.2.3.4:8443
socks5://user:pass@1.2.3.4:1080
```

实现方式是给每个账号、每个区域构造独立的 `http.Client`（`http.Transport.Proxy`），
客户端按账号缓存，凭证或代理变化后自动重建。

## 容量探测与 ARM 抢购

**为什么需要预检**：直接反复调用 `LaunchInstance` 去撞运气，会产生大量失败的写请求，
是 OCI 个人账户被风控的主要诱因。OCI 官方提供了只读的
`CreateComputeCapacityReport` 接口，可以在不创建实例的前提下查询某个可用域、
某个机型和规格是否有余量。

- **容量探测页**：手动查询，可按区域 / 可用域 / Shape / OCPU / 内存组合扫描，
  结果按故障域列出 `AVAILABLE` / `OUT_OF_HOST_CAPACITY` / `HARDWARE_NOT_SUPPORTED`，
  支持 60 秒自动刷新，并可一键用当前配置创建抢购任务
- **抢购预检**（默认开启）：任务每轮先查容量，只有 `AVAILABLE` 才发起
  `LaunchInstance`；无货时只记日志、等待下一轮，不产生写调用
- **自动降级**：个人账户若无权使用该接口（403/404），任务自动改为直接创建，
  并在日志标注，不会中断抢购。可在「设置 → 抢购策略」关闭预检
- 只在 OCI 返回 `Out of host capacity` 时继续重试；配额、权限、参数错误立即停止
- 可设置重试间隔、最大尝试次数（0 表示不限）、创建数量；日志实时可见
- 服务重启会把遗留的「运行中」任务标记为已中断，不留僵尸状态

> 注意：普通账户不会返回 `available_count`（该字段仅对 DRCC / 白名单租户开放），
> 因此判断有货一律以 `availability_status == AVAILABLE` 为准。
> 容量报告是准实时快照，显示有货后仍可能被别人抢走，属正常现象。

## 目录结构

```
cmd/server           服务入口
internal/config      运行配置
internal/secret      私钥加密（AES-256-GCM）
internal/store       SQLite 持久化（账号、抢购任务、日志）
internal/oci         OCI API 封装与多账号客户端池
internal/api         HTTP 接口与静态资源服务
internal/auth        管理员账号、登录会话与两步验证
internal/notify      Telegram 通知
internal/tlsmanager  Let's Encrypt 证书申请、续期与热加载（Cloudflare DNS-01）
internal/grabber     抢购任务调度（含容量预检）
internal/web         内嵌前端产物
web                  React 前端源码
.github/workflows    GitHub Actions 构建并发布多架构镜像到 GHCR
install.sh           VPS 一键部署 / 升级脚本
```

## 许可

MIT
