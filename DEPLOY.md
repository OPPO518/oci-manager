# 部署文档：GitHub → VPS → Docker

## 安全基线（2026-11-14 加固，录入真实凭证前必读）

1. **先启用 HTTPS，再录入 OCI 凭证**。未配置证书时页面顶部会显示明文警告。证书签发后
   宿主机不再暴露 HTTP 端口（需重跑一键脚本完成切换，见「零」），此时后端对仍到达的
   明文 HTTP 业务请求一律返回 **403**（仅容器内 `/api/health` 放行，不再做 308 跳转）。
   切换前证书虽已热加载，HTTP 仍可访问，不会把你锁在门外。
   会话 Cookie 强制带 `Secure`，并下发 HSTS、CSP、`X-Frame-Options`、
   `Referrer-Policy` 等响应头。判定基于真实 TLS 连接，不信任客户端伪造的
   `X-Forwarded-Proto`（服务端未设置任何可信代理）。若使用 Cloudflare Flexible
   这类「反代终结 TLS、回源走 HTTP」的模式，需设 `OCI_MANAGER_ALLOW_HTTP=true`
   且只监听本机，否则回源请求会被 403。
2. **加密密钥没有默认值**。`OCI_MANAGER_ENCRYPTION_KEY` 缺失或少于 32 字符时服务直接
   拒绝启动。`.env` 必须备份，丢失后已录入的账号无法解密。
3. **升级后需要重新登录一次**。会话令牌改为 SHA-256 摘要存储，旧版明文会话记录不再被
   接受——这是为了阻断「读到 `data/` 目录即拿到管理员会话」的攻击路径。
4. **改密码会踢掉所有其它设备**：服务端清空会话表，只给当前浏览器补发新会话。
5. **数据库权限自动收紧**：`data/` 为 0700，`.db` / `-wal` / `-shm` 为 0600；
   一键脚本也会 `chmod 700 data`。备份文件请同样按 0600 保管。
6. **防风控默认参数**：抢购轮询间隔服务端强制 30–3600 秒，单任务最多 10 台实例，
   所有 OCI 调用有 2 秒全局最小间隔；OCI 返回 429 / 5xx / 超时会指数退避（30 秒起，
   最长 15 分钟，带抖动），**不会**再降级成每轮直接创建。只有容量报告接口明确返回
   「不支持 / 无权限」时才降级，且降级后间隔自动拉长到 2 分钟。容量探测单次最多
   10 个可用域、每分钟最多 8 次。
7. **公开接口限流**：初始化 5 次/分钟、登录 10 次/分钟、OTP 20 次/分钟（按来源 IP）；
   登录失败同时按用户名与 IP 计数锁定，且不信任可伪造的 `X-Forwarded-For`。
8. **错误与日志脱敏**：Telegram 网络错误不再回显含 Bot Token 的 URL，代理地址里的
   账号密码在诊断结果中打码，对外错误信息统一过滤路径与凭据。
9. **演示账号零 OCI 请求**：账号列表首条为内置演示账号，所有调用短路到内存模拟数据，
   演示客户端不持有任何 SDK 客户端。未录入凭证时可放心试用全部页面；演示验证码
   `123456` 与真实账号的挑战类型在服务端隔离，不能被拿去执行真实租户的写操作。
   不想要它可在「设置 → 演示模式」关闭。
10. **IAM 双钥匙 + 三道闸门**：日常资源操作继续用库里的低权限凭证；IAM 高权限凭证
    在设置里临时粘贴，**只驻留内存 10 分钟、不落盘、不回显、不写日志**，超时或关闭模块
    立即销毁，且校验凭证租户与当前账号一致。模块默认关闭；每次写操作需 Telegram 验证码
    （绑定操作类型、一次性消费）；服务端硬禁止变更 `Administrators` 组成员、操作面板自用
    API 用户，策略（policy）类接口不提供。IAM 调用固定使用配置的 home region。
    较新租户可能使用 Identity Domains（SCIM），本版本只支持经典 IAM，切到 `domains` 模式
    会明确禁用写操作，不会静默失败。
