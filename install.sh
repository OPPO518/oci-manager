#!/usr/bin/env bash
# OCI 管理器一键部署 / 升级脚本
#
# 用法（Debian / Ubuntu / CentOS / Rocky，amd64 与 arm64 均可）：
#   curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh -o /tmp/oci-install.sh
#   sudo bash /tmp/oci-install.sh
#   （真终端且未用环境变量指定端口时，会交互询问 HTTP 端口，直接回车用默认值）
#   curl -fsSL .../install.sh | sudo bash         # 纯管道，不交互
#
# 重复执行即为升级：只拉取最新镜像并重启容器，不会覆盖加密密钥、数据库和证书。
# 端口写入 $DIR/.env 持久化，升级时沿用（除非显式用环境变量覆盖）。
#
# 两种部署模式（按 data/certs/fullchain.pem 是否存在判定，仅在重跑脚本时切换）：
#   · 首次部署（无证书）：只映射自定义 HTTP 端口 -> 容器 8080，完全不映射 443。
#     没有证书，也避开 443 被 nginx / 面板 / 旧容器占用导致部署失败。
#   · 证书已签发：宿主机不再暴露 HTTP 端口，只映射 HTTPS 端口 -> 容器 8443，
#     并提示 HTTP 已关闭。切换不会自动发生，必须用户主动重跑本脚本。
#
# 可覆盖的环境变量：
#   OCI_MANAGER_REPO        仓库 owner/name，默认 OPPO518/oci-manager
#   OCI_MANAGER_IMAGE       镜像地址，默认 ghcr.io/<repo>:latest
#   OCI_MANAGER_DIR         安装目录，默认 /opt/oci-manager
#   OCI_MANAGER_PORT        HTTP 端口，默认 8080（非交互/管道方式用它指定）
#   OCI_MANAGER_HTTPS_PORT  宿主机 HTTPS 端口，默认 443（写入 .env 持久化）
#
# 首次部署自定义 HTTP 端口（管道方式，绝不进入交互）：
#   curl -fsSL https://raw.githubusercontent.com/OPPO518/oci-manager/main/install.sh -o /tmp/oci-install.sh
#   sudo OCI_MANAGER_PORT=8088 bash /tmp/oci-install.sh
# 证书签发生效后，重跑同一条命令即关闭宿主机 HTTP 端口、只保留 HTTPS。
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

# 端口必须是 1-65535 的纯数字（交互输入、环境变量与 .env 保存值统一校验）。
validate_port() {
  local p="$1"
  case "$p" in
    ''|*[!0-9]*) return 1 ;;
  esac
  [ "${#p}" -le 5 ] || return 1
  [ "$((10#$p))" -ge 1 ] && [ "$((10#$p))" -le 65535 ]
}

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
if [ -f "$DIR/.env" ]; then
  info "沿用已有的 .env（不覆盖，避免已保存账号无法解密）"
else
  KEY="$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  cat >"$DIR/.env" <<EOF
# 用于加密数据库中保存的 OCI API 私钥。请妥善备份，丢失后已录入的账号将无法解密。
OCI_MANAGER_ENCRYPTION_KEY=$KEY
EOF
  chmod 600 "$DIR/.env"
  info "已生成加密密钥并保存到 $DIR/.env"
fi

if [ -f "$DIR/data/oci-manager.db" ]; then
  info "检测到已有数据库，本次升级将完整保留账号、任务与证书"
fi

# 端口优先级：命令行环境变量 > 已有 .env 中保存的值 > 默认 8080/443。
# 否则改过端口的部署再次执行一键命令时，会被悄悄改回 443 导致启动失败。
env_value() {
  sed -n "s/^$1=//p" "$DIR/.env" 2>/dev/null | tail -n1 | tr -d '\r' | sed -e "s/^[\"']//" -e "s/[\"']$//"
}
if [ -z "${OCI_MANAGER_PORT+x}" ]; then
  SAVED="$(env_value OCI_MANAGER_PORT)"
  if [ -n "$SAVED" ]; then HTTP_PORT="$SAVED"; fi
fi
if [ -z "${OCI_MANAGER_HTTPS_PORT+x}" ]; then
  SAVED="$(env_value OCI_MANAGER_HTTPS_PORT)"
  if [ -n "$SAVED" ]; then HTTPS_PORT="$SAVED"; fi
