#!/usr/bin/env bash
# OCI 管理器一键部署 / 升级脚本
#
# 用法（Debian / Ubuntu / CentOS / Rocky，amd64 与 arm64 均可）：
#   curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh | sudo bash
#
# 重复执行即为升级：只拉取最新镜像并重启容器，不会覆盖 .env、数据库和证书。
#
# 可覆盖的环境变量：
#   OCI_MANAGER_REPO        仓库 owner/name，默认 OPPO518/oci-manager
#   OCI_MANAGER_IMAGE       镜像地址，默认 ghcr.io/<repo>:latest
#   OCI_MANAGER_DIR         安装目录，默认 /opt/oci-manager
#   OCI_MANAGER_PORT        HTTP 端口，默认 8080
#   OCI_MANAGER_HTTPS_PORT  宿主机 HTTPS 端口，默认 443
set -euo pipefail

REPO="${OCI_MANAGER_REPO:-OPPO518/oci-manager}"
REPO_LC="$(printf '%s' "$REPO" | tr '[:upper:]' '[:lower:]')"
IMAGE="${OCI_MANAGER_IMAGE:-ghcr.io/${REPO_LC}:latest}"
DIR="${OCI_MANAGER_DIR:-/opt/oci-manager}"
HTTP_PORT="${OCI_MANAGER_PORT:-8080}"
HTTPS_PORT="${OCI_MANAGER_HTTPS_PORT:-443}"

info() { printf '\033[1;32m[信息]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[注意]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[失败]\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || fail "请用 root 运行（命令前加 sudo）"

# ---------- 1. Docker ----------
if ! command -v docker >/dev/null 2>&1; then
  info "未检测到 Docker，正在安装…"
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker >/dev/null 2>&1 || true
else
  info "Docker 已安装：$(docker --version)"
fi

if docker compose version >/dev/null 2>&1; then
  COMPOSE="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE="docker-compose"
else
  fail "缺少 Docker Compose 插件，请升级 Docker 或手动安装 docker-compose-plugin"
fi

# ---------- 2. 目录与密钥 ----------
mkdir -p "$DIR/data"
if [ ! -f "$DIR/.env" ]; then
  KEY="$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  cat >"$DIR/.env" <<EOF
# 用于加密数据库中保存的 OCI API 私钥。请妥善备份，丢失后已录入的账号将无法解密。
OCI_MANAGER_ENCRYPTION_KEY=$KEY
EOF
  chmod 600 "$DIR/.env"
  info "已生成加密密钥并保存到 $DIR/.env"
else
  info "沿用已有的 .env（不覆盖，避免已保存账号无法解密）"
fi

# ---------- 3. compose 文件（内嵌，无需再下载） ----------
cat >"$DIR/docker-compose.yml.tpl" <<'EOF'
services:
  oci-manager:
    image: __IMAGE__
    container_name: oci-manager
    restart: unless-stopped
    ports:
      - "__HTTP_PORT__:8080"
      - "__HTTPS_PORT__:8443"
    volumes:
      - ./data:/data
    environment:
      OCI_MANAGER_ADDR: ":8080"
      OCI_MANAGER_HTTPS_ADDR: ":8443"
      OCI_MANAGER_DB: "/data/oci-manager.db"
      OCI_MANAGER_ENCRYPTION_KEY: ${OCI_MANAGER_ENCRYPTION_KEY:?请先在 .env 中设置 OCI_MANAGER_ENCRYPTION_KEY}
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://127.0.0.1:8080/api/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
EOF

sed -e "s|__IMAGE__|$IMAGE|g" \
    -e "s|__HTTP_PORT__|$HTTP_PORT|g" \
    -e "s|__HTTPS_PORT__|$HTTPS_PORT|g" \
    "$DIR/docker-compose.yml.tpl" >"$DIR/docker-compose.yml"
rm -f "$DIR/docker-compose.yml.tpl"

# ---------- 4. 拉取镜像 ----------
info "正在拉取镜像 $IMAGE …"
if ! (cd "$DIR" && $COMPOSE pull); then
  warn "拉取失败。若仓库是私有仓库，请先执行：docker login ghcr.io"
  fail "无法获取镜像，部署中止。也可改用本地构建：git clone 后 docker compose up -d --build"
fi

# ---------- 5. 修正数据目录属主（容器内 app 用户非 root） ----------
# 绑定挂载时宿主机目录属主优先，若属主是 root，容器内 SQLite 会因无写权限
# 报 "unable to open database file (14)"，导致容器无限重启。
UID_VAL="$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c 'id -u app' 2>/dev/null || echo 100)"
GID_VAL="$(docker run --rm --entrypoint /bin/sh "$IMAGE" -c 'id -g app' 2>/dev/null || echo 101)"
UID_VAL="$(printf '%s' "$UID_VAL" | tr -cd '0-9')"
GID_VAL="$(printf '%s' "$GID_VAL" | tr -cd '0-9')"
[ -n "$UID_VAL" ] || UID_VAL=100
[ -n "$GID_VAL" ] || GID_VAL=101
chown -R "${UID_VAL}:${GID_VAL}" "$DIR/data"
info "数据目录属主已设为 ${UID_VAL}:${GID_VAL}"

# ---------- 6. 启动 ----------
info "正在启动服务…"
(cd "$DIR" && $COMPOSE up -d)

# ---------- 7. 健康检查 ----------
info "等待服务就绪…"
OK=0
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${HTTP_PORT}/api/health" >/dev/null 2>&1; then
    OK=1
    break
  fi
  sleep 2
done

if [ "$OK" -ne 1 ]; then
  (cd "$DIR" && $COMPOSE logs --tail=30) || true
  fail "服务未能在 60 秒内就绪，请查看上方日志"
fi

IP="$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || echo '<服务器IP>')"
cat <<EOF

$(printf '\033[1;32m')
╔══════════════════════════════════════════════════════════════╗
║              OCI 管理器部署完成                               ║
╚══════════════════════════════════════════════════════════════╝
$(printf '\033[0m')
  访问地址： http://${IP}:${HTTP_PORT}
  安装目录： ${DIR}
  配置文件： ${DIR}/.env（内含加密密钥，请备份）
  HTTPS：    容器内 8443 已映射到宿主机 ${HTTPS_PORT}，
             在面板「设置 → HTTPS 证书」配置后即可用 https://域名 访问

  常用命令：
    cd ${DIR} && ${COMPOSE} ps              查看状态
    cd ${DIR} && ${COMPOSE} logs -f         查看日志
    cd ${DIR} && ${COMPOSE} restart         重启
    curl -fsSL .../install.sh | sudo bash    再次执行即为升级

  首次打开请设置管理员账号，然后在「账号」页录入 OCI API 凭证。
  安全提示：请放行云厂商安全组的 ${HTTP_PORT} 与 ${HTTPS_PORT} 端口，
            并尽快在「设置」里启用 Telegram 两步验证。
EOF
