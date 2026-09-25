# 重力归算服务便捷命令
GO_IMAGE ?= golang:1.22
IMAGE    ?= gravity-reduce
PORT     ?= 8080

.PHONY: help build test test-race vet fmt run docker-build docker-run tidy vendor clean

help: ## 显示可用目标
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-14s %s\n", $$1, $$2}'

build: ## 编译二进制到 bin/
	CGO_ENABLED=0 go build -trimpath -o bin/gravity-server ./cmd/server

test: ## 运行全部测试（可独立执行）
	go test -count=1 ./...

test-race: ## 带竞态检测运行测试
	go test -race -count=1 ./...

vet: ## 静态检查
	go vet ./...

fmt: ## 格式化代码
	gofmt -w .

run: ## 本地启动服务
	PORT=$(PORT) go run ./cmd/server

docker-build: ## 用本地 Go 基础镜像构建容器（离线、使用 vendor）
	docker build -t $(IMAGE) --build-arg GO_IMAGE=$(GO_IMAGE) .

docker-run: ## 运行容器并映射端口
	docker run --rm -p $(PORT):8080 $(IMAGE)

vendor: ## 固化依赖到 vendor/
	go mod vendor

tidy: ## 整理依赖
	go mod tidy

clean:
	rm -rf bin/