11. **高危操作护栏**：换公网 IP 需二次确认 + 验证码，真实账号默认冷却 300 秒、每日 5 次；
    删除用户与重置控制台密码额外限流。重置返回的一次性密码只在响应里出现一次，
    不入库、不写日志、不进审计详情、不发 Telegram。所有敏感写操作写入只追加的
    `audit_logs` 表，可在「设置 → 审计日志」查看。

> 本次升级会自动新建 `audit_logs` 表并补一条演示账号记录，不需要手工迁移；
> 已录入的真实账号、抢购任务、证书与 `.env` 密钥全部保留，会话模型未变，无需重新登录。

---

## 零、一键部署（推荐，只需一条命令）

镜像由 GitHub Actions 预先构建并发布到 GHCR，VPS 上**不需要编译前端和 Go 代码**，
也不需要 clone 仓库、不需要手动生成密钥、不需要处理目录权限：

```bash
curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh | sudo bash
```

脚本会自动完成：安装 Docker → 生成随机加密密钥（保存到 `/opt/oci-manager/.env`）→
写入 compose 配置 → 拉取最新镜像 → 修正 `data` 目录属主 → 启动并做健康检查 →
打印访问地址。

**端口模式（按 `data/certs/fullchain.pem` 是否存在判定，只在你主动重跑脚本时切换）：**

| 模式 | 条件 | 宿主机对外映射 |
| --- | --- | --- |
| 首次部署 / HTTP | 无证书 | 仅 `OCI_MANAGER_PORT` → 容器 `8080`，**完全不映射 443** |
| HTTPS | 已签发证书 | 仅 `OCI_MANAGER_HTTPS_PORT` → 容器 `8443`，**不再暴露 HTTP 端口** |

- 首次不映射 443：没有证书，也避开 443 被 nginx / 面板 / 旧容器占用导致部署失败。
  脚本只预检本次真正会映射的那个端口，不再预检 443。
- 证书签发生效后，重跑**同一条命令**即关闭宿主机 HTTP 端口、只保留 HTTPS，
  并提示「HTTP 已关闭」。切换不会自动发生：后台热签发 / 续期证书不会偷偷改端口，
  不会把人锁在外面。
- 端口写入 `.env` 持久化，升级时沿用（除非显式用环境变量覆盖），不会改回默认。
- **重复执行同一条命令即为升级**：只拉新镜像并重启，不会覆盖 `.env`、数据库和证书。
- 可用环境变量覆盖：`OCI_MANAGER_REPO` / `OCI_MANAGER_IMAGE` / `OCI_MANAGER_DIR` /
  `OCI_MANAGER_PORT` / `OCI_MANAGER_HTTPS_PORT`。

### 首次部署自定义 HTTP 端口（两种写法）

**① 管道 / 非交互方式用环境变量**（`curl | sudo bash` 无控制终端，脚本绝不进入交互）：

```bash
curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh -o /tmp/oci-install.sh && sudo OCI_MANAGER_PORT=8088 bash /tmp/oci-install.sh
```

**② 本地脚本可交互输入**：把脚本存到本地、用真终端运行，脚本会提示输入 HTTP 端口，
直接回车用默认值（首次 `8080`，升级时为 `.env` 里已保存的值），输入必须是
1–65535 的纯数字：

```bash
curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh -o /tmp/oci-install.sh
sudo bash /tmp/oci-install.sh
```

> 交互只在**首次部署（HTTP 模式）**且 `[ -t 0 ]`（真终端）、**未通过
> `OCI_MANAGER_PORT` 显式指定**时触发；一旦显式指定端口（第 ① 种）或已检测到
> 证书（HTTPS 模式），都不再询问，保证「重跑同一条命令关闭 HTTP」不会被卡住。
> 端口会写入 `.env`，之后重跑不带参数也会沿用。

### 证书签发后：重跑同一条命令即关闭 HTTP

