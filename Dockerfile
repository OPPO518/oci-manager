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

USER app
WORKDIR /home/app
EXPOSE 8080
VOLUME ["/data"]

ENV OCI_MANAGER_ADDR=:8080 \
    OCI_MANAGER_DB=/data/oci-manager.db

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget --spider -q http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["/usr/local/bin/oci-manager"]
