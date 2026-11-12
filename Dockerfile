# 基础镜像变量必须声明在第一个 FROM 之前，否则属于阶段内作用域，
# 无法用于后续 FROM，会报 "base name should not be blank"
ARG NODE_IMAGE=node:22-alpine
ARG GO_IMAGE=golang:1.27-alpine
ARG RUNTIME_IMAGE=alpine:3.24

# ---------- 阶段 1：构建前端 ----------
FROM ${NODE_IMAGE} AS web

ARG NPM_REGISTRY=https://registry.npmmirror.com
WORKDIR /web
RUN npm config set registry "$NPM_REGISTRY"

COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---------- 阶段 2：构建后端 ----------
FROM ${GO_IMAGE} AS server

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY \
    GOTOOLCHAIN=local \
    CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# vite 的 outDir 指向 ../internal/web/dist，在 web 阶段对应 /internal/web/dist
COPY --from=web /internal/web/dist ./internal/web/dist

# 纯静态编译：SQLite 使用 modernc.org/sqlite（无 CGO），运行镜像无需任何额外依赖
RUN go build -trimpath -ldflags="-s -w" -o /out/oci-manager ./cmd/server

# ---------- 阶段 3：运行 ----------
FROM ${RUNTIME_IMAGE}

RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S app && adduser -S app -G app

COPY --from=server /out/oci-manager /usr/local/bin/oci-manager

# 预先创建 /data 并交给 app，避免绑定挂载的宿主机目录属主为 root 时
# 进程无权写入，导致 SQLite 报 unable to open database file (14)
RUN mkdir -p /data && chown -R app:app /data

USER app
WORKDIR /home/app
# 8080 = HTTP（始终可用），8443 = HTTPS（证书配置后生效）。
# 使用高位端口是因为容器以非 root 的 app 用户运行，无法绑定 443 特权端口。
EXPOSE 8080 8443
VOLUME ["/data"]

ENV OCI_MANAGER_ADDR=:8080 \
    OCI_MANAGER_HTTPS_ADDR=:8443 \
    OCI_MANAGER_DB=/data/oci-manager.db

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget --spider -q http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["/usr/local/bin/oci-manager"]