证书签发生效后，**重新执行上面同一条一键命令**（不带参数也会沿用 `.env` 里的端口）即可：

```bash
curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh -o /tmp/oci-install.sh && sudo bash /tmp/oci-install.sh
```

脚本检测到 `data/certs/fullchain.pem` 与 `privkey.pem` 后切换为 HTTPS 模式：
宿主机不再暴露 HTTP 端口，只映射 `OCI_MANAGER_HTTPS_PORT`（默认 443）→ 容器 8443，
并提示 HTTP 已关闭。注意事项：

- **不会把你锁在门外**：证书是热加载的，但禁用明文 HTTP 的开关
  （`OCI_MANAGER_HTTP_DISABLED=1`）只由脚本在 HTTPS 模式下注入。证书刚签发、
  尚未重跑脚本时，HTTP 仍可正常访问，你可以从容完成切换。
- 切换只在用户主动重跑脚本时发生；脚本不会常驻检测、不会自动改端口。
  切换完成后，任何仍到达容器 8080 的明文 HTTP 业务请求才返回 **403**
  （仅 `/api/health` 放行，供容器内健康检查使用，不再做 308 跳转）。
- 排障逃生阀：显式设置 `OCI_MANAGER_ALLOW_HTTP=true` 可临时放行明文 HTTP，
  默认关闭，正常部署不要开启。
- 若证书被删除（例如面板里关闭 HTTPS），重跑同一条命令会重新判定为 HTTP 模式，
  恢复 HTTP 端口映射。
- 切换后请放行安全组的 `443`（或自定义 HTTPS 端口），并关掉原 HTTP 端口。

### 端口映射是怎么切换的

基础 `docker-compose.yml` **不含 `ports`**：compose 的 `ports` 是「追加合并」，
基础文件若自带 ports，override 无法删掉不要的那条（例如首次部署的 443）。
脚本每次运行按证书状态重写 `docker-compose.override.yml`：

```yaml
# docker-compose.override.yml（由 install.sh 生成，请勿手工编辑或提交到 Git）
services:
  oci-manager:
    ports:
      - "${OCI_MANAGER_PORT}:8080"           # 首次部署（HTTP 模式）
      # 证书已签发后，这一行改为：
      # - "${OCI_MANAGER_HTTPS_PORT}:8443"  # HTTPS 模式，宿主机不暴露 HTTP
```

该文件由脚本生成，不要手工编辑或提交到 Git。容器的 `healthcheck` 走容器内
`127.0.0.1:8080/api/health`，不依赖宿主机 HTTP 映射，HTTPS 模式下依然可用。

> **纯 IP 证书不支持**：证书只允许 Cloudflare DNS-01 域名证书，不开放 80 端口，
> 不实现 HTTP-01 / TLS-ALPN-01 / IP 证书分支。**Passkey 需要域名 + HTTPS**，
> 请先用域名证书并按上文重跑脚本切换到 HTTPS 模式后再使用。

脚本成功的前提：GitHub Actions 已跑过一次并产出 `ghcr.io/oppo518/oci-manager:latest`。
首次推送代码后到仓库的 **Actions** 页确认构建成功（约 5–10 分钟，多架构）。
若仓库是私有的，VPS 上需先 `docker login ghcr.io`。

### 已有部署如何升级（重要）

如果你之前是用 `git clone` + `docker compose up -d --build` 部署的，
**不要直接跑默认的一键命令**，否则会在 `/opt/oci-manager` 另起一套空数据库。
正确做法是把安装目录指到原目录，让脚本复用已有的 `data/`、`.env` 与 compose：

```bash
cd ~/oci-manager && git pull        # 必须：新版 compose 不含 ports，改由 override 提供端口

curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh \
  -o /tmp/oci-install.sh
sudo OCI_MANAGER_DIR=/root/oci-manager bash /tmp/oci-install.sh
```

