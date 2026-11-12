# 部署文档：GitHub → VPS → Docker

## 零、一键部署（推荐，只需一条命令）

镜像由 GitHub Actions 预先构建并发布到 GHCR，VPS 上**不需要编译前端和 Go 代码**，
也不需要 clone 仓库、不需要手动生成密钥、不需要处理目录权限：

```bash
curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh | sudo bash
```

脚本会自动完成：安装 Docker → 生成随机加密密钥（保存到 `/opt/oci-manager/.env`）→
写入 compose 配置 → 拉取最新镜像 → 修正 `data` 目录属主 → 启动并做健康检查 →
打印访问地址。

- 默认 HTTP 端口 `8080`，HTTPS 端口 `443`（映射到容器 `8443`）。
- **重复执行同一条命令即为升级**：只拉新镜像并重启，不会覆盖 `.env`、数据库和证书。
- 可用环境变量覆盖：`OCI_MANAGER_REPO` / `OCI_MANAGER_IMAGE` / `OCI_MANAGER_DIR` /
  `OCI_MANAGER_PORT` / `OCI_MANAGER_HTTPS_PORT`。

脚本成功的前提：GitHub Actions 已跑过一次并产出 `ghcr.io/oppo518/oci-manager:latest`。
首次推送代码后到仓库的 **Actions** 页确认构建成功（约 5–10 分钟，多架构）。
若仓库是私有的，VPS 上需先 `docker login ghcr.io`。

### 已有部署如何升级（重要）

如果你之前是用 `git clone` + `docker compose up -d --build` 部署的，
**不要直接跑默认的一键命令**，否则会在 `/opt/oci-manager` 另起一套空数据库。
正确做法是把安装目录指到原目录，让脚本复用已有的 `data/`、`.env` 与 compose：

```bash
cd ~/oci-manager && git pull        # 先更新 compose（新增 GHCR 镜像与 443 映射）

curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh \
  -o /tmp/oci-install.sh
sudo OCI_MANAGER_DIR=/root/oci-manager bash /tmp/oci-install.sh
```

脚本会检测到已有数据库并打印「将完整保留账号、任务与证书」，同时沿用仓库里的
`docker-compose.yml`，不会覆盖造成 `git pull` 冲突。
`OCI_MANAGER_DIR` 请换成你实际的克隆目录（用 `pwd` 查看）。

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
仍建议不要直接把 8080 裸奔在公网。最省事的做法是直接在「设置 → HTTPS 证书」
里配置证书（见下），或者用下面两种方案之一。

### 内置 HTTPS（Cloudflare DNS-01，推荐）

无需 Nginx、无需 80 端口、无需提前解析域名：

1. 域名的 DNS 托管在 Cloudflare；
2. 在 Cloudflare 创建 API Token，权限为 `Zone:DNS:Edit` + `Zone:Zone:Read`；
3. 面板 →「设置 → HTTPS 证书」填域名、邮箱、Token，点「启用 HTTPS」。

服务会自动通过 DNS-01 挑战申请 Let's Encrypt 证书并**热加载生效，无需重启容器**，
剩余有效期不足 30 天时自动续期。容器内 HTTPS 监听 `8443`（非 root 可绑定高位端口），
由 compose 映射到宿主机 `443`，因此访问 `https://你的域名` 即可。HTTP 端口始终保留，
证书配置失败也不会把自己锁在外面。

启用后请放行云厂商安全组的 `443` 端口。

### 方案 A：只监听本地 + SSH 隧道（最省事，无需域名）

改 `docker-compose.yml` 的端口映射为仅本机：

```yaml
    ports:
      - "127.0.0.1:8080:8080"
```

然后在本机建隧道：

```bash
ssh -L 8080:127.0.0.1:8080 user@<VPS IP>
```

浏览器访问 `http://localhost:8080` 即可，公网完全扫不到端口。

### 方案 B：Nginx + HTTPS + Basic Auth

```bash
sudo apt install -y nginx apache2-utils
sudo htpasswd -c /etc/nginx/.htpasswd ociadmin
```

