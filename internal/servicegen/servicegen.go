// Package servicegen 从 proto 生成 internal/service 的一行转发胶水。
//
// service 的绝大多数方法恒一行转发 engine.Execute / engine.ExecuteWithCookie,
// 手写它们唯一的产出是复制粘贴漂移。本包以 proto 编译进 gen/ 的 descriptor
// 为准逐 rpc 生成;每个 rpc 必须在 spec 里显式登记为「转发到某 endpoint 变量」
// 或「手写」,未登记即报错——新增 rpc 不可能静默漏掉 service 层(新鲜度测试
// 会红,见 internal/service 的 freshness 测试)。
//
// 产物是 internal/service/<service>_gen.go(提交进版本控制,同 gen/ 与
// docs/cmd/ 的策略);Server 结构体、构造器与手写方法留在同名 .go 文件。
package servicegen

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// File 是一个生成产物。
type File struct {
	// Name 是相对 internal/service 的文件名(如 song_gen.go)。
	Name string
	// Content 是格式化后的 Go 源码。
	Content []byte
}

// forward 声明一个 rpc 的转发目标。
type forward struct {
	// Var 是 endpoint 包级变量名(如 Detail)。
	Var string
	// Override 为 true 时走 engine.ExecuteWithCookie(cookie 从 ctx 取,
	// 写操作 / 特定登录态接口);否则 engine.Execute(会话池 + 缓存)。
	Override bool
}

// serviceSpec 声明一个 proto service 如何落到 service 层。
type serviceSpec struct {
	// File 是 proto 文件 descriptor(services 数量必须为 1)。
	File protoreflect.FileDescriptor
	// PkgAlias / PkgPath 是 endpoint 包的导入别名与路径。
	PkgAlias, PkgPath string
	// Forwards 是 rpc 名 → 转发声明;未列出的 rpc 必须在 Custom,否则报错。
	Forwards map[string]forward
	// Custom 是保持手写的 rpc 名(组合调用、带 session 副作用等)。
	Custom map[string]bool
	// CustomReason 进生成失败信息,解释为什么这些 rpc 不生成。
	CustomReason string
}

const endpointRoot = "github.com/VOD-Studio/kite/internal/netease/endpoint/"