脚本会检测到已有数据库并打印「将完整保留账号、任务与证书」，同时沿用仓库里的
`docker-compose.yml`，不会覆盖造成 `git pull` 冲突；端口映射由脚本每次重写的
`docker-compose.override.yml` 决定（该文件由脚本生成，不要手工编辑或提交到 Git）。
`OCI_MANAGER_DIR` 请换成你实际的克隆目录（用 `pwd` 查看）。
**升级前务必先 `git pull`**：旧版 compose 自带 `ports`，会与 override 叠加，
导致「首次不映射 443 / 证书后关闭 HTTP」失效。

### 443 端口被占用（`address already in use`）

> **首次部署不会再遇到这个问题**：没有证书时脚本只映射你选的 HTTP 端口，
> 完全不映射 443，脚本也只预检 HTTP 端口。只有检测到已签发证书、重跑脚本切换为
> HTTPS 模式时，才会映射 `OCI_MANAGER_HTTPS_PORT`（默认 443）→ 容器 8443。

VPS 上如果已经跑了 nginx、宝塔面板或之前手动部署的旧容器，且已处于 HTTPS 模式，
重跑脚本时可能在最后一步报：

```
failed to bind host port 0.0.0.0:443/tcp: address already in use
```

先确认是谁占着端口（镜像和密钥此时已经下载/生成好了，不用重来）：

```bash
sudo ss -tlnp | grep -E ':(443|8080)\b'
docker ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
```

按实际情况三选一：

1. **停用占用者**：`sudo docker stop <容器名>` 或 `sudo systemctl stop nginx`，
   然后重新执行一键命令即可。
2. **保留该服务，把管理器换到 8443**（Cloudflare 代理支持 8443，DNS-01 申请证书不受影响）：

   ```bash
   curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh \
     -o /tmp/oci-install.sh
   sudo OCI_MANAGER_HTTPS_PORT=8443 bash /tmp/oci-install.sh
   ```

   此时浏览器需访问 `https://域名:8443`；若域名走 Cloudflare 代理，访客仍可用
   `https://域名`（CF 会回源 8443）。记得放行安全组的 8443。
   端口会写入 `.env`，之后直接重跑一键命令升级会一直沿用。
3. **之前手动 clone 部署过、想保留已录入的账号**：用上一节的 `OCI_MANAGER_DIR`
   指向原目录，不要直接用 `/opt/oci-manager` 里的新空库。

> 一键脚本已经包含登录鉴权、HTTPS 端口映射和目录权限修复。下面的章节是
> 手动部署 / 从源码构建的可选路径，以及排障参考。

---

## 一、上传到 GitHub

仓库里已经配置好 `.gitignore`，提交的是**源码**，不包含 `node_modules`、构建产物、
数据库与私钥。默认部署走预构建镜像，因此也不需要上传依赖。

当前已跟踪的均为源码文件，无需手工挑选。

### 1. 在 GitHub 上创建空仓库

打开 https://github.com/new ，仓库名填 `oci-manager`，
**不要勾选** Add README / .gitignore / LICENSE（本地已有，勾了会产生冲突）。

### 2. 在本机关联并推送

这台机器没有 `gh`，也没有 SSH 密钥，用 HTTPS + Personal Access Token 最简单。

**生成 Token**：GitHub → 右上角头像 → Settings → Developer settings →
Personal access tokens → Tokens (classic) → Generate new token，
勾选 `repo`（以及 `workflow` 如果用 Actions），生成后复制保存（只显示一次）。

```bash
cd oci-manager

git remote add origin https://github.com/<你的用户名>/oci-manager.git
git branch -M main
git push -u origin main
```

弹窗登录时：用户名填 GitHub 用户名，**密码栏粘贴上面生成的 Token**（不是登录密码）。

推送后验证：

```bash
git remote -v
git log --oneline -1
```

浏览器打开仓库页面，应当能看到全部源文件，且**没有** `web/node_modules`。

### 3. 推送前自检（强烈建议）

