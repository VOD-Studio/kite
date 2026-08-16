// Package config 实现 kite config 命令组:config.toml 的查看与写入(PRD-0017)。
//
// 子命令:path(裸打路径)/ get(单 key 裸打生效值,无参列表带来源)/ set(校验后写盘)。
// 输出遵守全局 --json 约定,但值对象是 Go struct 直接 marshal(配置不是 rpc 域)。
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/VOD-Studio/kite/internal/cli/kit"
	"github.com/VOD-Studio/kite/internal/cli/songdl"
)

// deps 注入 kit 函数,测试替身用(song download 的 downloadDeps 同款模式)。
type deps struct {
	path     func() (string, error)                      // config.toml 路径
	load     func() (kit.Config, map[string]bool, error) // 生效值 + 文件里显式设置的 key 集
	set      func(key, value string) error               // 校验并写入
	wantJSON func() bool                                 // 输出形态(kit.WantJSON 同链,测试可替身)
}

func defaultDeps(k *kit.Kit) deps {
	return deps{
		path:     kit.ConfigPath,
		load:     kit.LoadConfigWithSet,
		set:      kit.SetConfigKey,
		wantJSON: k.WantJSON,
	}
}

// completeConfigKeys 是 config get/set 第一个位置参数(key)的补全:
// key 枚举前缀过滤,禁止文件名补全(PRD-0018「进补全枚举」)。
func completeConfigKeys(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []cobra.Completion
	for _, key := range kit.ConfigKeys() {
		if strings.HasPrefix(key, toComplete) {
			out = append(out, key)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// NewCommand 创建 config 命令组(容器,生产装配)。
func NewCommand(k *kit.Kit) *cobra.Command {
	return newConfigCommand(k, defaultDeps(k))
}

// newConfigCommand 用注入 deps 构造(测试替身入口)。
func newConfigCommand(k *kit.Kit, d deps) *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "查看与写入用户偏好(config.toml)",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(
		newPathCommand(d),
		newGetCommand(d),
		newSetCommand(d),
	)
	// 本地命令(无 rpc),三个叶子各自打空标(rpc 守护要求叶子显式审视)。
	for _, sub := range c.Commands() {
		kit.AnnotateRpcs(sub)
	}
	return c
}

// newPathCommand 实现 config path:裸打 config.toml 完整路径(脚本可用)。
func newPathCommand(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "打印 config.toml 路径(裸路径,脚本可用)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := d.path()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), p)
			return nil
		},
	}
}

// newGetCommand 实现 config get:单 key 裸打生效值(不带格式,脚本可用,gh/jj 惯例);
// 无参列出全部 key:生效值、来源(config / 默认)。
func newGetCommand(d deps) *cobra.Command {
	return &cobra.Command{
		Use:               "get [key]",
		Short:             "查看配置生效值(单 key 裸值输出,可在脚本中使用)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeConfigKeys,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, set, err := d.load()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(args) == 1 {
				key := args[0]
				if !kit.IsKnownConfigKey(key) {
					return fmt.Errorf("未知配置项 %q(可用: %s)", key, strings.Join(kit.ConfigKeys(), " "))
				}
				fmt.Fprintln(out, displayValue(key, cfg))
				return nil
			}
			// 无参:全量列表。输出形态走 kit.WantJSON 同一条优先级链
			// (--json > 非TTY自动JSON > config > 默认),config get | jq 直接可用。
			rows := []entry{}
			for _, key := range kit.ConfigKeys() {
				source := "默认"
				if set[key] {
					source = "config"
				}
				rows = append(rows, entry{Key: key, Value: displayValue(key, cfg), Source: source})
			}
			if d.wantJSON != nil && d.wantJSON() {
				b, err := json.MarshalIndent(rows, "", "  ")
				if err != nil {
					return fmt.Errorf("序列化配置列表失败: %w", err)
				}
				fmt.Fprintln(out, string(b))
				return nil
			}
			renderEntries(out, rows)
			return nil
		},
	}
}

// newSetCommand 实现 config set:校验通过原子写盘,stdout 一行确认。
func newSetCommand(d deps) *cobra.Command {
	return &cobra.Command{
		Use:               "set <key> <value>",
		Short:             "写入配置项(校验后原子落盘)",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeConfigKeys,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			if err := d.set(key, value); err != nil {
				return err
			}
			p, err := d.path()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s = %s 已写入 %s\n", key, value, p)
			return nil
		},
	}
}

// entry 是无参列表的一行(--json 时直接 marshal)。
type entry struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// displayValue 返回 key 的生效值字符串形态。
// filename_template 未设(空)时展示实际生效的默认模板——get 永远回答「现在命令实际用什么」。
func displayValue(key string, cfg kit.Config) string {
	switch key {
	case "level":
		return strconv.Itoa(cfg.Level)
	case "workers":
		return strconv.Itoa(cfg.Workers)
	case "filename_template":
		if cfg.FilenameTemplate == "" {
			return songdl.DefaultFilenameTemplate
		}
		return cfg.FilenameTemplate
	case "output":
		return cfg.Output
	case "download_dir":
		return cfg.DownloadDir
	case "proxy":
		// 空=config 层未设置(回落环境变量,由 doctor 呈现完整解析链);裸值保持脚本可判。
		return cfg.Proxy
	default:
		return ""
	}
}

// renderEntries 渲染对齐表格(key/value/来源 三列,doctor.RenderHuman 同款极简风)。
func renderEntries(w io.Writer, rows []entry) {
	maxKey, maxVal := 0, 0
	for _, r := range rows {
		if l := len(r.Key); l > maxKey {
			maxKey = l
		}
		if l := len(r.Value); l > maxVal {
			maxVal = l
		}
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%-*s  %-*s  %s\n", maxKey, r.Key, maxVal, r.Value, r.Source)
	}
}
