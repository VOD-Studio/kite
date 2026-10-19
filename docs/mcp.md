# 在 AI 客户端里使用 kite(MCP)

`kite mcp` 以 [MCP](https://modelcontextprotocol.io) 服务(stdio 传输)运行,让 Claude、Cursor 等 AI 客户端直接调用网易云的只读接口:搜歌、查歌词、拿播放链接、看歌单 / 专辑 / 歌手、热搜、每日推荐。

- **在你本机运行**,请求从你的网络直接发往网易云,不经过任何第三方服务器。
- **只读**:不提供红心、收藏、编辑歌单等写操作,AI 不能替你修改账号(由测试守护)。
- **登录态用本机的 `kite login`**(环境变量 `NETEASE_COOKIE` 优先)。每次调用时读取,重新登录后无需重启客户端。

## 安装与登录

```bash
go install github.com/VOD-Studio/kite/cmd/kite@latest   # 或从 GitHub Releases 下载对应平台的 kite
kite login                                              # 可选:扫码登录,解锁 VIP 播放链接与每日推荐
```

## 配置客户端

**Claude Code**

```bash
claude mcp add kite -- kite mcp
```

**Claude Desktop**(`claude_desktop_config.json`)/ **Cursor**(`~/.cursor/mcp.json`)

```json
{
  "mcpServers": {
    "kite": { "command": "kite", "args": ["mcp"] }
  }
}
```

`kite` 不在客户端的 `PATH` 里时,`command` 写绝对路径(`which kite` 查看)。

## 工具

| 工具 | 说明 |
|---|---|
| `search_songs` | 按关键词搜索单曲 |
| `get_song` | 歌曲详情 |
| `get_lyric` | 歌词(含翻译 / 音译) |
| `get_song_url` | 播放直链;登录后按账号权益取,未登录只有免费歌 |
| `get_playlist` | 歌单详情与曲目 |
| `get_album` | 专辑详情与曲目 |
| `get_artist` | 歌手详情 |
| `get_artist_top_songs` | 歌手热门歌曲 |
| `similar_songs` | 相似歌曲 |
| `hot_searches` | 热搜关键词 |
| `daily_recommend` | 每日推荐(需登录) |

参数说明由客户端从 MCP 的工具描述自动获取。出错时(如未登录调用 `daily_recommend`)以工具错误返回,AI 能看到原因。
