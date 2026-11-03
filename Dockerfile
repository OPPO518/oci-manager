# ---------- 阶段 1：构建前端 ----------
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---------- 阶段 2：构建后端 ----------
FROM golang:1.27-alpine AS server
WORKDIR /src
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# vite 的 outDir 指向 ../internal/web/dist，在构建阶段对应 /internal/web/dist
COPY --from=web /internal/web/dist ./internal/web/dist

# 纯静态编译：SQLite 使用 modernc.org/sqlite（无 CGO），目标镜像无需任何运行时依赖
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/oci-manager ./cmd/server

# ---------- 阶段 3：运行 ----------
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S app -G app

COPY --from=server /out/oci-manager /usr/local/bin/oci-manager

USER app
WORKDIR /home/app
EXPOSE 8080
VOLUME ["/data"]

ENV OCI_MANAGER_ADDR=:8080 \
    OCI_MANAGER_DB=/data/oci-manager.db

ENTRYPOINT ["/usr/local/bin/oci-manager"]
