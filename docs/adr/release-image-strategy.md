# 发布镜像策略:dockers_v2 + distroless + 双 Dockerfile + ghcr.io

2026-07-27 决策;2026-10-20 更新发布完成状态、运维探针与标签推进策略(术语见 `CONTEXT.md`)。

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
- alpine:有 shell 易调试;但 musl libc 偶有兼容性坑,需额外安装 CA 证书和时区数据。
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

**当前范围**:CLI 与 server 均发布 Linux / macOS / Windows × amd64 / arm64 档案。
部署用 `kite-probe` 只编译 Linux 两种架构,随 server 镜像和签名部署安装包分发,不增加第三类用户档案。

- 镜像构建:GoReleaser `dockers_v2`,`ids: [server, deploy-probe]`;CLI 只走 archives。
- base:固定 digest 的 `gcr.io/distroless/static-debian12:nonroot`,两份 Dockerfile 保持相同运行时安全策略。
- Dockerfile:`Dockerfile.release` 只 COPY 已编译二进制;根 `Dockerfile` 供本地从源码多阶段编译。
- registry:`ghcr.io/vod-studio/kite-server`。
- 镜像 tag:构建只推精确版本。全部发布产物验真后,串行按数值 SemVer 推进 `latest` 和对应 minor 标签;
  补旧版不得倒退浮动标签,公开版本不得覆盖原字节。
- 签名:Cosign keyless(OIDC,不传 `--key`);镜像按 index digest 签名,校验和及部署安装包各有 Sigstore bundle。
- SBOM:`dockers_v2.sbom: true`(默认)。
- 完成状态:release-please 先建 draft / tag,档案、签名、镜像和安装包完成并验证后才公开;
  用户看见「已发布」不能仍缺附件。公开版重跑只验证原产物;损坏历史版本通过修正版解决。
- 管理权限:CI 仅部署已批准的应用版本;宿主机部署器由管理员验证签名安装包后升级。
  受限 SSH 密钥不能修改自己的授权文件或 root 程序。切换流程见 [部署指南](../deploy.md)。

### GoReleaser 编译/COPY 模式(核心)

GoReleaser 编译二进制后,`dockers_v2` 按 `$TARGETPLATFORM` 注入构建上下文。
`Dockerfile.release` 直接 COPY `server` 和 `kite-probe`,绝不重复 `go build`。
合并前 CI 从 snapshot 产物实际构建 / 运行发布镜像,公开前再验证发布 index 及双架构镜像。

### 版本信息嵌入

kite 的 `internal/cli/version` 包用 `debug.ReadBuildInfo()` 读 vcs 元数据(非 ldflags 注入)。GoReleaser 配 `mod_timestamp: "{{ .CommitTimestamp }}"` 让 vcs.time/revision 正确嵌入,version 包才能读到 commit/build time。`ldflags` 只保留 `-s -w`(strip 调试信息),**不需要 `-X main.version`**。

## Consequences

- 反悔成本中:改回老 `dockers` 要等 v3(无 ETA)移除后才完全不可用,但配置会一直 warning。
- 发布 Dockerfile 不编译,开发 Dockerfile 从源码构建;两条路径均有实际容器冒烟,不以源码文本相等作证明。
- 镜像无 shell,部署验收依靠独立的 REST/gRPC 静态探针,不在服务镜像安装运维工具。
- Cosign keyless 依赖 GitHub OIDC;仅可信 main 工作流拥有签名权限,部署端固定证书身份与 issuer。
- 单机部署接受短暂中断;先完成来源 / digest 验证再替换,失败使用旧容器实际本地 image ID 恢复。

## 反向条件

若未来需要 shell 调试能力(如线上排障),改回 alpine base 更合适;若无需 gRPC TLS(纯内网明文),可用 scratch 进一步缩小。
