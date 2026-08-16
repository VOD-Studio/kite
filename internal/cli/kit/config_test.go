// Package kit 的 config 加载/校验/写入测试(PRD-0017)。
//
// 覆盖 seam:LoadConfig / SetConfigKey / ConfigPath,全部经 withTestConfigDir
// 重定向到 t.TempDir(),不碰用户真实配置目录。
package kit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	mmpb "github.com/VOD-Studio/kite/gen/go/netease/music/v1"
	"github.com/stretchr/testify/require"
)

// writeTestConfig 把 toml 内容写进临时配置目录的 config.toml(模拟用户手改/已存在文件)。
func writeTestConfig(t *testing.T, content string) {
	t.Helper()
	dir := withTestConfigDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o600))
}

func TestLoadConfig_AbsentFile_ReturnsDefaults(t *testing.T) {
	withTestConfigDir(t)

	got, err := LoadConfig()
	require.NoError(t, err)
	require.Equal(t, DefaultConfig(), got)
}

func TestDefaultConfig_BuiltinValues(t *testing.T) {
	// 内置默认与各 flag 现状对齐(独立真相,非读代码回填)。
	require.Equal(t, Config{
		Level:            1,
		Output:           "table",
		DownloadDir:      ".",
		FilenameTemplate: "",
		Workers:          3,
	}, DefaultConfig())
}

func TestLoadConfig_ValidFile_ParsesAllKeys(t *testing.T) {
	writeTestConfig(t, `level = 3
output = "json"
download_dir = "~/Music"
filename_template = "{title} - {artist}"
workers = 5
`)

	got, err := LoadConfig()
	require.NoError(t, err)
	require.Equal(t, Config{
		Level:            3,
		Output:           "json",
		DownloadDir:      "~/Music",
		FilenameTemplate: "{title} - {artist}",
		Workers:          5,
	}, got)
}

func TestLoadConfig_PartialFile_UnsetKeysKeepDefaults(t *testing.T) {
	writeTestConfig(t, "level = 4\n")

	got, err := LoadConfig()
	require.NoError(t, err)
	want := DefaultConfig()
	want.Level = 4
	require.Equal(t, want, got)
}

func TestSetConfigKey_Roundtrip(t *testing.T) {
	dir := withTestConfigDir(t)

	require.NoError(t, SetConfigKey("level", "3"))
	require.NoError(t, SetConfigKey("filename_template", "{album}/{id} {title}"))

	got, err := LoadConfig()
	require.NoError(t, err)
	want := DefaultConfig()
	want.Level = 3
	want.FilenameTemplate = "{album}/{id} {title}"
	require.Equal(t, want, got)

	// 首写生成 0600 文件 + 管理注释头。
	path := filepath.Join(dir, "config.toml")
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "kite config")
	// 二次 set 保留首个 key(整文件重写,非覆盖单行)。
	require.Contains(t, string(data), "level = 3")
	// 只落显式设置过的 key:未设置的留空,「来源」列才有区分度。
	require.NotContains(t, string(data), "workers", "未设置的 key 不应写盘")
	require.NotContains(t, string(data), "output", "未设置的 key 不应写盘")
}

// TestLoadConfig_HardErrors 守护「坏文件 = 硬错误」:静默回退会掩盖用户设置(PRD-0017)。
func TestLoadConfig_HardErrors(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantKey string // 错误信息应点名肇因 key(坏 toml 除外)
	}{
		{"非法 toml 语法", "level = ", ""},
		{"未知 key", "volume = 80\n", "volume"},
		{"level 越界", "level = 9\n", "level"},
		{"level 类型错", `level = "high"` + "\n", "level"},
		{"output 枚举外", `output = "csv"` + "\n", "output"},
		{"output 类型错", "output = 2\n", "output"},
		{"download_dir 空", `download_dir = ""` + "\n", "download_dir"},
		{"workers 越界", "workers = 0\n", "workers"},
		{"workers 超上限", "workers = 6\n", "workers"},
		{"模板未知占位符", `filename_template = "{titel}"` + "\n", "filename_template"},
		{"模板未闭合", `filename_template = "{artist"` + "\n", "filename_template"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeTestConfig(t, tc.content)
			_, err := LoadConfig()
			require.Error(t, err, "坏配置必须硬错误,不允许静默回退默认值")
			if tc.wantKey != "" {
				require.Contains(t, err.Error(), tc.wantKey, "错误应点名肇事 key")
			}
			require.Contains(t, err.Error(), "config.toml", "错误应给出文件路径线索")
		})
	}
}

// TestSetConfigKey_RejectsInvalid 守护 set 入口校验;失败时文件不得被改动。
func TestSetConfigKey_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{"未知 key", "levil", "3"},
		{"level 越界", "level", "5"},
		{"level 非整数", "level", "high"},
		{"output 枚举外", "output", "csv"},
		{"workers 越界", "workers", "16"},
		{"模板未知占位符", "filename_template", "{titel}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := withTestConfigDir(t)
			err := SetConfigKey(tc.key, tc.value)
			require.Error(t, err)
			// 校验失败不得产生/改动文件。
			_, statErr := os.Stat(filepath.Join(dir, "config.toml"))
			require.True(t, os.IsNotExist(statErr), "校验失败不应落盘")
		})
	}
}

// TestSetConfigKey_BadFileRefused 坏文件在手时 set 拒绝执行(不允许绕过校验覆盖写)。
func TestSetConfigKey_BadFileRefused(t *testing.T) {
	writeTestConfig(t, "level = 9\n")
	err := SetConfigKey("output", "json")
	require.Error(t, err)
}

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
			k := &Kit{Out: &buf, Config: Config{Output: tc.output}}
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
