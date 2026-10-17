kite 是一个用 Go 写的网易云音乐能力服务,同时提供命令行工具和对外的 gRPC / REST 接口,目标是让 Rust 等其它语言的项目直接使用。后续会接入更多音乐源。

这一页回答三个问题:已经开发了哪些接口,哪些在做,接下来做什么。接口清单不是手写的,由代码生成。

## 概览

| 指标 | 数值 |
|---|---|
| 已建模并可用的接口(RPC) | 78 个,分布在 9 个服务 |
| 其中可通过命令行调用的 | 78 / 78 |
| 上游接口蓝图(估算) | 约 357 个,当前覆盖约 22% |

这里的「已完成」指:该接口在 proto 里有定义,并且至少有一条命令行命令调用它,二者的对应关系由测试守护。它不等于上游每个字段都已覆盖。

## 已完成的接口

### 歌单(PlaylistService,16 个)

| 接口 | 命令行 | 说明 |
|---|---|---|
| `GetPlaylist` | `kite playlist detail` | 歌单详情 |
| `HighQuality` | `kite playlist highquality` | 精品歌单 |
| `HighQualityTags` | `kite playlist highquality-tags` | 精品歌单标签 |
| `CatList` | `kite playlist catlist` | 歌单分类列表 |
| `BrowseHot` | `kite playlist hot` | 热门歌单 |
| `Subscribers` | `kite playlist subscribers` | 歌单收藏者 |
| `AllTracks` | `kite playlist tracks` | 歌单全部歌曲 |
| `Subscribe` | `kite playlist subscribe` | 收藏/取消收藏歌单(写操作,登录态) |
| `Create` | `kite playlist create` | 创建歌单(写操作,登录态) |
| `Delete` | `kite playlist delete` | 删除歌单(写操作,登录态) |
| `UpdateName` | `kite playlist update-name` | 歌单改名(写操作,登录态) |
| `UpdateDesc` | `kite playlist update-desc` | 修改歌单描述(写操作,登录态) |
| `UpdateTags` | `kite playlist update-tags` | 修改歌单标签(写操作,登录态) |
| `UpdateTracks` | `kite playlist update-tracks` | 歌单增删歌曲(写操作,登录态) |
| `SimilarPlaylists` | `kite playlist similar` | 相似歌单(按歌曲) |
| `RelatedPlaylistRecommend` | `kite playlist related` | 相关歌单推荐 |

### 歌曲(SongService,16 个)

| 接口 | 命令行 | 说明 |
|---|---|---|
| `GetSongDetail` | `kite song detail` | 歌曲详情 |
| `GetSongURL` | `kite song url` | 歌曲播放地址 |
| `GetLyric` | `kite song lyric` | 歌词 |
| `Like` | `kite song like` | 红心/取消红心(写操作,登录态) |
| `Trash` | `kite song trash` | 丢进垃圾桶,降推荐权重(写操作,登录态) |
| `DisallowRecommend` | `kite song disallow-recommend` | 标记不感兴趣,影响日推(写操作,登录态) |
| `CheckAvailable` | `kite song check-available` | 检查歌曲是否可用 |
| `LikedList` | `kite song liked-list` | 红心歌曲列表(登录态) |
| `QualityDetail` | `kite song quality-detail` | 音质详情(eapi) |
| `LikeCount` | `kite song like-count` | 红心数(eapi) |
| `IsLike` | `kite song is-like` | 是否已红心(登录态) |
| `DynamicCover` | `kite song dynamic-cover` | 动态封面(eapi) |
| `ChorusTime` | `kite song chorus-time` | 副歌(eapi) |
| `CreatorInfo` | `kite song creator-info` | 创作者(eapi) |
| `GetWordLyric` | `kite song word-lyric` | 逐字歌词 |
| `SimilarSongs` | `kite song similar-songs` | 相似歌曲 |

### 用户(UserService,12 个)

