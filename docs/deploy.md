# 生产部署

kite-server 与 violet 部署在同一台服务器上:容器加入 violet 的内网 `violet_network`(网络别名 `kite`),
不映射宿主机端口,violet 经 `http://kite:3721`(REST)/ `kite:3722`(gRPC)访问。
监听非回环地址,所以访问令牌是必需的(见 [server-security.md](server-security.md))。

## 自动部署

release PR 合并 → `release.yml` 发布镜像 `ghcr.io/vod-studio/kite-server:vX.Y.Z` → `deploy.yml` 部署该版本。

```
runner ──ssh kite-deploy@服务器 "ghcr.io/vod-studio/kite-server:vX.Y.Z"──▶ deploy-ssh-gate.sh(强制命令,只收镜像引用)
        stdin: GITHUB_TOKEN(拉私有镜像,job 结束即失效)          └─sudo─▶ deploy-server.sh
                                                                      拉取 → 锁定 digest → 替换容器 → /healthz
                                                                      失败:回滚到替换前的镜像,job 变红
```

- **runner**:私有 runner(组织级,标签 `omarchy`)在线且空闲时优先使用,否则用 GitHub 托管 runner
  (`scripts/pick-runner.sh`)。查询私有 runner 状态需要仓库 secret `RUNNER_STATUS_TOKEN`,不配则总用托管 runner。
- **部署私钥的权限**:服务器上的 `kite-deploy` 用户没有 shell,公钥被 `restrict,command=` 限定为只能执行
  `deploy-ssh-gate.sh`;sudo 只放行 `deploy-server.sh` 一个命令。私钥泄露时,能做的只有部署某个已发布的版本。
- **公开仓库**:`deploy.yml` 只响应发版和手动触发;部署 job 不检出代码,只执行一条 SSH 命令。

部署历史记录在服务器的 `/srv/kite/deploy-history`(时间、版本、digest)。

## 手动部署 / 回滚

Actions → Deploy → Run workflow,填已发布的 tag。填旧版本即回滚:

```bash
gh workflow run deploy.yml -f tag=v0.3.0
```

## 服务器一次性安装

以 root 在服务器上执行(可重复执行;改了 `scripts/deploy-*.sh` 后重跑即更新服务器上的副本):

```bash
# 在本地生成部署专用密钥(不要复用个人密钥)
ssh-keygen -t ed25519 -N '' -C kite-deploy -f kite-deploy
# 把 scripts/ 下的三个部署脚本和公钥传到服务器后:
sudo ./deploy-server-install.sh kite-deploy.pub
```

安装脚本建 `kite-deploy` 用户、装脚本到 `/usr/local/libexec/kite/`、写 sudoers,并在没有 podman secret
`kite-auth-token` 时生成访问令牌,副本存 `/srv/kite/auth-token`(0600),供 violet 配置。已有令牌不会轮换。

然后配置仓库 secret:

| secret | 内容 |
|---|---|
| `KITE_DEPLOY_SSH_KEY` | 部署私钥(`kite-deploy` 文件内容) |
| `KITE_DEPLOY_KNOWN_HOSTS` | `ssh-keyscan -p <端口> <主机>` 的输出,固定主机指纹 |
| `KITE_DEPLOY_HOST` / `KITE_DEPLOY_PORT` | 服务器地址与 SSH 端口 |
| `RUNNER_STATUS_TOKEN` | 可选,组织「Self-hosted runners」只读令牌 |

未配置前三项时 `deploy.yml` 只给提示并跳过,不会让发布流程变红。

## violet 侧

- `kite_url` 设为 `http://kite:3721`;每个请求带 `Authorization: Bearer <令牌>`(令牌即 `/srv/kite/auth-token`)。
- 网易云登录态经请求头 `Grpc-Metadata-X-Netease-Cookie` 传入,由 violet 保存;
  过期前调 `POST /api/v1/auth/refresh` 续期,收到 `LOGIN_REQUIRED` 时引导重新登录(见 [grpc-errors.md](grpc-errors.md))。
