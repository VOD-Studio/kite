// Package model 的 MV 解码。
//
// 上游现状(2026-10 实测):独立的 /mv/url 接口已下线(weapi / eapi 均业务 404),
// 多清晰度播放地址由 /mv/detail 的 brs 字段返回;MV 排行的现行接口是 /mv/toplist
// (老 /top/mv 已业务 404),其条目顶层 duration 恒为 0、发行日期在嵌套 mv 子对象上。
package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
)

// rawMvSummary 是列表类接口(相关 MV /simi/mv 的 mvs、收藏 MV /mv/sublist 的 data)
// 的列表元素,两接口元素同构;「全部 MV」/eapi/mv/all 的 data 亦是此形状。
type rawMvSummary struct {
	ID         int64       `json:"id"`         // MV ID
	Name       string      `json:"name"`       // MV 名
	Cover      string      `json:"cover"`      // 封面URL
	PlayCount  int64       `json:"playCount"`  // 播放数
	ArtistID   int64       `json:"artistId"`   // 主歌手ID
	ArtistName string      `json:"artistName"` // 主歌手名(上游冗余字段)
	Duration   int64       `json:"duration"`   // 时长毫秒
	Artists    []rawArtist `json:"artists"`    // 歌手数组
	BriefDesc  string      `json:"briefDesc"`  // 简介
	Desc       string      `json:"desc"`       // 详细描述(可 null)
}

// rawMvDetail 是 /mv/detail 响应(data 元素)。
type rawMvDetail struct {
	ID           int64             `json:"id"`           // MV ID
	Name         string            `json:"name"`         // MV 名
	ArtistID     int64             `json:"artistId"`     // 主歌手ID
	ArtistName   string            `json:"artistName"`   // 主歌手名
	BriefDesc    string            `json:"briefDesc"`    // 简介
	Cover        string            `json:"cover"`        // 封面URL
	PlayCount    int64             `json:"playCount"`    // 播放数
	SubCount     int64             `json:"subCount"`     // 收藏数
	ShareCount   int64             `json:"shareCount"`   // 分享数
	LikeCount    int64             `json:"likeCount"`    // 点赞数
	CommentCount int64             `json:"commentCount"` // 评论数
	Duration     int64             `json:"duration"`     // 时长毫秒
	PublishTime  string            `json:"publishTime"`  // 发行日期(如 1997-10-01)
	Brs          map[string]string `json:"brs"`          // 多清晰度播放地址{分辨率:完整带签名URL}
	Artists      []rawArtist       `json:"artists"`      // 歌手数组
}

// rawMvTopItem 是 /mv/toplist 的列表元素(与摘要不同:顶层 duration 恒 0,
// 发行日期与排行信息在嵌套 mv 子对象上)。
type rawMvTopItem struct {
	ID         int64       `json:"id"`         // MV ID
	Name       string      `json:"name"`       // MV 名
	Cover      string      `json:"cover"`      // 封面URL
	PlayCount  int64       `json:"playCount"`  // 播放数
	ArtistID   int64       `json:"artistId"`   // 主歌手ID
	ArtistName string      `json:"artistName"` // 主歌手名
	Duration   int64       `json:"duration"`   // 上游恒 0(时长在视频档位上)
	Artists    []rawArtist `json:"artists"`    // 歌手数组
	Mv         struct {
		PublishTime string `json:"publishTime"` // 发行日期(嵌套对象上)
	} `json:"mv"`
	LastRank int32 `json:"lastRank"` // 上期排名(0=新入榜)
	Score    int32 `json:"score"`    // 排行分
}

// mapMvSummary 把列表元素转成 proto Mv(摘要字段)。
func mapMvSummary(s rawMvSummary) *mmpb.Mv {
	return &mmpb.Mv{
		Id: s.ID, Name: s.Name,
		Artists: MapArtists(s.Artists), ArtistName: s.ArtistName,
		CoverUrl: s.Cover, PlayCount: s.PlayCount, DurationMs: s.Duration,
		BriefDesc: firstNonEmpty(s.BriefDesc, s.Desc),
	}
}

// DecodeMvDetail 解析 MV 详情响应(/mv/detail),含多清晰度播放地址(brs)。
func DecodeMvDetail(raw json.RawMessage) (*mmpb.Mv, error) {
	var resp struct {
		Data rawMvDetail `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析 MV 详情失败: %w", err)
	}
	d := resp.Data
	// brs 的 key 是分辨率字符串("240"/"480"...),map 迭代无序,按分辨率升序输出。
	resolutions := make([]int, 0, len(d.Brs))
	urls := make(map[int]string, len(d.Brs))
	for k, u := range d.Brs {
		r, err := strconv.Atoi(k)
		if err != nil {
			continue // 非分辨率 key(如未知档位)跳过,不让单个脏档位毁掉整个响应
		}
		resolutions = append(resolutions, r)
		urls[r] = u
	}
	sort.Ints(resolutions)
	mvUrls := make([]*mmpb.MvUrl, 0, len(resolutions))
	for _, r := range resolutions {
		mvUrls = append(mvUrls, &mmpb.MvUrl{Resolution: int32(r), Url: urls[r]})
	}
	return &mmpb.Mv{
		Id: d.ID, Name: d.Name,
		Artists: MapArtists(d.Artists), ArtistName: d.ArtistName,
		CoverUrl: d.Cover, PlayCount: d.PlayCount, DurationMs: d.Duration,
		PublishTime: d.PublishTime, BriefDesc: d.BriefDesc,
		SubCount: d.SubCount, ShareCount: d.ShareCount, LikeCount: d.LikeCount, CommentCount: d.CommentCount,
		Urls: mvUrls,
	}, nil
}

// DecodeMvSummaries 解析 MV 摘要列表响应(listKey 为列表字段名:
// /simi/mv 是 "mvs",/mv/sublist 是 "data"),并回传 hasMore。
func DecodeMvSummaries(raw json.RawMessage, listKey string) ([]*mmpb.Mv, bool, error) {
	var resp struct {
		Mvs     []rawMvSummary `json:"mvs"`
		Data    []rawMvSummary `json:"data"`
		HasMore bool           `json:"hasMore"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, false, fmt.Errorf("解析 MV 列表失败: %w", err)
	}
	var list []rawMvSummary
	switch listKey {
	case "mvs":
		list = resp.Mvs
	default:
		list = resp.Data
	}
	out := make([]*mmpb.Mv, 0, len(list))
	for _, s := range list {
		out = append(out, mapMvSummary(s))
	}
	return out, resp.HasMore, nil
}

// DecodeTopMvs 解析 MV 排行榜响应(/mv/toplist)。
func DecodeTopMvs(raw json.RawMessage) ([]*mmpb.Mv, bool, int64, error) {
	var resp struct {
		Data       []rawMvTopItem `json:"data"`
		HasMore    bool           `json:"hasMore"`
		UpdateTime int64          `json:"updateTime"` // 毫秒时间戳
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, false, 0, fmt.Errorf("解析 MV 排行榜失败: %w", err)
	}
	out := make([]*mmpb.Mv, 0, len(resp.Data))
	for _, item := range resp.Data {
		out = append(out, &mmpb.Mv{
			Id: item.ID, Name: item.Name,
			Artists: MapArtists(item.Artists), ArtistName: item.ArtistName,
			CoverUrl: item.Cover, PlayCount: item.PlayCount, DurationMs: item.Duration,
			PublishTime: item.Mv.PublishTime,
			LastRank:    item.LastRank, Score: item.Score,
		})
	}
	return out, resp.HasMore, resp.UpdateTime, nil
}
