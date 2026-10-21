// Package model 的主播电台(djradio)域解码。
//
// 第一阶段(issue #33)只做 endpoint 声明 + CLI 直连验证,响应是纯 Go 模型、
// 尚未 proto 建模(第二阶段替换);json tag 与上游字段同名,直接反序列化。
// 响应形状取自真实上游(见 endpoint/djradio/testdata 夹具,已裁剪)。
package model

import (
	"encoding/json"
	"fmt"
)

// DjCategory 电台分类。
type DjCategory struct {
	ID   int64  `json:"id"`   // 分类ID
	Name string `json:"name"` // 分类名(如「情感」「有声书」)
}

// DjAnchor 主播的公开身份摘要(节目与电台共用的 dj 字段)。
type DjAnchor struct {
	UserID    int64  `json:"userId"`    // 用户ID
	Nickname  string `json:"nickname"`  // 昵称
	AvatarURL string `json:"avatarUrl"` // 头像URL
}

// DjRadio 电台摘要(订阅列表的条目、节目内嵌的 radio 同一形状)。
type DjRadio struct {
	ID             int64     `json:"id"`             // 电台ID
	Name           string    `json:"name"`           // 电台名
	PicURL         string    `json:"picUrl"`         // 封面URL
	Desc           string    `json:"desc"`           // 简介
	Category       string    `json:"category"`       // 一级分类名
	SecondCategory string    `json:"secondCategory"` // 二级分类名
	SubCount       int64     `json:"subCount"`       // 订阅数
	ProgramCount   int64     `json:"programCount"`   // 节目数
	PlayCount      int64     `json:"playCount"`      // 播放数
	Subed          bool      `json:"subed"`          // 当前用户是否已订阅
	Anchor         *DjAnchor `json:"dj"`             // 主播
}

// DjProgram 电台节目。
type DjProgram struct {
	ID            int64     `json:"id"`            // 节目ID
	Name          string    `json:"name"`          // 节目名
	CoverURL      string    `json:"coverUrl"`      // 封面URL
	Description   string    `json:"description"`   // 节目描述
	DurationMs    int64     `json:"duration"`      // 时长(毫秒)
	SerialNum     int64     `json:"serialNum"`     // 期数
	CreateTimeMs  int64     `json:"createTime"`    // 上架时间(毫秒时间戳)
	ListenerCount int64     `json:"listenerCount"` // 播放数
	LikedCount    int64     `json:"likedCount"`    // 点赞数
	ShareCount    int64     `json:"shareCount"`    // 分享数
	CommentCount  int64     `json:"commentCount"`  // 评论数
	MainTrackID   int64     `json:"mainTrackId"`   // 关联歌曲ID(取播放链接用)
	Radio         *DjRadio  `json:"radio"`         // 所属电台
	Anchor        *DjAnchor `json:"dj"`            // 主播
}

// DjCategoryList 电台分类列表响应。
type DjCategoryList struct {
	Categories []*DjCategory `json:"categories"` // 分类列表(真实上游是数组,非旧文档的带 sub 对象)
}

// DjProgramPage 电台节目列表的一页。
type DjProgramPage struct {
	Programs []*DjProgram `json:"programs"` // 本页节目
	Count    int64        `json:"count"`    // 上游总数
	More     bool         `json:"more"`     // 是否还有下一页
}

// DjSubscribedPage 订阅电台列表的一页(需登录态)。
type DjSubscribedPage struct {
	DjRadios []*DjRadio `json:"djRadios"` // 本页电台
	Count    int64      `json:"count"`    // 上游总数(匿名调用时上游不给该字段)
	More     bool       `json:"more"`     // 是否还有下一页
}

// DecodeDjCategories 解析电台分类列表(/weapi/djradio/category/get)。
func DecodeDjCategories(raw json.RawMessage) (*DjCategoryList, error) {
	var out DjCategoryList
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析电台分类失败: %w", err)
	}
	return &out, nil
}

// DecodeDjPrograms 解析电台节目列表的一页(/weapi/dj/program/byradio)。
func DecodeDjPrograms(raw json.RawMessage) (*DjProgramPage, error) {
	var out DjProgramPage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析电台节目失败: %w", err)
	}
	return &out, nil
}

// DecodeDjProgramDetail 解析节目详情(/weapi/dj/program/detail,顶层 program)。
func DecodeDjProgramDetail(raw json.RawMessage) (*DjProgram, error) {
	var resp struct {
		Program *DjProgram `json:"program"` // 节目详情
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析节目详情失败: %w", err)
	}
	if resp.Program == nil {
		return nil, fmt.Errorf("节目详情响应缺少 program 字段")
	}
	return resp.Program, nil
}

// DecodeDjSubscribed 解析订阅电台列表的一页(/weapi/djradio/get/subed,需登录态)。
func DecodeDjSubscribed(raw json.RawMessage) (*DjSubscribedPage, error) {
	var out DjSubscribedPage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析订阅电台失败: %w", err)
	}
	return &out, nil
}
