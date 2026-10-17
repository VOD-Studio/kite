package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/VOD-Studio/kite/internal/cli"
)

const statusDir = "../../docs/status"

// 真实的 roadmap.yaml 必须与真实的 proto 一致——进度看板「不说谎」的核心守护。
func TestRoadmapConsistentWithProto(t *testing.T) {
	r, err := loadRoadmap(filepath.Join(statusDir, "roadmap.yaml"))
	require.NoError(t, err)
	svcs := protoServices()
	require.NotEmpty(t, svcs, "proto 反射出 0 个 service,说明描述符没有被注册")

	require.Empty(t, validate(r, svcs), "docs/status/roadmap.yaml 与 proto 不一致")
}

// 守护本身要能抓到问题:逐条构造违规输入。
func TestValidateCatchesInconsistency(t *testing.T) {
	svcs := []protoService{{Name: "SongService"}, {Name: "AlbumService"}}
	base := func() Roadmap {
		return Roadmap{
			Post:      Post{ID: "id", Slug: "s", Title: "t"},
			Blueprint: 100,
			Domains: []Domain{
				{ID: "song", Name: "歌曲", Status: statusDone, Service: "SongService"},
				{ID: "album", Name: "专辑", Status: statusDone, Service: "AlbumService"},
			},
			Tracks: []Track{{ID: "x", Name: "x", Status: statusPlanned, Refs: []int{1}}},
		}
	}
	require.Empty(t, validate(base(), svcs), "基线必须通过,否则下面的用例没有意义")

	tests := []struct {
		name   string
		mutate func(*Roadmap)
		want   string
	}{
		{"proto 有 service 但没域认领", func(r *Roadmap) { r.Domains = r.Domains[:1] }, "AlbumService 没有被任何域认领"},
		{"域认领了不存在的 service", func(r *Roadmap) { r.Domains[0].Service = "GhostService" }, "不存在的 service GhostService"},
		{"接口已存在却标 planned", func(r *Roadmap) { r.Domains[1].Status = statusPlanned; r.Domains[1].Issue = 5 }, "不能标 planned"},
		{"标 done 却没认领 service", func(r *Roadmap) { r.Domains = append(r.Domains, Domain{ID: "mv", Name: "MV", Status: statusDone}) }, "标了 done 却没有认领 service"},
		{"同一 service 被两个域认领", func(r *Roadmap) { r.Domains[1].Service = "SongService" }, "被多个域认领"},
		{"未完成的域没有 issue", func(r *Roadmap) {
			r.Domains = append(r.Domains, Domain{ID: "mv", Name: "MV", Status: statusPlanned})
		}, "必须填 issue"},
		{"状态取值非法", func(r *Roadmap) { r.Tracks[0].Status = "doing" }, "不合法"},
		{"事项没有 refs", func(r *Roadmap) { r.Tracks[0].Refs = nil }, "必须有 refs"},
		{"id 重复", func(r *Roadmap) { r.Tracks = append(r.Tracks, r.Tracks[0]) }, "事项 id 重复"},
		{"文章元信息缺失", func(r *Roadmap) { r.Post.Slug = "" }, "post.slug"},
		{"文章 ID 缺失", func(r *Roadmap) { r.Post.ID = "" }, "post.id"},
		{"蓝图总数缺失", func(r *Roadmap) { r.Blueprint = 0 }, "blueprint_total"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base()
			tt.mutate(&r)
			problems := validate(r, svcs)
			require.NotEmpty(t, problems)
			require.Contains(t, strings.Join(problems, "\n"), tt.want)
		})
	}
}

// 拼错字段名必须报错,而不是被悄悄忽略(否则一个手滑就让事项从看板上消失)。
func TestLoadRoadmapRejectsUnknownFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "roadmap.yaml")
	require.NoError(t, os.WriteFile(p, []byte("post: {id: x, slug: a, title: b}\nblueprint_total: 1\ndomans: []\n"), 0o600))
	_, err := loadRoadmap(p)
	require.Error(t, err)
}

// 入库的生成物必须与代码一致,类似 TestDocsFreshness。
// 失败时运行 `make status` 并提交 docs/status/。
func TestStatusFreshness(t *testing.T) {
	tmp := t.TempDir()
	rm, err := os.ReadFile(filepath.Join(statusDir, "roadmap.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "roadmap.yaml"), rm, 0o600)) //nolint:gosec // 测试临时目录

	require.NoError(t, run(tmp))

	for _, name := range []string{"progress.md", "status.json"} {
		want, err := os.ReadFile(filepath.Join(tmp, name))
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(statusDir, name))
		require.NoError(t, err, "缺少 docs/status/%s,运行 make status", name)
		require.Equal(t, string(want), string(got),
			"docs/status/%s 与代码不一致(新增 / 修改了 rpc、命令或 roadmap.yaml?)。运行 `make status` 并提交 docs/status/", name)
	}
}

// 看板完整性:78 个接口这类数字是算出来的,不是写死的;每个 rpc 都能对应到命令。
func TestReportCoversEveryRPC(t *testing.T) {
	r, err := loadRoadmap(filepath.Join(statusDir, "roadmap.yaml"))
	require.NoError(t, err)
	svcs := protoServices()
	rep, err := buildReport(r, svcs, cliCommands(cli.NewRootCommand()))
	require.NoError(t, err)

	want := 0
	for _, s := range svcs {
		want += len(s.Methods)
	}
	require.Equal(t, want, rep.TotalRPCs)
	require.Equal(t, rep.TotalRPCs, rep.RPCsWithCLI, "有 rpc 没有 CLI 命令:接 CLI,或在 rpcsWithoutCliAllowlist 登记后调整看板口径")
	for _, s := range rep.Services {
		require.NotEmpty(t, s.Domain, s.Service)
	}
}

// 输出必须是输入的纯函数:不含时间、提交号,否则 freshness 会随每次提交变红。
func TestRenderDeterministic(t *testing.T) {
	r, err := loadRoadmap(filepath.Join(statusDir, "roadmap.yaml"))
	require.NoError(t, err)
	build := func() ([]byte, []byte) {
		rep, err := buildReport(r, protoServices(), cliCommands(cli.NewRootCommand()))
		require.NoError(t, err)
		md, js, err := render(rep)
		require.NoError(t, err)
		return md, js
	}
	md1, js1 := build()
	md2, js2 := build()
	require.Equal(t, md1, md2)
	require.Equal(t, js1, js2)
}

func TestPrimaryPicksMostSpecificCommand(t *testing.T) {
	uses := []cmdUse{
		{Path: "kite song download", Short: "下载", NRPCs: 2},
		{Path: "kite song detail", Short: "详情", NRPCs: 1},
		{Path: "kite playlist download", Short: "整单下载", NRPCs: 3},
	}
	main, others := primary(uses)
	require.Equal(t, "kite song detail", main.Path, "声明 rpc 最少的命令最专一")
	require.ElementsMatch(t, []string{"kite song download", "kite playlist download"}, others)
}

func TestCellEscapesTableSyntax(t *testing.T) {
	require.Equal(t, `a\|b c`, cell("a|b\nc"))
}
