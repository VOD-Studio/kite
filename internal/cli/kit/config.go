// Package kit
// config.go 实现 config.toml 的加载/校验/写入(PRD-0017)。
//
// 设计原则(决策见 CONTEXT.md「不可逆决策」表):
//   - config key 只收「已存在 flag 或硬编码默认值」(当前六项,见 configKeys);
//   - 文件不存在 = 正常态(全部内置默认);文件存在但坏 = 硬错误,静默回退会掩盖用户设置;
//   - set 写盘前完成全部校验,读取侧复用同一套校验(手改文件同样被拦);
//   - tmp+rename 原子写,文件 0600、目录 0700(与 session 一致)。
package kit

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// configHeader 是 set 首写时文件顶部的管理注释(此后整文件重写,不保留用户注释)。
const configHeader = "# 本文件由 kite config 管理(kite config set);手改请遵循既有 key 与取值范围。\n"

// Config 是用户偏好的生效值集合(字段与 configKeys 一一对应)。
// 零值不可直接使用(Workers=0 非法),
// 一律经 DefaultConfig() 或 LoadConfig() 获得。
type Config struct {
	Level            int    // 默认音质,1=standard 2=exhigh 3=lossless 4=hires
	Output           string // "table" | "json"
	DownloadDir      string // 下载目录(--out)
	FilenameTemplate string // 文件名模板,空 = {artist} - {title}
	Workers          int    // playlist download 并发数(--workers)
	Proxy            string // 代理地址,空 = 未设置(回落环境变量层,PRD-0018)
}

// DefaultConfig 返回内置默认值(与各 flag 的硬编码默认对齐,独立真相)。
func DefaultConfig() Config {
	return Config{
		Level:            1,
		Output:           "table",
		DownloadDir:      ".",
		FilenameTemplate: "",
		Workers:          3,
		Proxy:            "",
	}
}

// ConfigPath 返回 config.toml 路径 <ConfigDir>/config.toml(只算路径不创建)。
func ConfigPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}

// LoadConfig 读取 config.toml 并校验。文件不存在返回内置默认(正常态,非错误);
// 文件存在但解析失败/校验不过返回 error(硬错误,调用方应拒绝执行,PRD-0017)。
func LoadConfig() (Config, error) {
	cfg, _, err := LoadConfigWithSet()
	return cfg, err
}

// LoadConfigWithSet 在 LoadConfig 基础上额外返回「文件里显式设置的 key 集」
// (config get 无参列表的「来源」列用)。
func LoadConfigWithSet() (Config, map[string]bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), map[string]bool{}, nil
		}
		return Config{}, nil, fmt.Errorf("读取配置 %s 失败: %w", path, err)
	}
	return parseConfig(path, data)
}

// ConfigKeys 返回合法 key 列表(顺序即 config get 无参列表顺序,单一真相)。
func ConfigKeys() []string {
	return append([]string(nil), configKeys...)
}

// IsKnownConfigKey 判断 key 是否在 configKeys 枚举内(config get/set 命令层校验用)。
func IsKnownConfigKey(key string) bool { return isKnownKey(key) }

// parseConfig 解析并校验 config.toml 内容,附带返回显式设置的 key 集。
// 未知 key / 越界值 / 类型不符均报错(带 key 名,硬错误语义)。
func parseConfig(path string, data []byte) (Config, map[string]bool, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return Config{}, nil, fmt.Errorf("解析配置 %s 失败: %w", path, err)
	}
	cfg := DefaultConfig()
	set := make(map[string]bool, len(raw))
	for key, val := range raw {
		if err := applyConfigKey(&cfg, key, val); err != nil {
			return Config{}, nil, fmt.Errorf("配置项 %s:%w(文件 %s)", key, err, path)
		}
		set[key] = true
	}
	return cfg, set, nil
}

// SetConfigKey 校验并写入单个 key(整文件重写,tmp+rename 原子落盘)。
// 只落显式设置过的 key——未设置的留空,读取侧回退内置默认,
// 「来源」列(config get 无参列表)才有区分度。
// 坏文件在手时同样报错——不允许绕过校验覆盖写。
func SetConfigKey(key, value string) error {
	if !isKnownKey(key) {
		return fmt.Errorf("未知配置项 %q(可用: %s)", key, strings.Join(ConfigKeys(), " "))
	}
	cfg, set, err := LoadConfigWithSet()
	if err != nil {
		return err
	}
	if err := applyConfigKey(&cfg, key, value); err != nil {
		return fmt.Errorf("配置项 %s:%w", key, err)
	}
	// proxy 的空值有真实语义 = 清除该 key(回落环境变量层),不写空串行(PRD-0018)。
	if key == "proxy" && value == "" {
		delete(set, key)
	} else {
		set[key] = true
	}
	return writeConfigFile(cfg, set)
}

