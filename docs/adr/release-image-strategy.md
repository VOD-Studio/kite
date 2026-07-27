# 发布镜像策略:dockers_v2 + distroless + 双 Dockerfile + ghcr.io

2026-07-27 决策(访谈见 `CONTEXT.md`)。

## Context

kite 是双产物:CLI 跨平台二进制 + gRPC 服务镜像。发布管道由 GoReleaser 驱动,需要决定:

1. 镜像构建用 GoReleaser 的哪种 docker 配置?
2. 镜像 base 用什么?
3. 发布用 Dockerfile 与本地开发 Dockerfile 的关系?
4. 镜像推到哪个 registry?

## Considered Options

### 镜像构建字段:`dockers_v2`(采用) vs 老 `dockers`(否决)

- **`dockers_v2`**(v2.16 起稳定):一段搞定多平台 + manifest,内部用 `docker buildx`,基于 `$TARGETPLATFORM` 注入二进制。2026 推荐。
- 老 `dockers` + `docker_manifests`:v2.12 起 deprecated,要拆两段(单架构构建 + 合并 manifest),v3 才移除(无 ETA)。来源:[dockers 弃用通告](https://goreleaser.com/resources/deprecations/)、[dockers_v2 文档](https://goreleaser.com/customization/package/dockers_v2/)。

### base 镜像:distroless static-debian12(采用) vs alpine vs scratch

- **distroless `gcr.io/distroless/static-debian12:nonroot`**(采用):Go 静态二进制(`CGO_ENABLED=0`)完美匹配,**自带 CA 证书包**(gRPC over TLS 必需),无 shell 极小攻击面。GoReleaser 官方 supply-chain example 用 `FROM scratch`,社区 gRPC 教程普遍用 distroless。
- alpine:现 Dockerfile 在用,~5MB,有 shell 易调试;但 musl libc 偶有兼容性坑,需 `apk add ca-certificates tzdata`。
- scratch:最极端(~0+binary),但要手动管 CA 证书。

选 distroless:服务端场景对安全/体积敏感,gRPC TLS 需 CA 证书,且 `:nonroot` 变体已是非 root。

### Dockerfile 关系:双 Dockerfile(采用) vs 单一 Dockerfile(否决)

- **双 Dockerfile**(采用):`Dockerfile.release`(发布用,极简,COPY 二进制不重编译)+ 根 `Dockerfile`(本地开发用,多阶段自己 go build)。GoReleaser 的设计前提是「发布 Dockerfile 只 COPY 二进制」,与「本地开发多阶段编译」是两种性质,不该混用。
- 单一 Dockerfile:若让 GoReleaser 调多阶段 Dockerfile,会在容器内重复 `go build`——GoReleaser 官方明确警告不推荐(慢、失去跨平台编译优势)。

来源:[GoReleaser example-supply-chain](https://github.com/goreleaser/example-supply-chain)、[dockers_v2 文档](https://goreleaser.com/customization/package/dockers_v2/)

### Registry:ghcr.io(采用) vs Docker Hub(否决)

- **ghcr.io**(`ghcr.io/vod-studio/kite-server`):GitHub 原生,`GITHUB_TOKEN` 零配置推送,仓库级权限继承,与 GitHub Actions 无缝集成。
- Docker Hub:大众分发,但需额外配 token、有匿名拉取限额(rate limit)。kite 托管在 GitHub,ghcr.io 无优势损失。

## Decision

**当前范围**:仅 server 镜像发布。CLI 二进制发版(build + archives + signs)暂未配——CLI 的 beep/oto 依赖 cgo,与 `CGO_ENABLED=0` 静态编译冲突;播放功能将迁移到 TUI(#14 移除 CLI 播放),故不为将死功能解决 cgo。等 #14 完成后补回 CLI build,届时:

- 镜像构建:GoReleaser `dockers_v2`,`ids: [server]`,只把 server 产物打进镜像(CLI 走 archives)。
- base:`gcr.io/distroless/static-debian12:nonroot`。
- Dockerfile:新增 `Dockerfile.release`(发布用,COPY 二进制);根 `Dockerfile`(本地开发)保留不动。
- registry:`ghcr.io/vod-studio/kite-server`(全小写,镜像名必须小写)。
- 镜像 tag:`latest` + `{{ .Tag }}` + `v{{ .Major }}.{{ .Minor }}`。
- 签名:cosign keyless(OIDC,不传 `--key`);workflow 需 `id-token: write`。
- SBOM:`dockers_v2.sbom: true`(默认)。

### GoReleaser 编译/COPY 模式(核心)

GoReleaser 在 `dist/<os>_<arch>/` 编译好二进制 → `dockers_v2` 把 `dist/<target>/` 按 `$TARGETPLATFORM` 注入构建上下文 → `Dockerfile.release` 里 `COPY $TARGETPLATFORM/server` 直接拿。**Dockerfile 绝不 `RUN go build`**。

### 版本信息嵌入

kite 的 `internal/cli/version` 包用 `debug.ReadBuildInfo()` 读 vcs 元数据(非 ldflags 注入)。GoReleaser 配 `mod_timestamp: "{{ .CommitTimestamp }}"` 让 vcs.time/revision 正确嵌入,version 包才能读到 commit/build time。`ldflags` 只保留 `-s -w`(strip 调试信息),**不需要 `-X main.version`**。

## Consequences

- 反悔成本中:改回老 `dockers` 要等 v3(无 ETA)移除后才完全不可用,但配置会一直 warning。
- 发布 Dockerfile 极简(4 行),本地 Dockerfile 多阶段,两者职责分离,互不影响。
- 镜像无 shell,调试需 `docker cp` 或临时换 alpine 镜像。
- cosign keyless 需 GitHub OIDC,自托管 runner 要支持 OIDC 才能签名。

## 反向条件

若未来需要 shell 调试能力(如线上排障),改回 alpine base 更合适;若无需 gRPC TLS(纯内网明文),可用 scratch 进一步缩小。