fi

# ---------- 2b. 判定部署模式（HTTP / HTTPS） ----------
# 证书实际落盘位置（已核对源码，不猜测）：
#   internal/tlsmanager/tlsmanager.go：certFile = "fullchain.pem"、keyFile = "privkey.pem"
#   cmd/server/main.go：证书目录 = filepath.Join(filepath.Dir(DBPath), "certs")
#   compose：DBPath=/data/oci-manager.db、卷 ./data:/data
# 因此容器内为 /data/certs/{fullchain,privkey}.pem，宿主机为 $DIR/data/certs/。
CERT_FILE="$DIR/data/certs/fullchain.pem"
KEY_FILE="$DIR/data/certs/privkey.pem"
if [ -f "$CERT_FILE" ] && [ -f "$KEY_FILE" ]; then
  HTTPS_MODE=1
  info "检测到已签发证书（$CERT_FILE），本次按 HTTPS 模式部署：宿主机不再暴露 HTTP 端口"
else
  HTTPS_MODE=0
  info "未检测到证书，按首次部署（HTTP 模式）：只映射 HTTP 端口 ${HTTP_PORT}，完全不映射 443"
fi

# 交互式询问 HTTP 端口：仅「首次部署（HTTP 模式）」+ 真终端 + 未通过环境变量
# 显式指定时。纯管道（curl | sudo bash）没有控制终端，[ -t 0 ] 为假，绝不进入
# read；HTTPS 模式（证书已签发）同样不询问，保证「重跑同一条命令关闭 HTTP」
# 不会被交互卡住。默认值就是上面解析出的端口（升级时为 .env 里已保存的值），
# 直接回车即保留，不会被改回默认。
if [ "$HTTPS_MODE" -eq 0 ] && [ -t 0 ] && [ -z "${OCI_MANAGER_PORT+x}" ]; then
  printf '请输入首次部署的 HTTP 访问端口（1-65535，直接回车用默认 %s）：' "$HTTP_PORT"
  IFS= read -r INPUT_PORT || INPUT_PORT=""
  if [ -n "$INPUT_PORT" ]; then
    if ! validate_port "$INPUT_PORT"; then
      fail "端口必须是 1-65535 的纯数字：${INPUT_PORT}"
    fi
    HTTP_PORT="$((10#$INPUT_PORT))"
  fi
  printf '\n'
fi

if ! validate_port "$HTTP_PORT"; then
  fail "HTTP 端口非法（$HTTP_PORT）：必须是 1-65535 的纯数字"
fi
if ! validate_port "$HTTPS_PORT"; then
  fail "HTTPS 端口非法（$HTTPS_PORT）：必须是 1-65535 的纯数字"
fi

# 把最终端口写回 .env：以后直接重跑一键命令（不带参数）也不会被改回 443。
# 用同目录临时文件 + mv 替换，避免破坏密钥行、注释和 600 权限。
upsert_env() {
  local key="$1" val="$2" file="$DIR/.env" tmp
  if grep -q "^${key}=" "$file" 2>/dev/null; then
    tmp="$(mktemp "${file}.XXXXXX")"
    awk -v k="$key" -v v="$val" '$0 ~ "^" k "=" { print k "=" v; next } { print }' "$file" >"$tmp"
    chmod 600 "$tmp"
    mv "$tmp" "$file"
  else
    if ! grep -q '^# 宿主机端口' "$file" 2>/dev/null; then
      printf '\n# 宿主机端口（由一键脚本写入，修改后重跑脚本即可生效）\n' >>"$file"
    fi
    printf '%s=%s\n' "$key" "$val" >>"$file"
    chmod 600 "$file"
  fi
}
upsert_env OCI_MANAGER_PORT "$HTTP_PORT"
upsert_env OCI_MANAGER_HTTPS_PORT "$HTTPS_PORT"

# 导出给 docker compose：端口和镜像同样由这些变量决定
export OCI_MANAGER_IMAGE="$IMAGE" OCI_MANAGER_PORT="$HTTP_PORT" OCI_MANAGER_HTTPS_PORT="$HTTPS_PORT"