`.gitignore` 写错会导致文件在本地存在、却没进版本库，本地一切正常、VPS 构建才炸。
推送前用全新克隆验证一次，成本几秒钟：

```bash
cd /tmp && rm -rf _verify && git clone <你的仓库路径> _verify && cd _verify
go build ./...          # 能过说明推送内容完整
cd /tmp && rm -rf _verify
```

> 曾经踩过：`.gitignore` 里裸写 `server`，把服务入口 `cmd/server/` 一并忽略，
> 结果 VPS 上报 `stat /src/cmd/server: directory not found`。
> 目录名一定要写成 `/server` 锚定到根目录。

> 想省掉每次输 Token，可以配 SSH：
> `ssh-keygen -t ed25519 -C "you@example.com"`，把 `~/.ssh/id_ed25519.pub` 内容加到
> GitHub → Settings → SSH and GPG keys，然后把 remote 换成
> `git@github.com:<用户名>/oci-manager.git`。

---

## 二、在 VPS 上构建镜像

### 1. 安装 Docker（Ubuntu / Debian）

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker          # 或重新登录 shell
docker --version
```

国内机器若拉镜像慢，配置镜像加速：

```bash
sudo mkdir -p /etc/docker
sudo tee /etc/docker/daemon.json <<'EOF'
{
  "registry-mirrors": ["https://docker.m.daocloud.io"]
}
EOF
sudo systemctl restart docker
```

### 2. 拉取代码并生成密钥

```bash
git clone https://github.com/<你的用户名>/oci-manager.git
cd oci-manager

cp .env.example .env
# 写入随机密钥，务必保存好：录入账号后更改会导致已存账号无法解密
echo "OCI_MANAGER_ENCRYPTION_KEY=$(openssl rand -hex 32)" > .env
cat .env
```

### 3. 准备数据目录权限（必做，否则容器会无限重启）

容器以非 root 的 `app` 用户运行，而 `./data` 由 Docker 以 root 创建，
`app` 无写权限时 SQLite 打不开文件，容器会一直 `Restarting`：

```bash
mkdir -p data
sudo chown -R 100:101 data     # 实测 alpine:3.24 中 app 的 uid=100、gid=101
```

不确定 uid 时先构建一次镜像再查：

```bash
docker run --rm --entrypoint /bin/sh oci-manager:latest -c 'id app'
```

### 4. 构建并启动

> 基础 `docker-compose.yml` **不含 `ports`**（对外映射由一键脚本生成的
> `docker-compose.override.yml` 提供）。手动构建部署时请自行创建该文件，例如首次
> （HTTP 模式）：
>
> ```yaml
> # docker-compose.override.yml
> services:
>   oci-manager:
>     ports:
>       - "8080:8080"
> ```
>
> 证书签发后改成 `"443:8443"` 即关闭 HTTP、只保留 HTTPS。更简单的做法是直接跑
> 「零」的一键脚本，由它按证书状态自动切换。

```bash
docker compose up -d --build
```

首次构建约 5–10 分钟（要拉 node / golang 镜像并下载依赖），
后续只改代码时同样用这条命令，Docker 会复用缓存层。

查看状态与日志：

```bash
docker compose ps                 # 状态应为 Up (healthy)
docker compose logs -f            # 实时日志
docker compose logs --tail=100    # 最近 100 行
```

验证服务：

```bash
curl http://127.0.0.1:8080/api/health
# 返回 {"status":"ok"} 之类即正常
```

浏览器访问 `http://<VPS 公网 IP>:8080`，进入「账号」页面录入 OCI 凭证。

### 4. 更新部署

```bash
cd oci-manager
git pull
docker compose up -d --build
docker image prune -f             # 清理旧镜像，可选
```

---

## 三、安全加固（建议做）

面板能开关机、终止实例，虽然已有**登录鉴权 + 可选 Telegram 两步验证**，
首次部署的明文 HTTP 端口仍不应长期裸奔在公网。最省事的做法是直接在
「设置 → HTTPS 证书」里配置 Cloudflare DNS-01 域名证书，然后**重跑一键脚本**
切换到 HTTPS 模式（见「零」），或者用下面两种方案之一。

