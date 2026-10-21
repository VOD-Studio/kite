# Changelog

本项目所有值得注意的变更都会记录在此文件。

格式基于 [Keep a Changelog 2.0.0](https://keepachangelog.com/zh-CN/2.0.0/),
版本号遵循 [Semantic Versioning 2.0.0](https://semver.org/lang/zh-CN/)。

`v0.x` 阶段 API 不保证稳定;`BREAKING CHANGE` 只 bump minor(semver 0.x 规则)。

## 0.5.0 (2026-10-21)

## What's Changed
* feat(player): 网页播放器 @violetjs/kite-player 与 kite-server 公开模式 by @xunrua in https://github.com/VOD-Studio/kite/pull/95


**Full Changelog**: https://github.com/VOD-Studio/kite/compare/v0.4.2...v0.5.0

## 0.4.2 (2026-10-21)

## What's Changed
* fix(ci): 发布闸门从 release 列表里找 draft by @xunrua in https://github.com/VOD-Studio/kite/pull/93
* fix(netease): 修复账号歌单的分页与登录上下文 by @xunrua in https://github.com/VOD-Studio/kite/pull/94


**Full Changelog**: https://github.com/VOD-Studio/kite/compare/v0.4.1...v0.4.2

## 0.4.1 (2026-10-20)

## What's Changed
* ci(deploy): kite-server 改用专用网络 kite_network by @xunrua in https://github.com/VOD-Studio/kite/pull/87
* fix(ci): 重构发布与部署信任边界 by @xunrua in https://github.com/VOD-Studio/kite/pull/91


**Full Changelog**: https://github.com/VOD-Studio/kite/compare/v0.4.0...v0.4.1

## [0.4.0](https://github.com/VOD-Studio/kite/compare/v0.3.1...v0.4.0) (2026-10-19)


### Features

* **cli:** 新增 kite mcp,以 stdio MCP 服务暴露只读接口 ([#84](https://github.com/VOD-Studio/kite/issues/84)) ([c329322](https://github.com/VOD-Studio/kite/commit/c32932211ba9a0228f52c203b047d7dcaa1a7455))
* **netease:** 播放链接可选登录与 RefreshLogin 登录态续期 ([#82](https://github.com/VOD-Studio/kite/issues/82)) ([6545ec3](https://github.com/VOD-Studio/kite/commit/6545ec3e968fb2a4915937d5409ea3ec1ffe703c))

## [0.3.1](https://github.com/VOD-Studio/kite/compare/v0.3.0...v0.3.1) (2026-10-17)


### Bug Fixes

* **netease:** 修复 5 个对真实上游不可用的 RPC,新增真实上游冒烟 ([#77](https://github.com/VOD-Studio/kite/issues/77)) ([89ed8ef](https://github.com/VOD-Studio/kite/commit/89ed8efe34eb4b2d67b11f1c074eefb311a68cdf))
* **proto:** BSR 补推早于接入的历史 tag 时补上模块名 ([#71](https://github.com/VOD-Studio/kite/issues/71)) ([13c606e](https://github.com/VOD-Studio/kite/commit/13c606ef97195a0cb3f64a0dd76e3f28c6d79ce1))

## [0.3.0](https://github.com/VOD-Studio/kite/compare/v0.2.2...v0.3.0) (2026-10-17)


### Features

* **server:** gRPC 错误映射为标准 status + ErrorInfo ([#55](https://github.com/VOD-Studio/kite/issues/55)) ([803391c](https://github.com/VOD-Studio/kite/commit/803391c33de7df3625ef9e9d142c646b6a81f880))
* **server:** 访问令牌鉴权、默认绑回环、gRPC 健康检查 ([#59](https://github.com/VOD-Studio/kite/issues/59)) ([57d49f3](https://github.com/VOD-Studio/kite/commit/57d49f3de1760e0b2971181af30a22ec57da0d06))


### Bug Fixes

* **ci:** workflow_dispatch 下 Lint 改用显式基线 ([#57](https://github.com/VOD-Studio/kite/issues/57)) ([109515b](https://github.com/VOD-Studio/kite/commit/109515b79ae8dbc032144723021dc5135ce1e422))
* **ci:** 自动批准 release PR 上等待批准的 CI 运行 ([#58](https://github.com/VOD-Studio/kite/issues/58)) ([a1dd911](https://github.com/VOD-Studio/kite/commit/a1dd9112976af1e4233d1195f36d9ef1c3265988))
* **netease:** 私人 FM 不缓存 ([#53](https://github.com/VOD-Studio/kite/issues/53)) ([46d8b38](https://github.com/VOD-Studio/kite/commit/46d8b38579443a1053c5ec5984fc7713024e69aa))
* **release:** 镜像签名改按 digest ([#62](https://github.com/VOD-Studio/kite/issues/62)) ([7004996](https://github.com/VOD-Studio/kite/commit/70049966f3dc088dda57dd5efcb3a3309ba57b63))

## [0.2.2](https://github.com/VOD-Studio/kite/compare/v0.2.1...v0.2.2) (2026-10-17)


### Bug Fixes

* **ci:** 固定 buf 插件版本并升级 x/net,消除无关 PR 的红灯 ([#46](https://github.com/VOD-Studio/kite/issues/46)) ([9d08821](https://github.com/VOD-Studio/kite/commit/9d08821c1e91d3240d3a7f231c71a160d1c9229d))
* **cli:** TestDocsFreshness 归一化 cobra 生成日期页脚 ([#41](https://github.com/VOD-Studio/kite/issues/41)) ([d2d1708](https://github.com/VOD-Studio/kite/commit/d2d17085b0a7c5c2b797bd01a1de048027e6a812))
* **netease:** 修复服务端登录态凭据、缓存串数据与熔断误触发 ([#51](https://github.com/VOD-Studio/kite/issues/51)) ([f7bac42](https://github.com/VOD-Studio/kite/commit/f7bac42dc1f7fd5015dc0e6c67d31448ba8fba92))

## [0.2.1](https://github.com/VOD-Studio/kite/compare/v0.2.0...v0.2.1) (2026-08-27)


### Bug Fixes

* **ci:** 弃用 MiniMax,默认评审模型改 glm5.3 flash(GLM Coding Plan) ([d1f01b7](https://github.com/VOD-Studio/kite/commit/d1f01b7a2befc6b4ae067e604e26abb938d17349))

## [0.2.0](https://github.com/VOD-Studio/kite/compare/v0.1.0...v0.2.0) (2026-08-16)


### Features

* **cli:** Phase B 配置——config.toml 与 config 命令组(PRD-0017) ([#23](https://github.com/VOD-Studio/kite/issues/23)) ([ce0771a](https://github.com/VOD-Studio/kite/commit/ce0771ada571727cba64e13ef70f5095275717cb))
* **netease:** 代理支持——engine/CLI 双路径与 --proxy flag(PRD-0018) ([#24](https://github.com/VOD-Studio/kite/issues/24)) ([6de0c38](https://github.com/VOD-Studio/kite/commit/6de0c38465152ed13c7da4ebd177b8ebd8d2be5c))

## [Unreleased]

后续版本由 release-please 从 Conventional Commits 自动维护。

### Fixed

- **ci**:支持业务 PR 自选合并方式,以 release PR 的 Squash 合并作为发版批准;发布先完成并验证全部产物再公开,补旧版不覆盖字节或倒退浮动标签。
- **security**:升级 Go 安全补丁工具链,修复将高于当前工具链的已发布修复误判为 advisory 的漏洞门禁;固定工具 / Action / 镜像来源,故障不再静默跳过;开发镜像上下文排除本地凭据、Git 历史和构建产物。
- **deploy**:production 环境隔离部署凭据,增加签名及发布来源验证、宿主机互斥、不可变 digest 历史和确定性失败回滚;管理员通过签名安装包维护版本化部署器,真实 REST/gRPC 探针不访问网易云。

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
