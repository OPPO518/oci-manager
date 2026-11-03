# 部署文档：GitHub → VPS → Docker

## 一、上传到 GitHub

仓库里已经配置好 `.gitignore`，提交的是**源码**，不包含 `node_modules`、构建产物、
数据库与私钥。前端在 VPS 上构建镜像时现场编译，所以不需要上传依赖。

当前已跟踪 48 个文件，无需手工挑选。

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

浏览器打开仓库页面，应当能看到 48 个文件，且**没有** `web/node_modules`。

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

### 3. 构建并启动

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

直接把 8080 暴露到公网风险较高——这个面板能开关机、终止实例，
且目前没有登录鉴权。至少做下面两件事之一。

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
| 容器反复重启 | `docker compose logs` 看报错；最常见是没设 `OCI_MANAGER_ENCRYPTION_KEY` |
| 端口不通 | 检查 `ufw`、云厂商安全组是否放行 8080 |
| 数据在哪 | `./data/oci-manager.db`，备份整个 `data` 目录即可；不要把 `data/` 提交到 Git |

## 五、备份

```bash
# 在 VPS 上
docker compose exec oci-manager sh -c 'echo' >/dev/null 2>&1
tar czf oci-manager-backup-$(date +%F).tar.gz data .env
```

数据库里存的是**加密后的私钥**，但备份文件仍应妥善保管。
