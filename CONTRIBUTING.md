# 贡献指南

感谢你有兴趣为 kite 贡献!本文档是上手摘要,**开发规范的权威来源是 [`AGENTS.md`](AGENTS.md)**——分支命名、commit 格式、原子性、架构耦合等细节以它为准。本文件不重复其内容(避免双真相腐烂)。

## 开发环境

- **Go 1.25**(见 `go.mod`)。缺 Go 时 `make` 会直接失败。
- **buf**:改 `proto/` 时需要(Makefile 自动探测 `$GOPATH/bin/buf`)。安装见 [buf.build](https://buf.build/docs/installation/)。

## 常用命令

```bash
make proto       # 改 proto 后重新生成 gen/(gen/ 已进版本控制,改完记得 git add)
make docs        # 改命令树后刷新 docs/cmd/(freshness 守护测试依赖此 target)
go build ./cmd/server/    # 编译服务
go build ./cmd/kite/      # 编译 CLI
go test ./...             # 测试
go vet ./...              # 检查
```

## 提交流程(摘要)

1. 从 `main` 新建分支(命名见 AGENTS.md「分支命名」段)
2. 开发,按 AGENTS.md「原子性」段落提交——**一个 commit = 一个逻辑变更**
3. 推送,用 `gh pr create` 创建 PR(参数见 AGENTS.md「PR 与 issue 规范」)
4. 等待 review,人工 squash merge

**注意**:分支单位是 feature / PRD,**不是单个 issue**。同一 PRD 的子 issue 共用一个分支。

## 报告安全问题

见 [`SECURITY.md`](SECURITY.md)——**不要在公开 issue 报安全漏洞**,走 GitHub 私有安全公告。

## 项目结构

详见 AGENTS.md「架构与代码边界」段。核心约定:

- `proto/` 是 RPC 契约唯一真相,改 RPC 先改 proto 再 `make proto`
- `internal/service/` 是 CLI 与 TUI 的共享边界,两者不互相直接调用
- `gen/`、`docs/cmd/` 是生成产物,已进版本控制(不要手改)
