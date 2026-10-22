package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/VOD-Studio/kite/internal/servicegen"
)

// TestServiceGenFreshness 守护 *_gen.go 与 proto / 映射表不漂移(同 docs/cmd 的
// TestDocsFreshness 策略):重新生成必须与提交的文件逐字节一致。
//
// 新增 proto rpc 后本测试会红:在 internal/servicegen 的 spec 里登记转发目标,
// 或加入 Custom 保持手写,然后运行 make service-gen。
func TestServiceGenFreshness(t *testing.T) {
	files, err := servicegen.Generate()
	require.NoError(t, err, "生成失败通常是新 rpc 未登记,见错误信息")

	generated := map[string]bool{}
	for _, f := range files {
		generated[f.Name] = true
		got, err := os.ReadFile(f.Name)
		require.NoError(t, err, "读取 %s:先运行 make service-gen 生成", f.Name)
		require.Equal(t, string(f.Content), string(got),
			"%s 与 proto/映射表不一致:运行 make service-gen 并提交", f.Name)
	}

	// 目录里不许有生成物之外的 *_gen.go(改名/删除 service 后的残留)。
	entries, err := filepath.Glob("*_gen.go")
	require.NoError(t, err)
	for _, e := range entries {
		require.True(t, generated[filepath.Base(e)],
			"%s 不是当前生成物(生成物清单:%v),请删除", e, keysOf(generated))
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
