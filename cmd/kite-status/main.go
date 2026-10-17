// Package main 生成 kite 的开发进度看板(docs/status/)。
//
// 数据来源只有三处,没有手写的接口清单:
//   - proto 描述符:已建模的 service / rpc(唯一真相);
//   - CLI 命令树的 rpc 注解(kit.RpcsAnnotationKey):每个 rpc 对应哪条命令、命令的一句话说明;
//   - docs/status/roadmap.yaml:正在做 / 规划中的事项与各域中文名(手写,有守护测试)。
//
// 产物:
//   - progress.md  进度看板正文(发布到博客的就是它);
//   - status.json  同一份数据的机器可读形式,附带发布所需的文章元信息。
//
// 产物入库,TestStatusFreshness 守护它们与代码不脱节,风格同 cmd/kite-docs。
//
// 用法:go run ./cmd/kite-status [输出目录](默认 docs/status)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	// 导入即把全部 proto 文件注册进 protoregistry,下面据此动态发现 service,不硬编码清单。
	_ "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/cli"
	"github.com/VOD-Studio/kite/internal/cli/kit"
)

const (
	protoPackage = "netease.music.v1"
	repoURL      = "https://github.com/VOD-Studio/kite"
)

// 事项状态。
const (
	statusDone       = "done"
	statusInProgress = "in_progress"
	statusPlanned    = "planned"
)

var statusLabel = map[string]string{
	statusDone:       "已完成",
	statusInProgress: "进行中",
	statusPlanned:    "规划中",
}

// Roadmap 是 docs/status/roadmap.yaml 的结构。
type Roadmap struct {
	Post      Post     `yaml:"post" json:"post"`
	Blueprint int      `yaml:"blueprint_total" json:"blueprintTotal"`
	Domains   []Domain `yaml:"domains" json:"domains"`
	Tracks    []Track  `yaml:"tracks" json:"tracks"`
}

// Post 是看板文章在博客里的元信息。
type Post struct {
	// ID 是文章在博客里的 ID,发布脚本据此更新同一篇文章(首次创建由人工完成,见 scripts/publish-status.sh)。
	ID      string   `yaml:"id" json:"id"`
	Slug    string   `yaml:"slug" json:"slug"`
	Title   string   `yaml:"title" json:"title"`
	Excerpt string   `yaml:"excerpt" json:"excerpt"`
	Tags    []string `yaml:"tags" json:"tags"`
}

// Domain 是一个接口域。Service 非空表示它认领了 proto 里的一个 service。
type Domain struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Status  string `yaml:"status" json:"status"`
	Service string `yaml:"service,omitempty" json:"service,omitempty"`
	Issue   int    `yaml:"issue,omitempty" json:"issue,omitempty"`
	Note    string `yaml:"note,omitempty" json:"note,omitempty"`
}

// Track 是平台 / 工程路线上的一件事。
type Track struct {
	ID     string `yaml:"id" json:"id"`
	Name   string `yaml:"name" json:"name"`
	Status string `yaml:"status" json:"status"`
	Refs   []int  `yaml:"refs,omitempty" json:"refs,omitempty"`
	Note   string `yaml:"note,omitempty" json:"note,omitempty"`
}

// RPC 是一个已建模的接口。
type RPC struct {
	Method  string `json:"method"`
	Command string `json:"command,omitempty"` // 主命令,如 "kite song detail";无命令为空
	Short   string `json:"short,omitempty"`   // 主命令的一句话说明
	// AlsoUsedBy 是其它同样调用该 rpc 的命令(如 download 也会查详情)。
	AlsoUsedBy []string `json:"alsoUsedBy,omitempty"`
}

// ServiceView 是一个 proto service 及其归属的域。
type ServiceView struct {
	Service string `json:"service"`
	Domain  string `json:"domain"` // 域中文名
	RPCs    []RPC  `json:"rpcs"`
}

// Report 是渲染所需的全部数据,也是 status.json 的结构。
type Report struct {
	SchemaVersion int           `json:"schemaVersion"`
	Post          Post          `json:"post"`
	TotalRPCs     int           `json:"totalRpcs"`
	RPCsWithCLI   int           `json:"rpcsWithCli"`
	Blueprint     int           `json:"blueprintTotal"`
	Services      []ServiceView `json:"services"`
	Domains       []Domain      `json:"domains"`
	Tracks        []Track       `json:"tracks"`
}

