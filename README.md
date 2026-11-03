# OCI 管理器

用 Go 编写的 Oracle Cloud Infrastructure 个人账户管理器。通过 OCI 官方 API 完成资源盘点、
实例控制、ARM 资源抢占与成本监控，界面仿照 Oracle Cloud 控制台。

支持多个 OCI 账号，且**每个账号可以配置独立的出口代理**，避免多账号共用同一 IP 触发风控。

## 功能

| 模块 | 能力 |
| --- | --- |
| 资源总览 | 实例数量、运行中 OCPU / 内存、区间数量、本月消费 |
| 实例管理 | 列表、启动 / 停止 / 重启、调整 Flex 规格、终止（可选保留启动卷） |
| ARM 抢购 | 定时重试创建 Ampere A1 实例，仅在 `Out of host capacity` 时继续，实时日志 |
| 成本监控 | 按服务 / 区域聚合消费，支持按天、按月、合计三种粒度 |
| 多账号 | 凭证加密存入 SQLite，每个账号独立代理出口 |

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

### 2. 设置加密密钥并启动

```bash
cp .env.example .env
echo "OCI_MANAGER_ENCRYPTION_KEY=$(openssl rand -hex 32)" > .env

docker compose up -d --build
```

打开 http://localhost:8080 ，进入「账号」页面填入上一步的凭证。

> 该密钥用于加密 OCI 私钥，录入账号后**不可再更改**，否则已存账号将无法解密。

完整部署步骤（推 GitHub、VPS 构建、HTTPS 与鉴权加固、备份）见 [DEPLOY.md](DEPLOY.md)。

### 3. 本地开发

```bash
make deps          # 安装前端依赖
make web           # 构建前端到 internal/web/dist
make dev           # 启动后端（:8080）
cd web && npm run dev   # 前端热更新（:5173，已配置代理到后端）
```

## 配置

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `OCI_MANAGER_ADDR` | `:8080` | HTTP 监听地址 |
| `OCI_MANAGER_DB` | `./data/oci-manager.db` | SQLite 数据库路径 |
| `OCI_MANAGER_ENCRYPTION_KEY` | 开发默认值 | 用于加密私钥，部署前务必替换 |
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

## ARM 抢购说明

- 只在 OCI 返回 `Out of host capacity` 时继续重试；配额不足、权限错误、参数错误会立即停止，
  避免无效循环
- 可设置重试间隔、最大尝试次数（0 表示不限）、创建数量
- 任务日志实时可见，创建成功后记录实例 OCID
- 服务重启时会把遗留的「运行中」任务标记为已中断，不会留下僵尸状态

## 目录结构

```
cmd/server           服务入口
internal/config      运行配置
internal/secret      私钥加密（AES-256-GCM）
internal/store       SQLite 持久化（账号、抢购任务、日志）
internal/oci         OCI API 封装与多账号客户端池
internal/api         HTTP 接口与静态资源服务
internal/grabber     抢购任务调度
internal/web         内嵌前端产物
web                  React 前端源码
```

## 许可

MIT
