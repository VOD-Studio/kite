package termqr

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRender(t *testing.T) {
	out := Render("https://music.163.com/login?codekey=abc")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Greater(t, len(lines), 10, "应输出多行二维码")
	width := len([]rune(lines[0]))
	for _, l := range lines {
		require.Equal(t, width, len([]rune(l)), "每行宽度一致")
	}
	require.True(t, strings.ContainsAny(out, "█▀▄"), "应包含半高方块")
}
