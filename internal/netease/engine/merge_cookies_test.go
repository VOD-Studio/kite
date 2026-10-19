package engine_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/VOD-Studio/kite/internal/netease/engine"
)

// 刷新登录态时上游只下发变化的项,合并结果必须仍是可直接使用的完整 cookie。
func TestMergeCookies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, base, update, want string
	}{
		{"没有新值时原样返回", "MUSIC_U=old; __csrf=c", "", "MUSIC_U=old; __csrf=c"},
		{"同名项替换且保留位置", "MUSIC_U=old; __csrf=c; NMTID=n", "MUSIC_U=new", "MUSIC_U=new; __csrf=c; NMTID=n"},
		{"新出现的项追加在末尾", "MUSIC_U=old", "__csrf=c2; MUSIC_U=new", "MUSIC_U=new; __csrf=c2"},
		{"容忍多余空白与空段", " MUSIC_U=old ;; __csrf=c ", "__csrf=c2", "MUSIC_U=old; __csrf=c2"},
		{"值里的等号原样保留", "MUSIC_U=a=b", "MUSIC_U=x==", "MUSIC_U=x=="},
		{"base 为空时就是新值", "", "MUSIC_U=new", "MUSIC_U=new"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, engine.MergeCookies(tc.base, tc.update))
		})
	}
}
