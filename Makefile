.PHONY: all help install-tools install-cli fmt fmt-check tidy-check vet test lint vuln build build-server \
        build-cli run dev proto proto-lint proto-breaking proto-deps verify-gen docs scripts-check interop-rust interop-go upstream-smoke player-check check ci-env clean

# 工具探测:优先 PATH,回退 $GOPATH/bin(GOBIN 也在 PATH 时无所谓)
BUF           ?= $(shell command -v buf 2>/dev/null           || echo $$(go env GOPATH)/bin/buf)
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || echo $$(go env GOPATH)/bin/golangci-lint)
AIR           ?= $(shell command -v air 2>/dev/null           || echo $$(go env GOPATH)/bin/air)

# ── 工具版本:唯一来源 ─────────────────────────────────────────────
# CI 通过 `make -s ci-env` 读取这里的值(不再在 workflow 里重复写),本地 install-tools
# 与 CI 因此天然一致。升级工具只改这一处。
# golangci-lint 锁到 minor.patch 之外的 minor(v2.12 = 该 minor 的最新 patch),不用 latest。
GOLANGCI_LINT_VERSION := v2.12
BUF_VERSION           := 1.71.0
GOVULNCHECK_VERSION   := v1.6.0
GORELEASER_VERSION    := 2.17.1
COSIGN_VERSION        := v3.1.2
# packages/kite-player(npm 包 @violetjs/kite-player)用的 Node 大版本;CI 的 setup-node 读这里。
NODE_VERSION          := 24
# 仅本地热重载用,不影响 CI。
AIR_VERSION           := latest

# proto breaking 检查的对照基线:本地 .git 里 origin/main 的远端跟踪引用(PR 对比 main)。
# 用 buf 文档明确支持的 ref= 语法;CI 需 fetch-depth: 0 才有该引用。
PROTO_BREAKING_BASE ?= ../.git\#ref=refs/remotes/origin/main,subdir=proto

## all: 默认 —— 等同 help(裸 make 不应触发有副作用的生成)
all: help

## help: 列出所有 target 及说明(解析本文件里的 `## xxx:` 注释)
help:
	@awk '/^## [a-zA-Z_-]+:/ { line=$$0; sub(/^## /,"",line); n=index(line,":"); printf "  \033[36m%-18s\033[0m %s\n", substr(line,1,n-1), substr(line,n+2) }' $(MAKEFILE_LIST)

## install-tools: 装 buf / golangci-lint / govulncheck / air,版本与 CI 一致(见文件顶部版本表)
## 用 go install 装到 $GOPATH/bin。前置:PATH 含 $(go env GOPATH)/bin
install-tools:
	@echo "→ 安装 buf $(BUF_VERSION)..."
	go install github.com/bufbuild/buf/cmd/buf@v$(BUF_VERSION)
	@echo "→ 安装 golangci-lint $(GOLANGCI_LINT_VERSION)..."
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "→ 安装 govulncheck $(GOVULNCHECK_VERSION)..."
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
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

## fmt-check: 检查全仓库 gofmt(排除 gen/),有未格式化文件则失败(不改文件)
fmt-check:
	@out="$$(git ls-files '*.go' | grep -v '^gen/' | xargs gofmt -l)"; \
	if [ -n "$$out" ]; then echo "以下文件未 gofmt(跑 make fmt):"; echo "$$out"; exit 1; fi

## tidy-check: go mod tidy 后 go.mod / go.sum 不应有变化
tidy-check:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

## vet: go vet ./...(轻量静态检查)
vet:
	go vet ./...

## test: 跑全量测试,带 race detector + 覆盖率(与 CI 等价)
test:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

## lint: golangci-lint 全量检查(与 CI 一致)
lint:
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { \
		echo "✗ 未找到 golangci-lint,跑 'make install-tools' 安装"; exit 1; }
	$(GOLANGCI_LINT) run --timeout 5m ./...

