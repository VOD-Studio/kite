// Package archtest 的重复样板守护。
//
// 依赖方向之外的另一类回潮:「收敛过的样板重新手写长出来」。issue #64 把
// endpoint 响应解码收敛到 engine.Decode / DecodeLoose、CLI 命令骨架收敛到
// kit.ReadCmd / WriteCmd / LoginReadCmd;本文件守护这两处不再出现手写样板,
// 与 TestRawEngineEntriesNotCalledOutsideEngine、servicegen freshness 共同
// 覆盖 P1 的去重面。
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// eachSourceFile 遍历 root 下、prefix 子树内的非测试 .go 源文件,解析后交给 fn。
// 解析失败的文件直接判红(源码必须可解析)。
func eachSourceFile(t *testing.T, prefix string, fn func(rel string, fset *token.FileSet, f *ast.File)) {
	t.Helper()
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
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
		dir, _ := filepath.Rel(root, filepath.Dir(p))
		dir = filepath.ToSlash(dir)
		if !under(dir, prefix) {
			return nil
		}
		f, perr := parser.ParseFile(fset, p, nil, 0)
		require.NoError(t, perr, p)
		relFile := dir + "/" + d.Name()
		fn(relFile, fset, f)
		return nil
	})
	require.NoError(t, err)
}

// TestEndpointDecodeUsesEngineDecode 守护:endpoint 包里不再出现
// 「if err := json.Unmarshal(raw, &resp); err != nil { return …, fmt.Errorf("解析…失败") }」
// 的手写严格解码样板——收敛到 engine.Decode(错误归类带哨兵)/ DecodeLoose。
//
// 例外形态不触发:内层值探测(`err == nil` 正向判断,如 rawToString)、
// 逐项宽容尝试(`err != nil || 其它条件`,如 eapi 音质项)、无赋值的形状嗅探
// (`if json.Unmarshal(raw, &n) == nil`)。
func TestEndpointDecodeUsesEngineDecode(t *testing.T) {
	var violations []string
	eachSourceFile(t, "internal/netease/endpoint", func(_ string, fset *token.FileSet, f *ast.File) {
		ast.Inspect(f, func(n ast.Node) bool {
			ifStmt, ok := n.(*ast.IfStmt)
			if !ok || ifStmt.Init == nil {
				return true
			}
			// Init 必须是「err := json.Unmarshal(raw, …)」。
			assign, ok := ifStmt.Init.(*ast.AssignStmt)
			if !ok || len(assign.Rhs) != 1 {
				return true
			}
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok || !isSelector(call.Fun, "Unmarshal") {
				return true
			}
			if len(call.Args) == 0 {
				return true
			}
			if id, ok := call.Args[0].(*ast.Ident); !ok || id.Name != "raw" {
				return true
			}
			// Cond 必须恰是「err != nil」(整条比较,不与其它条件组合)。
			bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
			if !ok || bin.Op != token.NEQ || !isErrNilCompare(bin) {
				return true
			}
			violations = append(violations, fset.Position(ifStmt.Pos()).String())
			return true
		})
	})
	sort.Strings(violations)
	require.Empty(t, violations,
		"endpoint 内不得手写严格解码样板,改用 engine.Decode / engine.DecodeLoose:\n%s",
		strings.Join(violations, "\n"))
}

// isErrNilCompare 判断 e 是否恰为「err != nil」的两端形态。
func isErrNilCompare(e *ast.BinaryExpr) bool {
	l, lok := e.X.(*ast.Ident)
	r, rok := e.Y.(*ast.Ident)
	return lok && rok && l.Name == "err" && r.Name == "nil"
}

// isSelector 判断 x 是否是名为 name 的选择器(如 json.Unmarshal 的 Fun)。
func isSelector(x ast.Expr, name string) bool {
	sel, ok := x.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name
}

// cliHandwrittenAllowlist 登记「ResolveID + RenderExec/ConfirmFatal 同函数」的
// 手写命令(key 为 文件#函数),每条必须写理由——构造器表达不了的解析/组合。
var cliHandwrittenAllowlist = map[string]string{
	"internal/cli/playlist/playlist.go#newUpdateTracks": "--op/--tracks 解析与操作文案在确认前完成,WriteCmd 的闭包承载不了",
	"internal/cli/song/download.go#newDownload":         "下载核心消费多个 rpc(URL/Detail/CheckAvailable)并带落盘渲染",
	"internal/cli/playlist/download.go#newDownload":     "歌单下载跨 service 组合(曲目 + 歌单名 + 逐曲 URL)",
}

// TestCliIdCommandsUseConstructors 守护:internal/cli(除 kit 本身)里,
// 「ResolveID 解析 ID + RenderExec 渲染 / ConfirmFatal 确认」的命令骨架必须走
// kit.ReadCmd / LoginReadCmd / WriteCmd 构造器;确需手写的在
// cliHandwrittenAllowlist 登记理由。
func TestCliIdCommandsUseConstructors(t *testing.T) {
	var violations []string
	eachSourceFile(t, "internal/cli", func(rel string, _ *token.FileSet, f *ast.File) {
		if under(rel, "internal/cli/kit") {
			return // 构造器实现自身
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			var hasResolve, hasRender, hasConfirm bool
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok {
					switch sel.Sel.Name {
					case "ResolveID":
						hasResolve = true
					case "RenderExec":
						hasRender = true
					case "ConfirmFatal":
						hasConfirm = true
					}
				}
				return true
			})
			if hasResolve && (hasRender || hasConfirm) {
				key := rel + "#" + fd.Name.Name
				if _, allowed := cliHandwrittenAllowlist[key]; !allowed {
					violations = append(violations, key)
				}
			}
		}
	})
	sort.Strings(violations)
	require.Empty(t, violations,
		"ID 型命令骨架请用 kit.ReadCmd / LoginReadCmd / WriteCmd;确需手写在 cliHandwrittenAllowlist 登记理由:\n%s",
		strings.Join(violations, "\n"))
}