### 内置 HTTPS（Cloudflare DNS-01，推荐）

无需 Nginx、无需 80 端口、无需提前解析域名：

1. 域名的 DNS 托管在 Cloudflare；
2. 在 Cloudflare 创建 API Token，权限为 `Zone:DNS:Edit` + `Zone:Zone:Read`；
3. 面板 →「设置 → HTTPS 证书」填域名、邮箱、Token，点「启用 HTTPS」。

服务会自动通过 DNS-01 挑战申请 Let's Encrypt 证书并**热加载生效，无需重启容器**，
剩余有效期不足 30 天时自动续期。容器内 HTTPS 监听 `8443`（非 root 可绑定高位端口）。
**证书只支持 Cloudflare DNS-01 域名证书；不支持纯 IP 证书，不开放 80 端口，
不实现 HTTP-01 / TLS-ALPN-01 分支。Passkey 需要域名 + HTTPS。**

流程与注意事项：

- 首次部署只映射 HTTP 端口、不映射 443，此时用 `http://IP:端口` 进入面板配置证书。
- 证书热加载成功后，后端立即对明文 HTTP 业务请求返回 **403**（仅容器内
  `/api/health` 放行）。此时首次部署尚未映射 443，**请立即重跑同一条一键命令**：
  脚本检测到 `data/certs/fullchain.pem` 后切换为 HTTPS 模式，关闭宿主机 HTTP 端口、
  只映射 `443->8443`，之后访问 `https://你的域名`。
- 切换**不会自动发生**：端口映射只在用户主动重跑脚本时重建，后台签发 / 续期证书
  不会偷偷改端口把人锁在外面。
- 若来不及重跑而被 403 挡住，可临时在 `docker-compose.override.yml` 里补一条
  `"443:8443"` 映射并 `docker compose up -d` 恢复访问，再重跑脚本。
- 证书配置失败不会删除已有证书，也不会提前关闭 HTTP 端口。

启用后请放行云厂商安全组的 `443` 端口（若改用非标准端口，见「零」的
`OCI_MANAGER_HTTPS_PORT`）。

### 方案 A：只监听本地 + SSH 隧道（最省事，无需域名）

在 `docker-compose.override.yml`（由脚本生成；手动部署时自行创建）里把端口绑定改为仅本机：

```yaml
services:
  oci-manager:
    ports:
      - "127.0.0.1:8080:8080"
```

然后在本机建隧道：

```bash
ssh -L 8080:127.0.0.1:8080 user@<VPS IP>
```

浏览器访问 `http://localhost:8080` 即可，公网完全扫不到端口。

### 方案 B：Nginx + HTTPS + Basic Auth

> 这是「自己用 Nginx 反代」的可选方案，与内置 DNS-01 证书二选一。内置证书只用
> Cloudflare DNS-01，**不需要 80 端口**；本项目不为 80 端口 / HTTP-01 /
> TLS-ALPN-01 / 纯 IP 证书留任何分支。下面示例只监听 443，直接复用内置证书
> 签出的 `data/certs/fullchain.pem` 与 `privkey.pem`（也可换成自备证书）。

```bash
sudo apt install -y nginx apache2-utils
sudo htpasswd -c /etc/nginx/.htpasswd ociadmin
```

`/etc/nginx/sites-available/oci-manager`：

```nginx
server {
    listen 443 ssl;
    server_name your.domain.com;
    ssl_certificate     /opt/oci-manager/data/certs/fullchain.pem;
    ssl_certificate_key /opt/oci-manager/data/certs/privkey.pem;
    location / {
        proxy_pass https://127.0.0.1:8443;   # 反代到容器 HTTPS 端口
        proxy_ssl_verify off;                # 仅本机回源，容器证书为域名证书
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        auth_basic "OCI Manager";
        auth_basic_user_file /etc/nginx/.htpasswd;
    }
}
```

