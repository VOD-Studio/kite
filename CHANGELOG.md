# Changelog

本项目所有值得注意的变更都会记录在此文件。

格式基于 [Keep a Changelog 2.0.0](https://keepachangelog.com/zh-CN/2.0.0/),
版本号遵循 [Semantic Versioning 2.0.0](https://semver.org/lang/zh-CN/)。

`v0.x` 阶段 API 不保证稳定;`BREAKING CHANGE` 只 bump minor(semver 0.x 规则)。

## [0.2.0](https://github.com/VOD-Studio/kite/compare/v0.1.0...v0.2.0) (2026-08-16)


### Features

* **cli:** Phase B 配置——config.toml 与 config 命令组(PRD-0017) ([#23](https://github.com/VOD-Studio/kite/issues/23)) ([ce0771a](https://github.com/VOD-Studio/kite/commit/ce0771ada571727cba64e13ef70f5095275717cb))
* **netease:** 代理支持——engine/CLI 双路径与 --proxy flag(PRD-0018) ([#24](https://github.com/VOD-Studio/kite/issues/24)) ([6de0c38](https://github.com/VOD-Studio/kite/commit/6de0c38465152ed13c7da4ebd177b8ebd8d2be5c))

## [Unreleased]

后续版本由 release-please 从 Conventional Commits 自动维护。

## [0.1.0] - 2026-07-27

kite 从 mimo-music 独立后的首个版本基线。确立双产物架构与网易云核心能力。

### Added

- **双产物架构**:gRPC + grpc-gateway 服务(`cmd/server`)+ 直连 engine 的 CLI(`cmd/kite`),共享 `internal/netease/` 核心与 service 层
- **网易云 9 域全接入** CLI:登录(`auth`)、歌曲(`song`)、专辑(`album`)、歌手(`artist`)、歌单(`playlist`)、用户(`user`)、搜索(`search`)、推荐(`recommend`)、私人 FM(`fm`)
- **gRPC 服务**:网易云 RPC 契约(`proto/netease/music/v1/`),grpc-gateway REST 暴露(`:3721`),gRPC 端口 `:3722`,SIGINT/SIGTERM 优雅关闭
- **CLI 输出层**:TTY 人类可读(表格 / 键值对,runewidth CJK 对齐)+ 非 TTY 自动回退 JSON;全局 `--json` / `--yes`;退出码规范(0 成功 / 1 错误 / 2 用法 / 3 未登录)
- **实用功能**:单曲/歌单下载(带 ID3 元数据)
- **可观测性**:OpenTelemetry(OTLP exporter)+ Prometheus 指标 + 结构化日志
- **基础设施**:MIT LICENSE、CI(lint + test + gen/ 守护)、Dependabot、SECURITY.md、CONTRIBUTING.md、AGENTS.md 开发规范

[Unreleased]: https://github.com/VOD-Studio/kite/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/VOD-Studio/kite/releases/tag/v0.1.0
