// Package config 的命令行为测试:经 cobra 执行,deps 注入替身(downloadDeps 同款)。
package config

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/VOD-Studio/kite/internal/cli/kit"
)

// fakeDeps 构造注入替身的 deps。set 记录调用供断言。
func fakeDeps(cfg kit.Config, set map[string]bool, setErr error) (deps, *[]string) {
	calls := &[]string{}
	return deps{
		path: func() (string, error) { return "/tmp/kite-config-dir/config.toml", nil },
		load: func() (kit.Config, map[string]bool, error) { return cfg, set, nil },
		set: func(key, value string) error {
			*calls = append(*calls, key+"="+value)
			return setErr
		},
	}, calls
}

// runConfig 执行 config 子命令并捕获 stdout。
func runConfig(t *testing.T, d deps, k *kit.Kit, args ...string) (string, error) {
	t.Helper()
	c := newConfigCommand(k, d)
	var buf bytes.Buffer
	c.SetOut(&buf)
	c.SetArgs(args)
	err := c.Execute()
	return buf.String(), err
}

func TestConfigPath_BarePath(t *testing.T) {
	d, _ := fakeDeps(kit.DefaultConfig(), nil, nil)
	out, err := runConfig(t, d, kit.New(), "path")
	require.NoError(t, err)
	require.Equal(t, "/tmp/kite-config-dir/config.toml\n", out, "config path 裸打路径")
}

func TestConfigGet_SingleKey_BareValue(t *testing.T) {
	cfg := kit.DefaultConfig()
	cfg.Level = 3
	d, _ := fakeDeps(cfg, map[string]bool{"level": true}, nil)

	out, err := runConfig(t, d, kit.New(), "get", "level")
	require.NoError(t, err)
	require.Equal(t, "3\n", out, "单 key 裸打生效值,不带任何格式")
}

func TestConfigGet_UnsetKey_AnswersDefault(t *testing.T) {
	d, _ := fakeDeps(kit.DefaultConfig(), map[string]bool{}, nil)

	out, err := runConfig(t, d, kit.New(), "get", "level")
	require.NoError(t, err)
	require.Equal(t, "1\n", out, "未设置时答内置默认(生效值语义)")
}

func TestConfigGet_FilenameTemplateUnset_ShowsEffectiveDefault(t *testing.T) {
	d, _ := fakeDeps(kit.DefaultConfig(), map[string]bool{}, nil)

	out, err := runConfig(t, d, kit.New(), "get", "filename_template")
	require.NoError(t, err)
	require.Equal(t, "{artist} - {title}\n", out, "空模板展示实际生效的默认模板")
}

func TestConfigGet_UnknownKey_Errors(t *testing.T) {
	d, _ := fakeDeps(kit.DefaultConfig(), map[string]bool{}, nil)

	_, err := runConfig(t, d, kit.New(), "get", "levil")
	require.Error(t, err)
	require.Contains(t, err.Error(), "levil")
}

func TestConfigGet_NoArgs_TableWithSource(t *testing.T) {
	cfg := kit.DefaultConfig()
	cfg.Level = 3
	cfg.Output = "json"
	d, _ := fakeDeps(cfg, map[string]bool{"level": true, "output": true}, nil)

	out, err := runConfig(t, d, kit.New(), "get")
	require.NoError(t, err)
	// 五项全列,来源区分 config / 默认。
	require.Contains(t, out, "level")
	require.Contains(t, out, "output")
	require.Contains(t, out, "download_dir")
	require.Contains(t, out, "filename_template")
	require.Contains(t, out, "workers")
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, "level") {
			require.Contains(t, line, "config", "已设置的 key 来源标 config")
		}
		if strings.HasPrefix(line, "workers") {
			require.Contains(t, line, "默认", "未设置的 key 来源标默认")
		}
	}
}

func TestConfigGet_NoArgs_JSONMode(t *testing.T) {
	cfg := kit.DefaultConfig()
	cfg.Level = 3
	d, _ := fakeDeps(cfg, map[string]bool{"level": true}, nil)
	k := kit.New()
	k.JSON = true

	out, err := runConfig(t, d, k, "get")
	require.NoError(t, err)
	require.Contains(t, out, `"key"`)
	require.Contains(t, out, `"source"`)
}

// TestConfigGet_NoArgs_ConfigOutputJSON 守护优先级链延伸:config output=json 时
// 无参列表也走结构化(与 Render 同链,config 偏好作用于一切人类可读输出)。
func TestConfigGet_NoArgs_ConfigOutputJSON(t *testing.T) {
	d, _ := fakeDeps(kit.DefaultConfig(), map[string]bool{}, nil)
	k := kit.New()
	k.Config.Output = "json"

	out, err := runConfig(t, d, k, "get")
	require.NoError(t, err)
	require.Contains(t, out, `"key"`)
	require.Contains(t, out, `"source"`)
}

func TestConfigSet_Success_ConfirmationLine(t *testing.T) {
	d, calls := fakeDeps(kit.DefaultConfig(), map[string]bool{}, nil)

	out, err := runConfig(t, d, kit.New(), "set", "level", "3")
	require.NoError(t, err)
	require.Equal(t, []string{"level=3"}, *calls)
	require.Contains(t, out, "level = 3")
	require.Contains(t, out, "config.toml", "确认行带文件路径")
}

func TestConfigSet_Failure_Propagates(t *testing.T) {
	d, calls := fakeDeps(kit.DefaultConfig(), map[string]bool{}, errors.New("配置 level 的值 9 越界"))

	_, err := runConfig(t, d, kit.New(), "set", "level", "9")
	require.Error(t, err)
	require.Equal(t, []string{"level=9"}, *calls)
}