# 已有 git 仓库时沿用它的 compose 文件，避免覆盖后 git pull 冲突。
# 该 compose 默认的镜像、端口与脚本一致，可以直接 pull + up。
REUSE_COMPOSE=0
if [ -d "$DIR/.git" ] && [ -f "$DIR/docker-compose.yml" ]; then
  REUSE_COMPOSE=1
  info "检测到已有仓库，沿用现有 docker-compose.yml"
fi

# ---------- 3. compose 文件（内嵌，无需再下载） ----------
if [ "$REUSE_COMPOSE" -eq 0 ]; then
cat >"$DIR/docker-compose.yml.tpl" <<'EOF'
services:
  oci-manager:
    image: __IMAGE__
    container_name: oci-manager
    restart: unless-stopped
    # 基础 compose 不含 ports：对外映射统一由脚本生成的
    # docker-compose.override.yml 按 HTTP / HTTPS 模式提供。
    volumes:
      - ./data:/data
    environment:
      OCI_MANAGER_ADDR: ":8080"
      OCI_MANAGER_HTTPS_ADDR: ":8443"
      OCI_MANAGER_DB: "/data/oci-manager.db"
      OCI_MANAGER_PUBLIC_HTTPS_PORT: "__HTTPS_PORT__"
      OCI_MANAGER_ENCRYPTION_KEY: ${OCI_MANAGER_ENCRYPTION_KEY:?请先在 .env 中设置 OCI_MANAGER_ENCRYPTION_KEY}
    healthcheck:
      # GET 探测：--spider 发 HEAD，/api/health 只注册了 GET，会被判 404
      test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/api/health"]
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
fi

if [ "$REUSE_COMPOSE" -eq 1 ] && grep -qE '^[[:space:]]*ports:' "$DIR/docker-compose.yml"; then
  warn "$DIR/docker-compose.yml 仍是旧版（自带 ports 段），端口会与 override 叠加。"
  warn "请先执行：cd $DIR && git pull，更新为不含 ports 的新版 compose 后重跑本脚本。"
fi

# 生成端口映射 override：docker compose 自动加载同目录的
# docker-compose.override.yml。基础 compose 不含 ports，故这里的 ports 就是
# 最终对外映射，不会叠加出多余的 443。
# 注意：该文件只在脚本运行时重建，后台热签发证书不会触发重建，因此模式
# 切换只发生在用户主动重跑脚本时，不会自动把人锁在外面。
if [ "$HTTPS_MODE" -eq 1 ]; then
cat >"$DIR/docker-compose.override.yml" <<'OVERRIDE'
# 由 install.sh 生成，请勿手工编辑或提交到 Git。
# HTTPS 模式：证书已签发，宿主机不暴露 HTTP，仅映射 HTTPS 端口到容器 8443。
# OCI_MANAGER_HTTP_DISABLED=1 让后端在此模式下拒绝明文 HTTP 业务请求；
# 未重跑脚本的旧 HTTP 模式不带这个变量，证书刚签发时不会被锁在门外。
services:
  oci-manager:
    ports:
      - "${OCI_MANAGER_HTTPS_PORT}:8443"
    environment:
      OCI_MANAGER_HTTP_DISABLED: "1"
OVERRIDE
else
cat >"$DIR/docker-compose.override.yml" <<'OVERRIDE'
# 由 install.sh 生成，请勿手工编辑或提交到 Git。
# 首次部署（HTTP 模式）：只映射自定义 HTTP 端口到容器 8080，完全不映射 443。
services:
  oci-manager:
    ports:
      - "${OCI_MANAGER_PORT}:8080"
OVERRIDE
fi

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
chmod 700 "$DIR/data"
info "数据目录属主已设为 ${UID_VAL}:${GID_VAL}，权限收紧为 700"

# ---------- 6. 端口预检 ----------
# 只预检本次真正会映射的宿主机端口：HTTP 模式完全不碰 443（首次部署即使 443
# 被 nginx / 面板 / 旧容器占用也不会失败），HTTPS 模式不碰 HTTP_PORT。
# 升级时端口由本项目旧容器占着属于正常情况（compose up 会重建容器），只拦首次安装。
# 不预检的话 Docker 只抛一句 "address already in use"，很难定位是谁占了端口。
if [ "$HTTPS_MODE" -eq 1 ]; then
  CHECK_PORTS=("$HTTPS_PORT")
  PORT_HINT="OCI_MANAGER_HTTPS_PORT"