// writeConfigFile 把显式设置的 key 序列化为 toml 落盘(带管理注释头)。
// 未设置的 key 不写(读时回退内置默认)。
func writeConfigFile(cfg Config, set map[string]bool) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录 %s 失败: %w", filepath.Dir(path), err)
	}
	var b strings.Builder
	b.WriteString(configHeader)
	if set["level"] {
		fmt.Fprintf(&b, "level = %d\n", cfg.Level)
	}
	if set["output"] {
		fmt.Fprintf(&b, "output = %q\n", cfg.Output)
	}
	if set["download_dir"] {
		fmt.Fprintf(&b, "download_dir = %q\n", cfg.DownloadDir)
	}
	if set["filename_template"] {
		fmt.Fprintf(&b, "filename_template = %q\n", cfg.FilenameTemplate)
	}
	if set["workers"] {
		fmt.Fprintf(&b, "workers = %d\n", cfg.Workers)
	}
	if set["proxy"] {
		fmt.Fprintf(&b, "proxy = %q\n", cfg.Proxy)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("写入配置 %s 失败: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("落盘配置 %s 失败: %w", path, err)
	}
	return nil
}

// applyConfigKey 把单个 key 的值(set 侧是 string,文件侧是 toml any)校验后写进 cfg。
// 两侧共用同一套校验(set 入口收紧 + 读取侧复用,PRD-0017)。
func applyConfigKey(cfg *Config, key string, val any) error {
	// set 侧传 string(命令行参数);文件侧 toml 反序列化出 int64/string。
	switch key {
	case "level":
		n, err := toInt(val)
		if err != nil {
			return fmt.Errorf("值必须是 1-4 的整数: %w", err)
		}
		if n < 1 || n > 4 {
			return fmt.Errorf("值 %d 越界(音质 1=standard 2=exhigh 3=lossless 4=hires)", n)
		}
		cfg.Level = n
	case "output":
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("值必须是 table 或 json")
		}
		if s != "table" && s != "json" {
			return fmt.Errorf("值 %q 不是 table 或 json", s)
		}
		cfg.Output = s
	case "download_dir":
		s, ok := val.(string)
		if !ok || s == "" {
			return fmt.Errorf("值必须是非空目录路径")
		}
		cfg.DownloadDir = s
	case "filename_template":
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("值必须是文件名模板字符串")
		}
		if err := validateFilenameTemplate(s); err != nil {
			return err
		}
		cfg.FilenameTemplate = s
	case "workers":
		n, err := toInt(val)
		if err != nil {
			return fmt.Errorf("值必须是 1-5 的整数: %w", err)
		}
		if n < 1 || n > 5 {
			return fmt.Errorf("值 %d 越界(并发 1-5,与 --workers 一致)", n)
		}
		cfg.Workers = n
	case "proxy":
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("值必须是代理地址字符串")
		}
		// 空串合法 = 未设置(清除语义);非空走与 --proxy 同一套校验。
		if _, err := ParseProxyURL(s); err != nil {
			return err
		}
		cfg.Proxy = s
	default:
		return fmt.Errorf("是未知配置项(可用: %s)", strings.Join(ConfigKeys(), " "))
	}
	return nil
}

// configKeys 是合法 key 的单一真相(顺序即 config get 无参列表顺序)。
var configKeys = []string{"level", "output", "download_dir", "filename_template", "workers", "proxy"}

// isKnownKey 判断 key 是否在 configKeys 枚举内。
func isKnownKey(key string) bool {
	for _, k := range configKeys {
		if k == key {
			return true
		}
	}
	return false
}

// FilenameTemplatePlaceholders 返回 filename_template 的合法占位符(config schema 的
// 单一真相)。songdl 的执行侧(songdl.FormatFilename)与这里由
// songdl 包的守护测试保持同步——公共层只定义 schema,不夹带下载领域逻辑。
func FilenameTemplatePlaceholders() []string {
	return []string{"{artist}", "{title}", "{album}", "{id}"}
} // toInt 把 set 侧 string / 文件侧 int64 / int 统一转 int。
func toInt(val any) (int, error) {
	switch v := val.(type) {
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("%q 不是整数", v)
		}
		return n, nil
	case int64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("%v 不是整数", v)
	}
}

// validateFilenameTemplate 校验模板占位符只含白名单(见 FilenameTemplatePlaceholders)。
// 未知占位符报错(拼错早暴露,同未知 key 逻辑)。
func validateFilenameTemplate(s string) error {
	if s == "" {
		return nil // 空 = 用默认模板(songdl.DefaultFilenameTemplate)
	}
	rest := s
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			return nil
		}
		end := open + strings.IndexByte(rest[open:], '}')
		if end < open {
			return fmt.Errorf("模板 %q 有未闭合的 {", s)
		}
		name := rest[open+1 : end]
		switch name {
		case "artist", "title", "album", "id":
		default:
			return fmt.Errorf("模板 %q 的占位符 {%s} 未知(可用: %s)", s, name, strings.Join(FilenameTemplatePlaceholders(), "/"))
		}
		rest = rest[end+1:]
	}
}