| 接口 | 命令行 | 说明 |
|---|---|---|
| `Account` | `kite user account` | 当前账号信息(登录态) |
| `Detail` | `kite user detail` | 用户详情 |
| `SubCount` | `kite user sub-count` | 当前用户计数(登录态) |
| `UserPlaylist` | `kite user playlists` | 用户歌单 |
| `DetailByName` | `kite user detail-by-name` | 按昵称查用户 ID |
| `FollowEachOther` | `kite user follow-each-other` | 是否互相关注 |
| `Follows` | `kite user follows` | 用户关注列表 |
| `Followeds` | `kite user followeds` | 用户粉丝列表 |
| `Events` | `kite user events` | 用户动态 |
| `Record` | `kite user record` | 听歌排行 |
| `Level` | `kite user level` | 用户等级 |
| `SimilarUsers` | `kite user similar` | 相似用户(按歌曲) |

### 专辑(AlbumService,9 个)

| 接口 | 命令行 | 说明 |
|---|---|---|
| `GetAlbum` | `kite album detail` | 专辑详情 |
| `NewAlbumShelf` | `kite album shelf` | 新碟上架 |
| `NewestAlbums` | `kite album newest` | 最新专辑 |
| `AllNewAlbums` | `kite album all` | 全部新碟 |
| `SubscribedAlbums` | `kite album subscribed` | 已收藏专辑列表(登录态) |
| `AlbumDynamic` | `kite album dynamic` | 专辑动态信息(收藏数等) |
| `AlbumSongQuality` | `kite album song-quality` | 专辑歌曲音质 |
| `SubscribeAlbum` | `kite album subscribe` | 收藏专辑(写操作,登录态) |
| `UnsubscribeAlbum` | `kite album unsubscribe` | 取消收藏专辑(写操作,登录态) |

### 歌手(ArtistService,9 个)

| 接口 | 命令行 | 说明 |
|---|---|---|
| `GetArtist` | `kite artist detail` | 歌手详情 |
| `AllSongs` | `kite artist songs` | 歌手全部歌曲 |
| `TopSongs` | `kite artist top-songs` | 歌手热门歌曲 |
| `Albums` | `kite artist albums` | 歌手专辑列表 |
| `Desc` | `kite artist desc` | 歌手简介 |
| `Similar` | `kite artist similar` | 相似歌手 |
| `Fans` | `kite artist fans` | 歌手粉丝 |
| `TopArtists` | `kite artist toplist` | 歌手榜 |
| `ArtistSubscribe` | `kite artist subscribe` | 收藏/取消收藏歌手(写操作,登录态) |

### 登录(AuthService,6 个)

| 接口 | 命令行 | 说明 |
|---|---|---|
| `SendCaptcha` | `kite send-captcha` | 发送短信验证码 |
| `LoginByCellphone` | `kite login-cellphone` | 手机号验证码登录 |
| `LoginQrcode` | `kite login` | 扫码登录,cookie 持久化到本地配置目录 |
| `CheckQrcode` | `kite login` | 扫码登录,cookie 持久化到本地配置目录 |
| `LoginStatus` | `kite login-status` | 查看当前登录态 |
| `Logout` | `kite logout` | 登出并删除本地会话文件 |

### 搜索(SearchService,5 个)

| 接口 | 命令行 | 说明 |
|---|---|---|
| `Search` | `kite search` | 搜索(默认单曲) |
| `Suggest` | `kite search suggest` | 搜索建议 |
| `Hot` | `kite search hot` | 热搜列表 |
| `HotDetail` | `kite search hot-detail` | 热搜详情 |
| `DefaultKeyword` | `kite search default-keyword` | 默认搜索关键词 |

### 推荐(RecommendService,4 个)

| 接口 | 命令行 | 说明 |
|---|---|---|
| `GetDailyRecommend` | `kite recommend daily-songs` | 每日推荐歌曲 |
| `DailyRecommendPlaylists` | `kite recommend daily-playlists` | 每日推荐歌单(登录态) |
| `RecommendPlaylists` | `kite recommend playlists` | 推荐歌单 |
| `RecommendNewSongs` | `kite recommend new-songs` | 推荐新歌 |

### 私人 FM(FMService,1 个)

| 接口 | 命令行 | 说明 |
|---|---|---|
| `GetPersonalFM` | `kite fm` | 私人 FM |

## 接口域:在做与规划