else
  CHECK_PORTS=("$HTTP_PORT")
  PORT_HINT="OCI_MANAGER_PORT"
fi

listener_info() {
  local p="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -tlnp 2>/dev/null | awk -v want="$p" 'NR>1 { n=split($4,a,":"); if (a[n]==want) print }'
  elif command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$p" -sTCP:LISTEN 2>/dev/null
  elif (exec 3<>"/dev/tcp/127.0.0.1/$p") 2>/dev/null; then
    printf '(本机 %s 端口有服务在监听，未安装 ss/lsof，无法显示进程名)' "$p"
  fi
  return 0
}

OWN_RUNNING=0
if (cd "$DIR" && $COMPOSE ps -q oci-manager 2>/dev/null | grep -q .); then
  OWN_RUNNING=1
fi

if [ "$OWN_RUNNING" -eq 0 ]; then
  for P in "${CHECK_PORTS[@]}"; do
    INFO="$(listener_info "$P")"
    if [ -n "$INFO" ]; then
      {
        printf '\n'
        warn "宿主机端口 ${P} 已被占用，容器无法绑定："
        printf '%s\n' "$INFO" | sed 's/^/      /'
        printf '\n'
        printf '  可选处理（任选其一，然后重新执行一键命令）：\n'
        printf '  1) 不再需要占用端口的服务，停掉它：\n'
        printf '       sudo ss -tlnp | grep :%s\n' "$P"
        printf '       sudo systemctl stop nginx        # 或停用旧容器：docker stop <容器名>\n'
        printf '  2) 要保留该服务，把管理器换到别的端口：\n'
        printf '       curl -fsSL https://raw.githubusercontent.com/%s/main/install.sh -o /tmp/oci-install.sh\n' "$REPO"
        printf '       sudo %s=<新端口> bash /tmp/oci-install.sh\n' "$PORT_HINT"
        printf '  3) 之前手动部署过本项目、要保留已录入的账号数据：\n'
        printf '       sudo OCI_MANAGER_DIR=/root/oci-manager bash /tmp/oci-install.sh\n'
        printf '\n'
      } >&2
      fail "端口 ${P} 冲突，部署中止（镜像、密钥与数据目录均已就绪，处理后重跑即可）"
    fi
  done
fi

# ---------- 7. 启动 ----------
info "正在启动服务…"
(cd "$DIR" && $COMPOSE up -d)

# ---------- 8. 健康检查 ----------
# 两条探测路径，任一成功即视为就绪：
#   1) 容器内 GET：绕过宿主机端口映射、防火墙与代理，最可靠；
#      HTTPS 模式下宿主机不暴露 HTTP 端口，健康检查完全依赖这一条。
#   2) 宿主机 GET：加 --noproxy 避免全局 http_proxy 把 127.0.0.1 也代理走。
# 注意 wget --spider 发的是 HEAD，而 /api/health 只注册了 GET，不能用它判定。
probe_health() {
  local body
  body="$(cd "$DIR" && $COMPOSE exec -T oci-manager \
    wget -q -O - http://127.0.0.1:8080/api/health 2>/dev/null || true)"
  case "$body" in
    *'"status":"ok"'*) return 0 ;;
  esac
  body="$(curl --noproxy '*' -fsS --max-time 3 \
    "http://127.0.0.1:${HTTP_PORT}/api/health" 2>/dev/null || true)"
  case "$body" in
    *'"status":"ok"'*) return 0 ;;
  esac
  return 1
}

info "等待服务就绪…"
OK=0
for _ in $(seq 1 30); do
  if probe_health; then OK=1; break; fi
  sleep 2
done

if [ "$OK" -ne 1 ]; then
  (cd "$DIR" && $COMPOSE logs --tail=30) || true
  {
    printf '\n'
    warn "容器内与宿主机均未探测到 /api/health，请按下列命令确认："
    printf '  容器状态：  cd %s && %s ps\n' "$DIR" "$COMPOSE"
    printf '  端口映射：  docker inspect oci-manager --format "{{json .HostConfig.PortBindings}}"\n'
    printf '  端口监听：  sudo ss -tlnp | grep :%s\n' "$HTTP_PORT"
    printf '  手动探测：  curl --noproxy "*" -v http://127.0.0.1:%s/api/health\n' "$HTTP_PORT"
    printf '  若容器已 Running 且日志显示「已启动」，多为全局 http_proxy 干扰，\n'
    printf '  浏览器仍可直接访问面板，健康检查失败不影响使用。\n'
    printf '\n'
  } >&2
  fail "健康检查未通过（容器可能已正常启动，请按上方命令确认）"
