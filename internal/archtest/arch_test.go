// Package archtest 守护包依赖方向。
//
// 架构边界写在 AGENTS.md 里,但只有被测试守护的边界才不会腐烂。本测试用 go/parser
// 扫描非测试源文件的 import,违反规则即红。新增层(如 internal/tui)时在 rules 追加一行。
package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const module = "github.com/VOD-Studio/kite/"

// rule 声明:from 前缀下的包不得 import forbidden 前缀下的包。
type rule struct {
	from      string
	forbidden []string
	why       string
}

var rules = []rule{
	{
		from:      "internal/netease/",
		forbidden: []string{"internal/cli", "internal/tui", "internal/service", "internal/server", "internal/localstate", "internal/recall"},
		why:       "netease 是核心层,只能被上层依赖,不能反向依赖任何上层或本地状态",
	},
	{
		from:      "internal/localstate",
		forbidden: []string{"internal/cli", "internal/tui", "internal/service", "internal/server"},
		why:       "localstate 是 CLI/TUI 共享的本地文件层,不得依赖任一前端",
	},
	{
		from:      "internal/recall",
		forbidden: []string{"internal/cli", "internal/tui", "internal/service", "internal/server"},
		why:       "recall 是 CLI/TUI 共享的召回池,不得依赖任一前端",
	},
	{
		from:      "internal/cli",
		forbidden: []string{"internal/tui", "internal/service", "internal/server"},
		why:       "CLI 直连 engine/endpoint,不经 gRPC adapter(service/server),也不依赖 TUI",
	},
	{
		from:      "internal/tui",
		forbidden: []string{"internal/cli", "internal/service", "internal/server"},
		why:       "TUI 与 CLI 平行不耦合;共享能力必须先提到 CLI 之外的包",
	},
	{
		from:      "internal/service",
		forbidden: []string{"internal/cli", "internal/tui", "internal/server"},
		why:       "service 是 gRPC adapter,不得依赖前端或装配层",
	},
	{
		from:      "internal/server",
		forbidden: []string{"internal/cli", "internal/tui"},
		why:       "server 不得依赖前端",
	},
	{
		from:      "cmd/server",
		forbidden: []string{"internal/cli", "internal/tui"},
		why:       "服务端二进制不应链接前端代码",
	},
	{
		from:      "cmd/kite-probe",
		forbidden: []string{"internal"},
		why:       "部署探针只通过公开 REST/gRPC 协议验证服务,不能绕过真实适配器和鉴权",
	},
}

// under 判断 path 是否等于 prefix 或在其子目录下(按路径段匹配,避免 internal/cli 误匹配 internal/client)。
func under(path, prefix string) bool {
	prefix = strings.TrimSuffix(prefix, "/")
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func TestDependencyDirection(t *testing.T) {
	root := filepath.Join("..", "..")

	var violations []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "gen", "node_modules", "tmp", "target":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		rel = filepath.ToSlash(rel)

		f, perr := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		require.NoError(t, perr, p)
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(path, module) {
				continue
			}
			target := strings.TrimPrefix(path, module)
			for _, r := range rules {
				if !under(rel, r.from) {
					continue
				}
				for _, bad := range r.forbidden {
					if under(target, bad) {
						violations = append(violations, rel+" → "+target+"  ("+r.why+")")
					}
				}
			}
		}
		return nil
	})
	require.NoError(t, err)

	sort.Strings(violations)
	require.Empty(t, violations, "依赖方向违规:\n%s", strings.Join(violations, "\n"))
}
