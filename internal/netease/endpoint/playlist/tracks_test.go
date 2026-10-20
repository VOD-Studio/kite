package playlist

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/VOD-Studio/kite/internal/netease/session"
)

// 账号的私有歌单要能带调用方 cookie 读,又不能借用会话池里保存的账号(issue #90)。
func TestPlaylistReads_OptionalAuth(t *testing.T) {
	t.Parallel()
	require.Equal(t, session.AuthOptional, AllTracks.Meta.Auth)
	require.Equal(t, session.AuthOptional, GetPlaylist.Meta.Auth)
}
