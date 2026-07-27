.PHONY: all help install-tools install-cli fmt vet test lint build build-server build-cli run dev \
        proto proto-lint proto-deps docs check clean

# 工具探测:优先 PATH,回退 $GOPATH/bin(GOBIN 也在 PATH 时无所谓)
BUF           ?= $(shell command -v buf 2>/dev/null           || echo $$(go env GOPATH)/bin/buf)
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || echo $$(go env GOPATH)/bin/golangci-lint)
AIR           ?= $(shell command -v air 2>/dev/null           || echo $$(go env GOPATH)/bin/air)

# 与 .github/workflows/ci.yml 对齐的版本,本地 / CI 一致避免「我这能过」
GOLANGCI_LINT_VERSION := v2.12   # 锁 patch,不用 latest,避免某天升级导致本地红
AIR_VERSION           := latest

## all: 默认 —— 等同 help(裸 make 不应触发有副作用的生成)
all: help

## help: 列出所有 target 及说明(解析本文件里的 `## xxx:` 注释)
help:
	@awk '/^## [a-zA-Z_-]+:/ { line=$$0; sub(/^## /,"",line); n=index(line,":"); printf "  \033[36m%-18s\033[0m %s\n", substr(line,1,n-1), substr(line,n+2) }' $(MAKEFILE_LIST)

## install-tools: 装 buf / golangci-lint / air(proto + lint + 热重载三件套)
## 用 go install 装到 $GOPATH/bin,版本与 CI 对齐。前置:PATH 含 $(go env GOPATH)/bin
install-tools:
	@echo "→ 安装 buf(go install,buffet/buf 最新稳定版)..."
	go install github.com/bufbuild/buf/cmd/buf@latest
	@echo "→ 安装 golangci-lint($(GOLANGCI_LINT_VERSION),与 CI 对齐)..."
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "→ 安装 air(热重载,$(AIR_VERSION))..."
	go install github.com/air-verse/air@$(AIR_VERSION)
	@echo "✓ 工具已装到 $$(go env GOPATH)/bin,确认该目录在 PATH 中"

## install-cli: 把 kite CLI 装到 $GOBIN / $GOPATH/bin(本地调试用,替代 go run)
install-cli:
	go install ./cmd/kite
	@echo "✓ kite 已安装到 $$(go env GOPATH)/bin"

## fmt: 应用 gofmt + goimports(与 .golangci.yaml formatter 一致)
fmt:
	gofmt -w .
	@command -v goimports >/dev/null 2>&1 || go install golang.org/x/tools/cmd/goimports@latest
	goimports -w .

## vet: go vet ./...(轻量静态检查)
vet:
	go vet ./...

## test: 跑全量测试,带 race detector + 覆盖率(与 CI 等价)
test:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

## lint: golangci-lint(本地全量;CI 用 only-new-issues 只报新增)
lint:
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { \
		echo "✗ 未找到 golangci-lint,跑 'make install-tools' 安装"; exit 1; }
	$(GOLANGCI_LINT) run --timeout 5m ./...

## build: 编译全部产物到 bin/(server + cli)
build: build-server build-cli

## build-server: 编译 gRPC + gateway server 到 bin/server
build-server:
	go build -o bin/server ./cmd/server

## build-cli: 编译 kite CLI 到 bin/kite
build-cli:
	go build -o bin/kite ./cmd/kite

## run: 直接跑 server(前台,无热重载)
run:
	go run ./cmd/server

## dev: 热重载跑 server(用 .air.toml,改 .go/.yaml 自动重建)
dev:
	@command -v $(AIR) >/dev/null 2>&1 || { \
		echo "✗ 未找到 air,跑 'make install-tools' 安装"; exit 1; }
	$(AIR)

## proto: 从 proto/ 生成 Go stub、gRPC service、gateway、OpenAPI 到 gen/
proto:
	cd proto && $(BUF) generate
	@echo "proto 代码生成完成"

## proto-lint: 检查 proto 文件规范
proto-lint:
	cd proto && $(BUF) lint
	@echo "proto lint 通过"

## proto-deps: 拉取 buf BSR 依赖（googleapis）
proto-deps:
	cd proto && $(BUF) dep update
	@echo "proto 依赖更新完成"

## docs: 生成 kite 全命令 markdown 参考(cobra GenMarkdownTree → docs/cmd/)
## 改命令树后跑此 target 刷新,否则 freshness 守护测试会红。
docs:
	go run cmd/kite-docs/main.go docs/cmd
	@echo "命令参考已刷新(若 docs/cmd/ 有变化记得提交)"

## check: push 前一键自检 = vet + test + lint(镜像 CI 跑的内容)
check: vet test lint
	@echo "✓ vet + test + lint 全通过,可以 push"

## clean: 清除本地构建产物(tmp/、bin/)。不清 gen/——已进版本控制。
clean:
	rm -rf tmp/ bin/