// specs 按 proto 文件顺序排列,保证生成物稳定。
var specs = []serviceSpec{
	{
		File: mmpb.File_netease_music_v1_fm_proto, PkgAlias: "fmendpoint", PkgPath: endpointRoot + "fm",
		Forwards: map[string]forward{
			"GetPersonalFM": {Var: "GetPersonalFM"},
		},
	},
	{
		File: mmpb.File_netease_music_v1_artist_proto, PkgAlias: "artistendpoint", PkgPath: endpointRoot + "artist",
		Forwards: map[string]forward{
			"GetArtist":       {Var: "GetArtist"},
			"AllSongs":        {Var: "AllSongs"},
			"TopSongs":        {Var: "TopSongs"},
			"Albums":          {Var: "Albums"},
			"Desc":            {Var: "Desc"},
			"Similar":         {Var: "Similar"},
			"Fans":            {Var: "Fans"},
			"TopArtists":      {Var: "TopArtists"},
			"ArtistSubscribe": {Var: "Subscribe", Override: true},
		},
	},
	{
		File: mmpb.File_netease_music_v1_auth_proto, PkgAlias: "authendpoint", PkgPath: endpointRoot + "auth",
		Forwards: map[string]forward{
			"SendCaptcha": {Var: "SendCaptcha", Override: true},
			"LoginQrcode": {Var: "LoginQrcode", Override: true},
			"Logout":      {Var: "Logout", Override: true},
		},
		Custom: map[string]bool{
			"LoginByCellphone": true, "CheckQrcode": true, "LoginStatus": true, "RefreshLogin": true,
		},
		CustomReason: "需要 Set-Cookie 建/续 session 或回填 ctx cookie,手写在 auth.go",
	},
	{
		File: mmpb.File_netease_music_v1_album_proto, PkgAlias: "albumendpoint", PkgPath: endpointRoot + "album",
		Forwards: map[string]forward{
			"GetAlbum":         {Var: "GetAlbum"},
			"NewAlbumShelf":    {Var: "NewAlbumShelf"},
			"NewestAlbums":     {Var: "NewestAlbums"},
			"AllNewAlbums":     {Var: "AllNewAlbums"},
			"SubscribedAlbums": {Var: "SubscribedAlbums", Override: true},
			"AlbumDynamic":     {Var: "AlbumDynamic"},
			"AlbumSongQuality": {Var: "AlbumSongQuality"},
			"SubscribeAlbum":   {Var: "Subscribe", Override: true},
			"UnsubscribeAlbum": {Var: "Unsubscribe", Override: true},
		},
	},
	{
		File: mmpb.File_netease_music_v1_recommend_proto, PkgAlias: "recommendendpoint", PkgPath: endpointRoot + "recommend",
		Forwards: map[string]forward{
			"GetDailyRecommend":       {Var: "GetDailyRecommend"},
			"DailyRecommendPlaylists": {Var: "DailyRecommendPlaylists", Override: true},
			"RecommendPlaylists":      {Var: "RecommendPlaylists"},
			"RecommendNewSongs":       {Var: "RecommendNewSongs"},
		},
	},
	{
		File: mmpb.File_netease_music_v1_playlist_proto, PkgAlias: "playlistendpoint", PkgPath: endpointRoot + "playlist",
		Forwards: map[string]forward{
			"GetPlaylist":              {Var: "GetPlaylist"},
			"HighQuality":              {Var: "HighQuality"},
			"HighQualityTags":          {Var: "HighQualityTags"},
			"CatList":                  {Var: "CatList"},
			"BrowseHot":                {Var: "BrowseHot"},
			"Subscribers":              {Var: "Subscribers"},
			"Subscribe":                {Var: "Subscribe", Override: true},
			"Create":                   {Var: "Create", Override: true},
			"Delete":                   {Var: "Delete", Override: true},
			"UpdateName":               {Var: "UpdateName", Override: true},
			"UpdateDesc":               {Var: "UpdateDesc", Override: true},
			"UpdateTags":               {Var: "UpdateTags", Override: true},
			"UpdateTracks":             {Var: "UpdateTracks", Override: true},
			"SimilarPlaylists":         {Var: "SimilarPlaylists"},
			"RelatedPlaylistRecommend": {Var: "RelatedPlaylistRecommend"},
		},
		Custom: map[string]bool{
			"AllTracks": true,
		},
		CustomReason: "按 trackIds 分页的组合调用(compose.PlaylistTracks),手写在 playlist.go",
	},
	{
		File: mmpb.File_netease_music_v1_search_proto, PkgAlias: "searchendpoint", PkgPath: endpointRoot + "search",
		Forwards: map[string]forward{
			"Search":         {Var: "Search"},
			"Suggest":        {Var: "Suggest"},
			"Hot":            {Var: "Hot"},
			"HotDetail":      {Var: "HotDetail"},
			"DefaultKeyword": {Var: "DefaultKeyword"},
		},
	},
	{
		File: mmpb.File_netease_music_v1_song_proto, PkgAlias: "songendpoint", PkgPath: endpointRoot + "song",
		Forwards: map[string]forward{
			"GetSongDetail":     {Var: "Detail"},
			"GetSongURL":        {Var: "URL"},
			"GetLyric":          {Var: "Lyric"},
			"Like":              {Var: "Like", Override: true},
			"Trash":             {Var: "Trash", Override: true},
			"DisallowRecommend": {Var: "DisallowRecommend", Override: true},
			"CheckAvailable":    {Var: "CheckAvailable"},
			"LikedList":         {Var: "LikedList"},
			"QualityDetail":     {Var: "QualityDetail"},
			"LikeCount":         {Var: "LikeCount"},
			"IsLike":            {Var: "IsLike", Override: true},
			"DynamicCover":      {Var: "DynamicCover"},
			"ChorusTime":        {Var: "ChorusTime"},
			"CreatorInfo":       {Var: "CreatorInfo"},
			"GetWordLyric":      {Var: "WordLyricEP"},
			"SimilarSongs":      {Var: "SimilarSongs"},
		},
	},
	{
		File: mmpb.File_netease_music_v1_user_proto, PkgAlias: "userendpoint", PkgPath: endpointRoot + "user",
		Forwards: map[string]forward{
			"Account":         {Var: "Account", Override: true},
			"Detail":          {Var: "Detail"},
			"SubCount":        {Var: "SubCount"},
			"DetailByName":    {Var: "DetailByName", Override: true},
			"FollowEachOther": {Var: "FollowEachOther", Override: true},
			"Follows":         {Var: "Follows"},
			"Followeds":       {Var: "Followeds"},
			"Events":          {Var: "Events"},
			"Record":          {Var: "Record"},
			"Level":           {Var: "Level"},
			"SimilarUsers":    {Var: "SimilarUsers"},
		},
		Custom: map[string]bool{
			"UserPlaylist": true,
		},
		CustomReason: "取全量后筛选分页的组合调用(compose.UserPlaylists),手写在 user.go",
	},
}

