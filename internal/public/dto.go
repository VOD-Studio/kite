package public

import (
	"strconv"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// 以下是 /player/v1 的响应结构,即播放器组件(packages/kite-player)依赖的公开协议。
// 字段只增不改;ID 一律是字符串,为接入其它音乐源时的 provider:id 留余地。

type infoDTO struct {
	Version string `json:"version"`
	// Login 是服务账号的登录态:none(未配置)/ active / expired(失效,只能按匿名播放)。
	Login   string `json:"login"`
	Quality string `json:"quality"`
}

type artistDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type albumDTO struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Cover string `json:"cover"`
}

type trackDTO struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Artists    []artistDTO `json:"artists"`
	Album      albumDTO    `json:"album"`
	DurationMs int64       `json:"durationMs"`
	// Access 是上游的付费类型:free / vip / paid。
	Access string `json:"access"`
	// Playable 是按当前站点的登录态推断的能否播放(最终以播放链接为准)。
	Playable bool `json:"playable"`
	// Stream 是带签名的播放链接(相对本服务),请求后 302 到上游直链。
	Stream string `json:"stream"`
	// Lyrics 是歌词接口地址(相对本服务)。
	Lyrics string `json:"lyrics"`
}

type playlistDTO struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Cover      string     `json:"cover"`
	TrackCount int32      `json:"trackCount"`
	Tracks     []trackDTO `json:"tracks"`
}

type wordDTO struct {
	Time     int64  `json:"time"`
	Duration int64  `json:"duration"`
	Text     string `json:"text"`
}

type lineDTO struct {
	Time        int64     `json:"time"`
	Duration    int64     `json:"duration,omitempty"`
	Text        string    `json:"text"`
	Translation string    `json:"translation,omitempty"`
	Romanized   string    `json:"romanized,omitempty"`
	Words       []wordDTO `json:"words,omitempty"`
}

type segmentDTO struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type lyricsDTO struct {
	Lines        []lineDTO    `json:"lines"`
	Chorus       []segmentDTO `json:"chorus"`
	Instrumental bool         `json:"instrumental"`
}

type errorDTO struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// access 把网易云 fee 映射成付费类型:0 免费、8 免费(低音质)、1 VIP、4 数字专辑。
func access(fee int32) string {
	switch fee {
	case 1:
		return "vip"
	case 4:
		return "paid"
	default:
		return "free"
	}
}

// https 把上游的 http 图片 / 直链换成 https,HTTPS 站点上不触发混合内容。
func https(u string) string {
	if len(u) > 7 && u[:7] == "http://" {
		return "https://" + u[7:]
	}
	return u
}

func (s *Server) track(song *mmpb.Song, ownerActive bool) trackDTO {
	id := song.GetId()
	t := trackDTO{
		ID:         strconv.FormatInt(id, 10),
		Title:      song.GetName(),
		Artists:    []artistDTO{},
		DurationMs: song.GetDurationMs(),
		Access:     access(song.GetFee()),
		Stream:     "/player/v1/tracks/" + strconv.FormatInt(id, 10) + "/stream?" + s.signer.streamQuery(id),
		Lyrics:     "/player/v1/tracks/" + strconv.FormatInt(id, 10) + "/lyrics",
	}
	for _, a := range song.GetArtists() {
		t.Artists = append(t.Artists, artistDTO{ID: strconv.FormatInt(a.GetId(), 10), Name: a.GetName()})
	}
	if al := song.GetAlbum(); al != nil {
		t.Album = albumDTO{ID: strconv.FormatInt(al.GetId(), 10), Name: al.GetName(), Cover: https(al.GetPicUrl())}
	}
	switch t.Access {
	case "free":
		t.Playable = true
	case "vip":
		t.Playable = ownerActive
	}
	return t
}