| 域 | 状态 | 追踪 | 说明 |
|---|---|---|---|
| 评论 | 规划中 | [#28](https://github.com/VOD-Studio/kite/issues/28) | — |
| MV | 规划中 | [#32](https://github.com/VOD-Studio/kite/issues/32) | — |
| 排行榜 | 规划中 | [#29](https://github.com/VOD-Studio/kite/issues/29) | — |
| 电台 | 规划中 | [#33](https://github.com/VOD-Studio/kite/issues/33) | — |
| 云盘 | 规划中 | [#30](https://github.com/VOD-Studio/kite/issues/30) | — |
| 签到与云贝 | 规划中 | [#31](https://github.com/VOD-Studio/kite/issues/31) | 可能只登记不接 CLI |

## 平台与工程

### 规划中

- **P1 去重**([#64](https://github.com/VOD-Studio/kite/issues/64)):无行为变化的重构:收敛请求引擎入口,统一 endpoint 声明方式,压缩新增一个接口需要手写的位置数。先于多源地基。
- **P2 多源地基**([#65](https://github.com/VOD-Studio/kite/issues/65)):领域模型、小而可选的能力接口、provider 注册。会有破坏性变更(命令行的 --id 变成 provider:id,proto 拆分)。
- **P3 第二个音乐源**([#63](https://github.com/VOD-Studio/kite/issues/63)):选与网易云差异最大的一个,用来验证抽象。所有音乐源都会接入,顺序未定。
- **P4 终端界面(TUI)**([#63](https://github.com/VOD-Studio/kite/issues/63)):建在多源地基之上的完整终端客户端。
- **发布到 Buf Schema Registry**([#66](https://github.com/VOD-Studio/kite/issues/66)):让其它语言的项目直接依赖 proto。待确定组织名与发布凭据。
- **服务加固**([#67](https://github.com/VOD-Studio/kite/issues/67)):内置 TLS、限流、多令牌与调用方身份。

### 已完成

- **gRPC 错误契约**([#55](https://github.com/VOD-Studio/kite/issues/55)):错误以标准 gRPC status 加 google.rpc.ErrorInfo 返回,reason 稳定可依赖;REST 同步映射为 401/404/429/503 等。契约见 docs/grpc-errors.md。
- **服务访问鉴权与健康检查**([#59](https://github.com/VOD-Studio/kite/issues/59)):bearer token 鉴权(gRPC 与 REST 同一套规则,reflection 也受保护),默认只监听本机回环,对外监听必须配令牌;标准 gRPC 健康检查与 REST /healthz。
- **Rust 互操作测试**([#60](https://github.com/VOD-Studio/kite/issues/60)):CI 里用 proto 生成 tonic 客户端,对真实运行的服务端验证鉴权与错误契约;文档里的 Rust 示例会被编译并执行。
- **发布与交付**([#45](https://github.com/VOD-Studio/kite/issues/45)、[#46](https://github.com/VOD-Studio/kite/issues/46)、[#49](https://github.com/VOD-Studio/kite/issues/49)、[#58](https://github.com/VOD-Studio/kite/issues/58)、[#62](https://github.com/VOD-Studio/kite/issues/62)):唯一 CI 门禁,合并 release PR 后自动发布 CLI 与 kite-server 的 6 个平台归档、ghcr 镜像,镜像与校验和经 cosign 签名。
- **请求引擎稳定性修复**([#51](https://github.com/VOD-Studio/kite/issues/51)、[#53](https://github.com/VOD-Studio/kite/issues/53)):修复 cookie 请求路径的缓存串号、熔断误判、服务端登录态接口失效;私人 FM 不再缓存。
- **进度看板**([#68](https://github.com/VOD-Studio/kite/issues/68)):本页。接口清单由代码生成并有测试守护,不会和代码脱节;同步到博客由维护者按需手动触发。

## 这一页怎么来的

接口数量与清单来自 proto 描述符,命令对应关系来自命令行的注解,二者在 CI 里有测试保证一一对应;在做与规划的事项来自仓库里的 `docs/status/roadmap.yaml`,并有测试保证它不会和 proto 矛盾(比如接口已经存在却还写着「规划中」)。

这一页本身由仓库里的 `cmd/kite-status` 生成;生成物和数据来源一起入库,改了接口或命令却没刷新,测试会失败。

源码与全部记录见 [VOD-Studio/kite](https://github.com/VOD-Studio/kite),发布记录见 [Releases](https://github.com/VOD-Studio/kite/releases)。
