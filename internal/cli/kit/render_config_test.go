package kit

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/VOD-Studio/kite/internal/localstate"
)

// Render 输出优先级链(PRD-0017)依赖 localstate.Config.Output,测试留在 kit(Render 属 kit)。

// withTTY 把 isTerminal 覆写为常量并注册还原(Render 三态测试的 seam)。
func withTTY(t *testing.T, tty bool) {
	t.Helper()
	orig := isTerminal
	isTerminal = func(int) bool { return tty }
	t.Cleanup(func() { isTerminal = orig })
}

// renderConfigFixture 是优先级测试用的最小 proto 响应。
func renderConfigFixture() *mmpb.HotResponse {
	return &mmpb.HotResponse{Keywords: []*mmpb.HotKeyword{{SearchWord: "周杰伦", Score: 1}}}
}

// TestRender_OutputPrecedence 守护优先级链(PRD-0017):
// --json > 非TTY自动JSON > config.output > 内置默认 table。
func TestRender_OutputPrecedence(t *testing.T) {
	cases := []struct {
		name    string
		json    bool
		tty     bool
		output  string
		wantJS  bool
		wantHum bool
	}{
		// config output=json 在 TTY 下生效(尊重人的偏好)。
		{"config json + TTY → JSON", false, true, "json", true, false},
		// config output=table 不能破管道契约。
		{"config table + 管道 → JSON(契约不破)", false, false, "table", true, false},
		{"config json + 管道 → JSON", false, false, "json", true, false},
		{"--json 强制 JSON(TTY 也 JSON)", true, true, "table", true, false},
		{"无 config + TTY → 人类可读(默认链)", false, true, "table", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTTY(t, tc.tty)
			var buf bytes.Buffer
			k := &Kit{Out: &buf, Config: localstate.Config{Output: tc.output}}
			k.JSON = tc.json
			require.NoError(t, k.Render(renderConfigFixture()))

			out := buf.String()
			if tc.wantJS {
				require.Contains(t, out, `"searchWord"`, "应输出 protojson")
			}
			if tc.wantHum {
				require.Contains(t, out, "== keywords", "应输出人类可读表格")
			}
			// HumanOutput 与 Render 同源:脱敏等决策跟随同一优先级。
			require.Equal(t, tc.wantHum, k.HumanOutput(), "HumanOutput 应与 Render 一致")
		})
	}
}
