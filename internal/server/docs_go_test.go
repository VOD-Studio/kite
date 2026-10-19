package server

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

var goBlock = regexp.MustCompile("(?s)```go\n(.*?)```")

// docs/integration.md 的 Go 示例必须逐字等于 interop/go/kite.go(该文件在 CI 里对真实服务端编译运行)。
// 要改示例:先改文档,再把代码块原样同步到 kite.go。
func TestDocsGoExampleMatchesInterop(t *testing.T) {
	doc, err := os.ReadFile("../../docs/integration.md")
	require.NoError(t, err)
	blocks := goBlock.FindAllStringSubmatch(string(doc), -1)
	require.Len(t, blocks, 1, "docs/integration.md 应恰有一个 go 代码块")

	src, err := os.ReadFile("../../interop/go/kite.go")
	require.NoError(t, err)
	require.Equal(t, blocks[0][1], string(src), "docs/integration.md 与 interop/go/kite.go 不一致:先改文档,再原样同步")
}

var llmsRepoLink = regexp.MustCompile(`https://(?:raw\.githubusercontent\.com/VOD-Studio/kite/main|github\.com/VOD-Studio/kite/tree/main)/([^)\s]+)`)

// llms.txt 指向本仓库的链接必须都存在,文档改名 / 删除时这里会红。
func TestLlmsTxtLinksExist(t *testing.T) {
	raw, err := os.ReadFile("../../llms.txt")
	require.NoError(t, err)
	links := llmsRepoLink.FindAllStringSubmatch(string(raw), -1)
	require.NotEmpty(t, links)
	for _, m := range links {
		_, err := os.Stat("../../" + m[1])
		require.NoError(t, err, "llms.txt 链接指向不存在的路径: %s", m[1])
	}
}