## vuln: govulncheck 门禁(有可修复漏洞则失败;判定逻辑见 scripts/govulncheck-gate.sh)
vuln:
	scripts/govulncheck-gate.sh

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

## proto-breaking: 对比 main 检查 proto 破坏性变更(gen/ 已提交且 REST/gRPC 对外,契约不能悄悄破)
proto-breaking:
	cd proto && $(BUF) breaking --against '$(PROTO_BREAKING_BASE)'
	@echo "proto 无破坏性变更"

## verify-gen: 重新生成后 gen/ 不应有漂移(CI 同款)
verify-gen: proto
	scripts/check-generated.sh gen/

## proto-deps: 拉取 buf BSR 依赖（googleapis）
proto-deps:
	cd proto && $(BUF) dep update
	@echo "proto 依赖更新完成"

## docs: 生成 kite 全命令 markdown 参考(cobra GenMarkdownTree → docs/cmd/)
## 改命令树后跑此 target 刷新,否则 freshness 守护测试会红。
docs:
	go run cmd/kite-docs/main.go docs/cmd
	@echo "命令参考已刷新(若 docs/cmd/ 有变化记得提交)"

## service-gen: 从 proto 生成 internal/service 的一行转发胶水(*_gen.go)
## 新增/改名 rpc 或调整 internal/servicegen 映射表后跑此 target,否则 freshness 测试会红。
service-gen:
	go run ./cmd/kite-gen-service
	@echo "service 转发已刷新(若 *_gen.go 有变化记得提交)"

## upstream-smoke: 真实上游冒烟(对网易云真实接口逐个调用服务端的 service 方法,断言响应非空)
## 手动运行,不进 make check / CI:外部依赖会让门禁「没改代码也变红」。改了 endpoint 或解码器后建议跑一次。
upstream-smoke:
	go test -tags upstream ./internal/service/ -run TestUpstreamSmoke -count=1 -v

## interop-rust: Rust 客户端互操作冒烟(buf 编 proto → 生成 tonic 客户端 → 对真实 kite-server 调用)
## 需要 cargo(工具链由 interop/rust/rust-toolchain.toml 固定);较慢,不在 make check 里。
interop-rust:
	BUF=$(BUF) scripts/rust-interop.sh

## interop-go: Go 互操作冒烟——用 BSR 上生成的 SDK 对真实运行的 kite-server 跑 docs/integration.md 的示例(需要访问 buf.build)
interop-go:
	scripts/go-interop.sh

## player-check: packages/kite-player 的类型检查 + 测试 + 构建(需要 Node,大版本见文件顶部;不在 make check 里)
player-check:
	cd packages/kite-player && npm ci --no-audit --no-fund && npm run check

## scripts-check: scripts/*.sh 的 shellcheck 与单元测试(release PR 的 CI 保障逻辑等)
scripts-check:
	shellcheck scripts/*.sh
	@set -e; for test in scripts/*_test.sh; do bash "$$test"; done

## check: push 前一键自检(镜像 CI 的主要检查;vuln / verify-gen 依赖网络,按需单独跑)
check: fmt-check tidy-check scripts-check vet test lint proto-lint
	@echo "✓ fmt + tidy + scripts + vet + test + lint + proto-lint 全通过,可以 push"

## ci-env: 打印工具版本为 KEY=VALUE,供 CI 写入 $GITHUB_ENV(`make -s ci-env >> "$GITHUB_ENV"`)
ci-env:
	@echo "GOLANGCI_LINT_VERSION=$(GOLANGCI_LINT_VERSION)"
	@echo "BUF_VERSION=$(BUF_VERSION)"
	@echo "GOVULNCHECK_VERSION=$(GOVULNCHECK_VERSION)"
	@echo "GORELEASER_VERSION=$(GORELEASER_VERSION)"
	@echo "COSIGN_VERSION=$(COSIGN_VERSION)"
	@echo "NODE_VERSION=$(NODE_VERSION)"

## clean: 清除本地构建产物(tmp/、bin/)。不清 gen/——已进版本控制。
clean:
	rm -rf tmp/ bin/
