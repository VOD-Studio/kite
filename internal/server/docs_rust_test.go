package server

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

var rustBlock = regexp.MustCompile("(?s)```rust\n(.*?)```")

// 文档里的 Rust 示例必须逐字等于 interop/rust 里被编译、被对着真实服务端运行的模块文件。
// 否则示例会悄悄腐烂(这类问题真实发生过:示例只用字面量测过,传临时字符串时编译失败)。
// 要改示例:先改文档,再把代码块原样同步到对应的 .rs 文件。
func TestDocsRustExamplesMatchInterop(t *testing.T) {
	tests := []struct{ doc, rs string }{
		{"../../docs/grpc-errors.md", "../../interop/rust/src/classify.rs"},
		{"../../docs/server-security.md", "../../interop/rust/src/auth.rs"},
	}
	for _, tt := range tests {
		doc, err := os.ReadFile(tt.doc)
		require.NoError(t, err)
		blocks := rustBlock.FindAllStringSubmatch(string(doc), -1)
		require.Len(t, blocks, 1, "%s 应恰有一个 rust 代码块", tt.doc)

		src, err := os.ReadFile(tt.rs)
		require.NoError(t, err)
		require.Equal(t, blocks[0][1], string(src), "%s 与 %s 不一致:先改文档,再原样同步到 .rs", tt.doc, tt.rs)
	}
}