fi

# ---------- 9. 完成提示 ----------
if [ "$HTTPS_MODE" -eq 1 ]; then
  cat <<EOF

$(printf '\033[1;32m')
╔══════════════════════════════════════════════════════════════╗
║              OCI 管理器部署完成（HTTPS 模式）                  ║
╚══════════════════════════════════════════════════════════════╝
$(printf '\033[0m')
  访问地址： https://<你的域名>:${HTTPS_PORT}
  安装目录： ${DIR}
  配置文件： ${DIR}/.env（内含加密密钥，请备份）

  检测到已签发证书，HTTP 已关闭：宿主机不再暴露 HTTP 端口，
  仅映射 ${HTTPS_PORT} -> 容器 8443（HTTPS）。
  容器内 127.0.0.1:8080/api/health 仍供 Docker 健康检查，不依赖宿主机映射；
  若有明文 HTTP 请求仍到达容器，后端一律返回 403（仅 /api/health 放行）。

  请放行云厂商安全组的 ${HTTPS_PORT} 端口。
  说明：切换只发生在你主动重跑本脚本时。若证书被删除（或临时需要恢复 HTTP），
        重跑同一条命令即可按证书是否存在重新判定模式。

  常用命令：
    cd ${DIR} && ${COMPOSE} ps              查看状态
    cd ${DIR} && ${COMPOSE} logs -f         查看日志
    cd ${DIR} && ${COMPOSE} restart         重启
EOF
else
  IP="$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || echo '<服务器IP>')"
  cat <<EOF

$(printf '\033[1;32m')
╔══════════════════════════════════════════════════════════════╗
║              OCI 管理器部署完成（首次 / HTTP 模式）             ║
╚══════════════════════════════════════════════════════════════╝
$(printf '\033[0m')
  访问地址： http://${IP}:${HTTP_PORT}
  安装目录： ${DIR}
  配置文件： ${DIR}/.env（内含加密密钥，请备份）

  本次仅映射 HTTP 端口 ${HTTP_PORT} -> 容器 8080，完全不映射 443（尚无证书）。
  容器内 /api/health 健康检查不依赖宿主机 HTTP 映射。

  首次打开请设置管理员账号，然后到「设置 → HTTPS 证书」用 Cloudflare DNS-01
  申请域名证书。当前是明文 HTTP，在启用 HTTPS 之前请勿录入真实 OCI API 凭证。

  证书签发生效后，重新执行同一条一键命令即关闭宿主机 HTTP 端口、只保留 HTTPS：
    curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh -o /tmp/oci-install.sh
    sudo bash /tmp/oci-install.sh      # 或 sudo OCI_MANAGER_PORT=${HTTP_PORT} bash /tmp/oci-install.sh
  证书热加载后 HTTP 仍可正常访问，不会把你锁在门外；只有在你重跑上面的命令、
        宿主机端口真正切成只映射 443 之后，明文 HTTP 业务请求才会返回 403。
        容器内 /api/health 始终放行，健康检查不受影响。
  HTTP 端口 ${HTTP_PORT} 已写入 .env，重跑不带参数也会沿用。

  请放行云厂商安全组的 ${HTTP_PORT} 端口，并尽快启用 Telegram 两步验证。
  纯 IP 证书不支持：证书只允许 Cloudflare DNS-01 域名证书；Passkey 需域名 + HTTPS。

  常用命令：
    cd ${DIR} && ${COMPOSE} ps              查看状态
    cd ${DIR} && ${COMPOSE} logs -f         查看日志
    cd ${DIR} && ${COMPOSE} restart         重启
EOF
fi

if [ "$HTTPS_MODE" -eq 1 ] && [ "$HTTPS_PORT" != "443" ]; then
  warn "HTTPS 使用非标准端口 ${HTTPS_PORT}，浏览器需访问 https://域名:${HTTPS_PORT}（或让 Cloudflare 代理到 443）"
fi