```bash
sudo ln -s /etc/nginx/sites-available/oci-manager /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

同时在 `docker-compose.override.yml` 里把容器 `8443` 映射到
`127.0.0.1:8443:8443`，只让本机 Nginx 能访问；不要再映射宿主机 HTTP 端口。
回源走 TLS，服务端看到的是 HTTPS 请求，不会被 403 中间件拒绝。

### 防火墙

```bash
sudo ufw allow 22/tcp
sudo ufw allow 8080/tcp   # 首次部署的 HTTP 端口（改成你选的端口）
sudo ufw allow 443/tcp    # 证书签发、重跑脚本切换为 HTTPS 模式后需要
sudo ufw enable
```

内置证书走 Cloudflare DNS-01，**不需要 80 端口**，请勿为 80 留例外；也无需
HTTP-01 / TLS-ALPN-01。首次部署只放行自定义 HTTP 端口；切换 HTTPS 后只放行
`443`（或 `OCI_MANAGER_HTTPS_PORT` 指定的端口），并关掉原 HTTP 端口。
（方案 A 不需要放行这些端口，只留 22。）

---

## 四、常见问题

| 现象 | 原因 / 处理 |
| --- | --- |
| 启动失败：`缺少 OCI_MANAGER_ENCRYPTION_KEY` 或密钥过短 | 新版不再提供默认密钥。执行 `openssl rand -hex 32`，把结果写入 `.env` 的 `OCI_MANAGER_ENCRYPTION_KEY` 后重启；**注意别覆盖原有密钥**，否则已录入的账号无法解密 |
| 升级后被退出登录，需要重新输一次密码 | 会话令牌改为摘要存储，旧会话作废。属预期行为 |
| 配置证书后 HTTP 访问返回 403 / 面板打不开 | 证书签发后后端对明文 HTTP 业务请求一律 403（仅容器内 `/api/health` 放行），不再 308 跳转；而首次部署尚未映射 443。请重跑同一条一键命令切换为 HTTPS 模式并用 `https://域名` 访问（见「零」）。Cloudflare Flexible 回源 HTTP 的场景需设 `OCI_MANAGER_ALLOW_HTTP=true`，或改用 Full SSL |
| 抢购任务日志出现「退避 … 后重试」 | OCI 返回 429 / 5xx / 超时，系统按设计暂停以避免触发风控，不是故障 |
| `go mod download` 卡住或超时 | 国内 VPS 需要 GOPROXY 镜像，compose 里已配 `goproxy.cn`；仍慢可临时改成 `https://goproxy.io,direct` |
| `npm ci` 报 lock 文件不匹配 | 本地改过 `package.json` 却没更新 `package-lock.json`，本机 `cd web && npm install` 后重新提交 |
| `golang:1.27-alpine` 拉取失败 | 该标签已确认存在；若仍拉不动多半是镜像源问题，换 `--build-arg GO_IMAGE=golang:1.27-bookworm`（Debian 版，体积大但源更稳） |
| `failed to solve: base name ($XXX) should not be blank` | `ARG` 写在第一个 `FROM` 之后属于阶段内作用域，无法用于后续 `FROM`。所有用于 `FROM` 的 `ARG` 必须声明在第一个 `FROM` 之前 |
| `stat /src/cmd/server: directory not found` | `.gitignore` 里裸写 `server` 把 `cmd/server/` 入口目录也忽略了，该文件没进版本库。改成 `/server`，然后 `git add cmd/server/main.go` 重新提交 |
| 一键脚本报 `failed to bind host port 0.0.0.0:443/tcp: address already in use` | **仅 HTTPS 模式才会映射 443；首次部署完全不映射 443，不会遇到**。若已签发证书，443 被 nginx / 宝塔 / 旧容器占用：见「零、一键部署 → 443 端口被占用」，停用占用者，或改用 `sudo OCI_MANAGER_HTTPS_PORT=8443 bash /tmp/oci-install.sh` |
| 一键脚本最后报「健康检查未通过」，日志里 `HEAD "/api/health"` 全是 404 | `/api/health` 原先只注册 GET，而 healthcheck 的 `wget --spider` 发 HEAD。现已改为 GET 探测并给后端补了 HEAD 路由。另外宿主机全局 `http_proxy` 会把 `curl 127.0.0.1` 代理走，用 `curl --noproxy '*' -i http://127.0.0.1:8080/api/health` 验证。只要日志出现「OCI 管理器已启动」，服务就可用，健康检查失败不影响访问 |
| 容器一直 `Restarting`，日志报 `unable to open database file (14)` | `./data` 属主是 root，容器内的 `app` 用户写不进去。执行 `sudo chown -R 100:101 data` 后 `docker compose up -d`（一键脚本已自动处理） |
| 容器反复重启 | `docker compose logs` 看报错；最常见是没设 `OCI_MANAGER_ENCRYPTION_KEY` |
| 一键脚本报「拉取失败」 | Actions 还没产出镜像，或仓库为私有。到 Actions 页确认构建成功；私有仓库先 `docker login ghcr.io` |
| GHCR 拉取很慢 | 配置 Docker 镜像加速；或改用本地构建：`git clone` 后 `docker compose up -d --build` |
| HTTPS 申请证书失败 | 确认域名 DNS 托管在 Cloudflare、Token 有 `Zone:DNS:Edit` / `Zone:Zone:Read`；国内 VPS 若 DNS 传播检查失败可稍后重试。错误详情会显示在设置页 |
| 证书已签发但 `https://域名` 打不开 | 先确认**已重跑一键脚本切换为 HTTPS 模式**（首次部署不映射 443）；放行安全组 443；确认域名已解析到 VPS（DNS-01 不要求申请前解析，但访问时需要） |
| 容量探测全部显示「状态未知」或报 403 | 该账户无权使用 Compute Capacity Report（需 tenancy 级读取权限）。抢购任务会自动降级为直接创建，也可在「设置 → 抢购策略」关闭预检 |
| 容量状态显示「有库存」但创建仍失败 | 容量报告是准实时快照，资源可能在两次调用之间被抢走；属正常现象，任务会继续下一轮 |
| 端口不通 | 首次部署只映射自定义 HTTP 端口（放行它即可，不映射 443）；切换 HTTPS 后只放行 443（或 `OCI_MANAGER_HTTPS_PORT`）。检查 `ufw` 与云厂商安全组 |
| 配置证书后立刻被 403 挡住、面板进不去 | 证书热加载后 HTTP 立即 403，而首次部署还没映射 443。重跑同一条一键命令即完成切换；急用可临时在 `docker-compose.override.yml` 里加 `"443:8443"` 再 `docker compose up -d` |
| 想用纯 IP 上 HTTPS / 不想要域名 | **纯 IP 证书不支持**，证书只允许 Cloudflare DNS-01 域名证书，不开放 80 / HTTP-01 / TLS-ALPN-01；Passkey 需域名 + HTTPS。首次可用 `http://IP:自定义端口`，但仅建议用于配置证书 |
| 首次部署能否指定端口、升级会不会被改回默认 | 能：`sudo OCI_MANAGER_PORT=8088 bash /tmp/oci-install.sh`（管道/非交互）或本地真终端交互输入；端口写入 `.env`，重跑不带参数会沿用 |
| 数据在哪 | `./data/oci-manager.db` 与 `./data/certs/fullchain.pem` + `privkey.pem`（证书），备份整个 `data` 目录；不要把 `data/` 提交到 Git。一键脚本据 `data/certs/fullchain.pem` 是否存在判定 HTTP / HTTPS 模式 |

## 五、备份

```bash
# 在 VPS 上
docker compose exec oci-manager sh -c 'echo' >/dev/null 2>&1
tar czf oci-manager-backup-$(date +%F).tar.gz data .env
```

数据库里存的是**加密后的私钥**，但备份文件仍应妥善保管。