`/etc/nginx/sites-available/oci-manager`：

```nginx
server {
    listen 80;
    server_name your.domain.com;
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        auth_basic "OCI Manager";
        auth_basic_user_file /etc/nginx/.htpasswd;
    }
}
```

```bash
sudo ln -s /etc/nginx/sites-available/oci-manager /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
sudo certbot --nginx -d your.domain.com     # 申请证书
```

同时把 compose 的端口改成 `127.0.0.1:8080:8080`，只让 Nginx 能访问。

### 防火墙

```bash
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

（方案 A 不需要放行 80/443，只留 22。）

---

## 四、常见问题

| 现象 | 原因 / 处理 |
| --- | --- |
| `go mod download` 卡住或超时 | 国内 VPS 需要 GOPROXY 镜像，compose 里已配 `goproxy.cn`；仍慢可临时改成 `https://goproxy.io,direct` |
| `npm ci` 报 lock 文件不匹配 | 本地改过 `package.json` 却没更新 `package-lock.json`，本机 `cd web && npm install` 后重新提交 |
| `golang:1.27-alpine` 拉取失败 | 该标签已确认存在；若仍拉不动多半是镜像源问题，换 `--build-arg GO_IMAGE=golang:1.27-bookworm`（Debian 版，体积大但源更稳） |
| `failed to solve: base name ($XXX) should not be blank` | `ARG` 写在第一个 `FROM` 之后属于阶段内作用域，无法用于后续 `FROM`。所有用于 `FROM` 的 `ARG` 必须声明在第一个 `FROM` 之前 |
| `stat /src/cmd/server: directory not found` | `.gitignore` 里裸写 `server` 把 `cmd/server/` 入口目录也忽略了，该文件没进版本库。改成 `/server`，然后 `git add cmd/server/main.go` 重新提交 |
| 容器一直 `Restarting`，日志报 `unable to open database file (14)` | `./data` 属主是 root，容器内的 `app` 用户写不进去。执行 `sudo chown -R 100:101 data` 后 `docker compose up -d`（一键脚本已自动处理） |
| 容器反复重启 | `docker compose logs` 看报错；最常见是没设 `OCI_MANAGER_ENCRYPTION_KEY` |
| 一键脚本报「拉取失败」 | Actions 还没产出镜像，或仓库为私有。到 Actions 页确认构建成功；私有仓库先 `docker login ghcr.io` |
| GHCR 拉取很慢 | 配置 Docker 镜像加速；或改用本地构建：`git clone` 后 `docker compose up -d --build` |
| HTTPS 申请证书失败 | 确认域名 DNS 托管在 Cloudflare、Token 有 `Zone:DNS:Edit` / `Zone:Zone:Read`；国内 VPS 若 DNS 传播检查失败可稍后重试。错误详情会显示在设置页 |
| 证书已签发但 `https://域名` 打不开 | 放行安全组 443；确认域名已解析到 VPS（DNS-01 不要求申请前解析，但访问时需要） |
| 容量探测全部显示「状态未知」或报 403 | 该账户无权使用 Compute Capacity Report（需 tenancy 级读取权限）。抢购任务会自动降级为直接创建，也可在「设置 → 抢购策略」关闭预检 |
| 容量状态显示「有库存」但创建仍失败 | 容量报告是准实时快照，资源可能在两次调用之间被抢走；属正常现象，任务会继续下一轮 |
| 端口不通 | 检查 `ufw`、云厂商安全组是否放行 8080（HTTP）与 443（HTTPS） |
| 数据在哪 | `./data/oci-manager.db` 与 `./data/certs/`（证书），备份整个 `data` 目录；不要把 `data/` 提交到 Git |

## 五、备份

```bash
# 在 VPS 上
docker compose exec oci-manager sh -c 'echo' >/dev/null 2>&1
tar czf oci-manager-backup-$(date +%F).tar.gz data .env
```

数据库里存的是**加密后的私钥**，但备份文件仍应妥善保管。