// Generate 生成全部 service 转发文件。proto 里出现未登记的 rpc 时返回错误——
// 调用方(生成器 / 新鲜度测试)据此强制显式选择转发或手写。
func Generate() ([]File, error) {
	var out []File
	for _, spec := range specs {
		services := spec.File.Services()
		if services.Len() != 1 {
			return nil, fmt.Errorf("%s: 期望恰好 1 个 service,得到 %d", spec.File.Path(), services.Len())
		}
		svc := services.Get(0)
		f, err := generateService(spec, svc)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec.File.Path(), err)
		}
		if f != nil {
			out = append(out, *f)
		}
	}
	return out, nil
}

// generateService 生成单个 service 的转发文件;没有可生成方法(全手写)时返回 nil。
func generateService(spec serviceSpec, svc protoreflect.ServiceDescriptor) (*File, error) {
	receiver := strings.TrimSuffix(string(svc.Name()), "Service") + "Server"
	fileStem := strings.ToLower(strings.TrimSuffix(receiver, "Server"))

	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by kite-gen-service; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "// 由 proto 的 %s 生成:每方法一行转发 engine。重新生成:make service-gen。\n", svc.FullName())
	fmt.Fprintf(&b, "// Server 结构体 / 构造器%s在 %s.go 手写。\n\n", customNote(spec), fileStem)
	b.WriteString("package service\n\nimport (\n\t\"context\"\n\n\tmmpb \"github.com/VOD-Studio/kite/gen/go/netease/music/v1\"\n")
	fmt.Fprintf(&b, "\t%[1]s \"%[2]s\"\n", spec.PkgAlias, spec.PkgPath)
	b.WriteString("\t\"github.com/VOD-Studio/kite/internal/netease/engine\"\n)\n\n")

	methods := svc.Methods()
	emitted := 0
	for i := 0; i < methods.Len(); i++ {
		m := methods.Get(i)
		name := string(m.Name())
		fwd, ok := spec.Forwards[name]
		if !ok {
			if spec.Custom[name] {
				continue
			}
			return nil, fmt.Errorf("rpc %s 未登记:在 internal/servicegen 的 Forwards 声明转发目标,或加入 Custom 手写(%s)", name, spec.CustomReason)
		}
		entry := "engine.Execute"
		if fwd.Override {
			entry = "engine.ExecuteWithCookie"
		}
		in, out := m.Input().Name(), m.Output().Name()
		fmt.Fprintf(&b, "// %s 转发 %s.%s(语义注释见 proto 与 endpoint 声明)。\n", name, spec.PkgAlias, fwd.Var)
		fmt.Fprintf(&b, "func (s *%s) %s(ctx context.Context, req *mmpb.%s) (*mmpb.%s, error) {\n", receiver, name, in, out)
		fmt.Fprintf(&b, "\treturn %s(s.eng, ctx, %s.%s, req)\n", entry, spec.PkgAlias, fwd.Var)
		b.WriteString("}\n\n")
		emitted++
	}
	if emitted == 0 {
		return nil, nil
	}

	// Forwards 里登记了 proto 不存在的 rpc(拼错 / rpc 改名)也要报出来。
	for name := range spec.Forwards {
		if methods.ByName(protoreflect.Name(name)) == nil {
			return nil, fmt.Errorf("forwards 登记的 rpc %s 在 proto 里不存在(拼错或已改名)", name)
		}
	}

	src, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("生成物无法 gofmt: %w", err)
	}
	return &File{Name: fileStem + "_gen.go", Content: src}, nil
}

// customNote 生成文件头里对手写方法的说明。
func customNote(spec serviceSpec) string {
	if len(spec.Custom) == 0 {
		return ""
	}
	names := make([]string, 0, len(spec.Custom))
	for n := range spec.Custom {
		names = append(names, n)
	}
	sort.Strings(names)
	return "与手写方法(" + strings.Join(names, ", ") + ")"
}