func main() {
	outDir := "docs/status"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := run(outDir); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
	fmt.Printf("已生成进度看板到 %s\n", outDir)
}

func run(outDir string) error {
	roadmap, err := loadRoadmap(filepath.Join(outDir, "roadmap.yaml"))
	if err != nil {
		return err
	}
	report, err := buildReport(roadmap, protoServices(), cliCommands(cli.NewRootCommand()))
	if err != nil {
		return err
	}
	md, js, err := render(report)
	if err != nil {
		return err
	}
	// 生成物要入库、要被所有人读到,0644 是有意的;outDir 来自命令行参数,本工具只在开发 / CI 里本地运行。
	if err := os.WriteFile(filepath.Join(outDir, "progress.md"), md, 0o644); err != nil { //nolint:gosec // 入库生成物需可读
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "status.json"), js, 0o644) //nolint:gosec // 同上
}

func loadRoadmap(path string) (Roadmap, error) {
	var r Roadmap
	b, err := os.ReadFile(path) //nolint:gosec // 开发工具,路径来自命令行参数
	if err != nil {
		return r, fmt.Errorf("读取 %s: %w", path, err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true) // 拼错字段名要报错,而不是被悄悄忽略
	if err := dec.Decode(&r); err != nil {
		return r, fmt.Errorf("解析 %s: %w", path, err)
	}
	return r, nil
}

// protoService 是从描述符读出的 service。
type protoService struct {
	Name    string   // 如 SongService
	Methods []string // 按 proto 声明顺序
}

// protoServices 从 protoregistry 动态发现 netease.music.v1 下的全部 service,按名字排序。
func protoServices() []protoService {
	var out []protoService
	protoregistry.GlobalFiles.RangeFilesByPackage(protoPackage, func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := 0; i < svcs.Len(); i++ {
			svc := svcs.Get(i)
			ps := protoService{Name: string(svc.Name())}
			for j := 0; j < svc.Methods().Len(); j++ {
				ps.Methods = append(ps.Methods, string(svc.Methods().Get(j).Name()))
			}
			out = append(out, ps)
		}
		return true
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// cmdUse 是一条消费了若干 rpc 的 CLI 命令。
type cmdUse struct {
	Path  string // "kite song detail"
	Short string
	NRPCs int // 该命令声明的 rpc 总数,越少越「专一」
}

// cliCommands 遍历命令树,返回 "Service/Method" → 消费它的命令们。
func cliCommands(root *cobra.Command) map[string][]cmdUse {
	out := map[string][]cmdUse{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if v, ok := c.Annotations[kit.RpcsAnnotationKey]; ok {
			rpcs := kit.ParseRpcs(v)
			for _, rpc := range rpcs {
				out[rpc] = append(out[rpc], cmdUse{Path: c.CommandPath(), Short: c.Short, NRPCs: len(rpcs)})
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return out
}

// primary 选出一个 rpc 的主命令:声明 rpc 最少(最专一)的优先,其次路径短,再按字典序,结果稳定。
func primary(uses []cmdUse) (main cmdUse, others []string) {
	sorted := append([]cmdUse(nil), uses...)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.NRPCs != b.NRPCs {
			return a.NRPCs < b.NRPCs
		}
		if len(a.Path) != len(b.Path) {
			return len(a.Path) < len(b.Path)
		}
		return a.Path < b.Path
	})
	main = sorted[0]
	for _, u := range sorted[1:] {
		others = append(others, u.Path)
	}
	return main, others
}

// validate 检查 roadmap 与 proto 的一致性,返回全部问题(空表示通过)。
//
// 这是进度看板「不说谎」的保证:
//   - proto 里的每个 service 必须被一个已上线(done / in_progress)的域认领,且只被认领一次;
//   - 域认领的 service 必须真实存在;
//   - 标 planned 的域不能已经在 proto 里有 service(做完了却还写规划中);
//   - id 唯一,状态取值合法,planned / in_progress 的事项必须有可点击的追踪编号。
func validate(r Roadmap, services []protoService) []string {
	var problems []string
	add := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }

	if r.Post.ID == "" || r.Post.Slug == "" || r.Post.Title == "" {
		add("post.id、post.slug 与 post.title 不能为空")
	}
	if r.Blueprint <= 0 {
		add("blueprint_total 必须为正数")
	}

	exists := map[string]bool{}
	for _, s := range services {
		exists[s.Name] = true
	}

	ids := map[string]bool{}
	claimed := map[string][]string{}
	checkStatus := func(kind, id, status string) {
		if _, ok := statusLabel[status]; !ok {
			add("%s %q 的 status %q 不合法(可选 done / in_progress / planned)", kind, id, status)
		}
	}
	for _, d := range r.Domains {
		if d.ID == "" || d.Name == "" {
			add("域缺少 id 或 name: %+v", d)
			continue
		}
		if ids["domain:"+d.ID] {
			add("域 id 重复: %s", d.ID)
		}
		ids["domain:"+d.ID] = true
		checkStatus("域", d.ID, d.Status)

		if d.Service != "" {
			claimed[d.Service] = append(claimed[d.Service], d.ID)
			if !exists[d.Service] {
				add("域 %s 认领了不存在的 service %s", d.ID, d.Service)
			}
			if d.Status == statusPlanned {
				add("域 %s 在 proto 里已有 service %s,不能标 planned", d.ID, d.Service)
			}
		} else if d.Status == statusDone {
			add("域 %s 标了 done 却没有认领 service", d.ID)
		}
		if d.Status != statusDone && d.Issue <= 0 {
			add("域 %s 未完成,必须填 issue 追踪编号", d.ID)
		}
	}
	for _, s := range services {
		switch owners := claimed[s.Name]; len(owners) {
		case 0:
			add("proto 里的 service %s 没有被任何域认领(在 roadmap.yaml 的 domains 里补上)", s.Name)
		case 1:
		default:
			add("service %s 被多个域认领: %s", s.Name, strings.Join(owners, ", "))
		}
	}

	for _, t := range r.Tracks {
		if t.ID == "" || t.Name == "" {
			add("事项缺少 id 或 name: %+v", t)
			continue
		}
		if ids["track:"+t.ID] {
			add("事项 id 重复: %s", t.ID)
		}
		ids["track:"+t.ID] = true
		checkStatus("事项", t.ID, t.Status)
		if len(t.Refs) == 0 {
			add("事项 %s 必须有 refs(PR / issue 编号),否则读者无处追踪", t.ID)
		}
		for _, n := range t.Refs {
			if n <= 0 {
				add("事项 %s 的 refs 含非法编号 %d", t.ID, n)
			}
		}
	}
	return problems
}

// buildReport 把三个数据源合成渲染所需的 Report。
func buildReport(r Roadmap, services []protoService, cmds map[string][]cmdUse) (Report, error) {
	if problems := validate(r, services); len(problems) > 0 {
		return Report{}, fmt.Errorf("roadmap.yaml 与 proto 不一致:\n  - %s", strings.Join(problems, "\n  - "))
	}

	domainOf := map[string]Domain{}
	for _, d := range r.Domains {
		if d.Service != "" {
			domainOf[d.Service] = d
		}
	}

	rep := Report{
		SchemaVersion: 1,
		Post:          r.Post,
		Blueprint:     r.Blueprint,
		Domains:       r.Domains,
		Tracks:        r.Tracks,
	}
	for _, s := range services {
		sv := ServiceView{Service: s.Name, Domain: domainOf[s.Name].Name}
		for _, m := range s.Methods {
			rpc := RPC{Method: m}
			if uses := cmds[s.Name+"/"+m]; len(uses) > 0 {
				main, others := primary(uses)
				rpc.Command, rpc.Short, rpc.AlsoUsedBy = main.Path, main.Short, others
				rep.RPCsWithCLI++
			}
			sv.RPCs = append(sv.RPCs, rpc)
			rep.TotalRPCs++
		}
		rep.Services = append(rep.Services, sv)
	}
	// 展示顺序:接口多的域在前,同数量按名字,稳定且让读者先看到主要能力。
	sort.SliceStable(rep.Services, func(i, j int) bool {
		a, b := rep.Services[i], rep.Services[j]
		if len(a.RPCs) != len(b.RPCs) {
			return len(a.RPCs) > len(b.RPCs)
		}
		return a.Service < b.Service
	})
	return rep, nil
}

// cell 转义 markdown 表格单元格里的竖线与换行。
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.ReplaceAll(s, "\n", " ")
}

func link(n int) string { return fmt.Sprintf("[#%d](%s/issues/%d)", n, repoURL, n) }

func refs(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = link(n)
	}
	return strings.Join(parts, "、")
}

// render 生成 progress.md 与 status.json 的字节。输出必须是输入的纯函数(不含时间、提交号),
// 否则 freshness 测试会随每次提交变红。
func render(r Report) (md, js []byte, err error) {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	pct := 100 * float64(r.TotalRPCs) / float64(r.Blueprint)

	w("kite 是一个用 Go 写的网易云音乐能力服务,同时提供命令行工具和对外的 gRPC / REST 接口," +
		"目标是让 Rust 等其它语言的项目直接使用。后续会接入更多音乐源。\n\n")
	w("这一页回答三个问题:已经开发了哪些接口,哪些在做,接下来做什么。接口清单不是手写的,由代码生成。\n\n")

	w("## 概览\n\n")
	w("| 指标 | 数值 |\n|---|---|\n")
	w("| 已建模并可用的接口(RPC) | %d 个,分布在 %d 个服务 |\n", r.TotalRPCs, len(r.Services))
	w("| 其中可通过命令行调用的 | %d / %d |\n", r.RPCsWithCLI, r.TotalRPCs)
	w("| 上游接口蓝图(估算) | 约 %d 个,当前覆盖约 %.0f%% |\n\n", r.Blueprint, pct)
	w("这里的「已完成」指:该接口在 proto 里有定义,并且至少有一条命令行命令调用它,二者的对应关系由测试守护。" +
		"它不等于上游每个字段都已覆盖。\n\n")

	w("## 已完成的接口\n\n")
	for _, s := range r.Services {
		w("### %s(%s,%d 个)\n\n", s.Domain, s.Service, len(s.RPCs))
		w("| 接口 | 命令行 | 说明 |\n|---|---|---|\n")
		for _, rpc := range s.RPCs {
			cmd := "—"
			if rpc.Command != "" {
				cmd = "`" + rpc.Command + "`"
			}
			short := rpc.Short
			if short == "" {
				short = "—"
			}
			w("| `%s` | %s | %s |\n", rpc.Method, cmd, cell(short))
		}
		w("\n")
	}

	w("## 接口域:在做与规划\n\n")
	var pending []Domain
	for _, d := range r.Domains {
		if d.Status != statusDone {
			pending = append(pending, d)
		}
	}
	if len(pending) == 0 {
		w("当前没有待接入的接口域。\n\n")
	} else {
		w("| 域 | 状态 | 追踪 | 说明 |\n|---|---|---|---|\n")
		for _, d := range pending {
			note := d.Note
			if note == "" {
				note = "—"
			}
			w("| %s | %s | %s | %s |\n", d.Name, statusLabel[d.Status], link(d.Issue), cell(note))
		}
		w("\n")
	}

	w("## 平台与工程\n\n")
	for _, st := range []string{statusInProgress, statusPlanned, statusDone} {
		var items []Track
		for _, t := range r.Tracks {
			if t.Status == st {
				items = append(items, t)
			}
		}
		if len(items) == 0 {
			continue
		}
		w("### %s\n\n", statusLabel[st])
		for _, t := range items {
			w("- **%s**(%s)", t.Name, refs(t.Refs))
			if t.Note != "" {
				w(":%s", t.Note)
			}
			w("\n")
		}
		w("\n")
	}

	w("## 这一页怎么来的\n\n")
	w("接口数量与清单来自 proto 描述符,命令对应关系来自命令行的注解,二者在 CI 里有测试保证一一对应;" +
		"在做与规划的事项来自仓库里的 `docs/status/roadmap.yaml`,并有测试保证它不会和 proto 矛盾" +
		"(比如接口已经存在却还写着「规划中」)。\n\n")
	w("这一页本身由仓库里的 `cmd/kite-status` 生成;生成物和数据来源一起入库,改了接口或命令却没刷新,测试会失败。\n\n")
	w("源码与全部记录见 [VOD-Studio/kite](%s),发布记录见 [Releases](%s/releases)。\n", repoURL, repoURL)

	js, err = json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return []byte(b.String()), append(js, '\n'), nil
}
