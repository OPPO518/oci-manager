GO ?= go
NPM ?= npm
BINARY ?= oci-manager

.PHONY: all deps web server build run dev docker fmt vet test clean

all: web server

deps:
	cd web && $(NPM) install --no-audit --no-fund

web:
	cd web && $(NPM) run build

server:
	$(GO) build -trimpath -o bin/$(BINARY) ./cmd/server

build: web server

run: server
	./bin/$(BINARY)

# 后端热重载式的开发入口：前端用 vite dev server（端口 5173）代理到后端
dev:
	$(GO) run ./cmd/server

docker:
	docker build -t oci-manager:latest .

up:
	docker compose up -d --build

fmt:
	$(GO) fmt ./...
	cd web && $(NPM) run format

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

clean:
	rm -rf bin web/dist
