package public

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/VOD-Studio/kite/config"
	"github.com/stretchr/testify/require"
)

func TestParseServiceAccounts(t *testing.T) {
	dir := t.TempDir()
	accounts := []ServiceAccount{
		{ID: "primary", SessionFile: filepath.Join(dir, "primary.json")},
		{ID: "secondary", SessionFile: filepath.Join(dir, "secondary.json")},
	}
	raw, err := json.Marshal(accounts)
	require.NoError(t, err)
	parsed, err := ParseServiceAccounts(config.PublicConfig{Accounts: string(raw)})
	require.NoError(t, err, "账号解析不要求播放器白名单")
	require.Equal(t, accounts, parsed, "保留部署顺序")

	opts, err := ParseOptions(config.PublicConfig{
		Accounts: string(raw), Songs: "42", RatePerMinute: 60, Quality: "exhigh",
	})
	require.NoError(t, err)
	require.Equal(t, accounts, opts.Accounts)
	require.Empty(t, opts.SessionFile)

	legacy := config.PublicConfig{SessionFile: " relative-session.json ", Songs: "42", RatePerMinute: 60, Quality: "exhigh"}
	opts, err = ParseOptions(legacy)
	require.NoError(t, err)
	require.Equal(t, "relative-session.json", opts.SessionFile)
	require.Empty(t, opts.Accounts)
	parsed, err = ParseServiceAccounts(config.PublicConfig{})
	require.NoError(t, err)
	require.Empty(t, parsed)
}

func TestParseServiceAccountsRejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{name: "malformed", raw: `[{`},
		{name: "object", raw: `{}`},
		{name: "unknown field", raw: `[{"id":"primary","sessionFile":"/data/a.json","cookie":"secret"}]`},
		{name: "trailing value", raw: `[{"id":"primary","sessionFile":"/data/a.json"}] []`},
		{name: "trailing garbage", raw: `[{"id":"primary","sessionFile":"/data/a.json"}] garbage`},
		{name: "empty list", raw: `[]`},
		{name: "null", raw: `null`},
		{name: "whitespace", raw: " "},
		{name: "null entry", raw: `[null]`},
		{name: "empty ID", raw: `[{"id":"","sessionFile":"/data/a.json"}]`},
		{name: "invalid ID", raw: `[{"id":"../primary","sessionFile":"/data/a.json"}]`},
		{name: "spaced ID", raw: `[{"id":"primary ","sessionFile":"/data/a.json"}]`},
		{name: "long ID", raw: `[{"id":"` + strings.Repeat("a", 65) + `","sessionFile":"/data/a.json"}]`},
		{name: "duplicate ID", raw: `[{"id":"primary","sessionFile":"/data/a.json"},{"id":"primary","sessionFile":"/data/b.json"}]`},
		{name: "empty path", raw: `[{"id":"primary","sessionFile":""}]`},
		{name: "spaced path", raw: `[{"id":"primary","sessionFile":" "}]`},
		{name: "relative path", raw: `[{"id":"primary","sessionFile":"a.json"}]`},
		{name: "duplicate path", raw: `[{"id":"primary","sessionFile":"/data/a.json"},{"id":"secondary","sessionFile":"/data/a.json"}]`},
		{name: "normalized duplicate path", raw: `[{"id":"primary","sessionFile":"/data/sub/../a.json"},{"id":"secondary","sessionFile":"/data/a.json"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseServiceAccounts(config.PublicConfig{Accounts: tc.raw})
			require.Error(t, err)
		})
	}
	_, err := ParseServiceAccounts(config.PublicConfig{
		Accounts: `[{"id":"primary","sessionFile":"/data/a.json"}]`, SessionFile: "legacy.json",
	})
	require.ErrorContains(t, err, "不能同时设置")
}

func TestParseServiceAccountsRejectsSymlinkAliases(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "session.json")
	require.NoError(t, os.WriteFile(file, []byte("{}"), 0o600))
	alias := filepath.Join(dir, "alias.json")
	require.NoError(t, os.Symlink(file, alias))
	parentAlias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(dir, parentAlias))
	for _, paths := range [][2]string{
		{file, alias},
		{filepath.Join(dir, "future.json"), filepath.Join(parentAlias, "future.json")},
	} {
		raw, err := json.Marshal([]ServiceAccount{{ID: "one", SessionFile: paths[0]}, {ID: "two", SessionFile: paths[1]}})
		require.NoError(t, err)
		_, err = ParseServiceAccounts(config.PublicConfig{Accounts: string(raw)})
		require.ErrorContains(t, err, "sessionFile 重复")
	}
}
