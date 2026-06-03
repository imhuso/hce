# HCE 便捷命令。`make` 或 `make help` 查看全部。
.DEFAULT_GOAL := help

.PHONY: help up down logs restart cli vet build

help: ## 显示可用命令
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-9s\033[0m %s\n", $$1, $$2}'

up: ## 一键起全栈：自动建 .env、起 compose、等就绪、给出下一步
	@bash scripts/up.sh

down: ## 停掉全栈（保留数据卷）
	docker compose down

logs: ## 跟踪后端日志
	docker compose logs -f hce-server

restart: ## 重启后端容器
	docker compose restart hce-server

cli: ## 本地编译 hce 到 ./bin/hce
	go build -o bin/hce ./cmd/hce/

build: ## 本机编译服务端（需 CGO）
	CGO_ENABLED=1 CGO_CFLAGS="-Wno-null-character" go build -ldflags="-s -w" -o hce-server ./cmd/server/

vet: ## 静态检查（go vet）
	go vet ./...
