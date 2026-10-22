package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/VOD-Studio/kite/config"
	"github.com/VOD-Studio/kite/internal/public"
	"github.com/stretchr/testify/require"
)

func TestPublicLoginTarget(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "primary.json")
	secondary := filepath.Join(dir, "secondary.json")
	raw, err := json.Marshal([]public.ServiceAccount{
		{ID: "primary", SessionFile: primary},
		{ID: "secondary", SessionFile: secondary},
	})
	require.NoError(t, err)
	multi := config.PublicConfig{Accounts: string(raw)}
	legacy := config.PublicConfig{SessionFile: " legacy.json "}
	for _, tc := range []struct {
		name string
		cfg  config.PublicConfig
		args []string
		want string
	}{
		{name: "primary", cfg: multi, args: []string{"--account", "primary"}, want: primary},
		{name: "secondary", cfg: multi, args: []string{"--account=secondary"}, want: secondary},
		{name: "legacy", cfg: legacy, want: "legacy.json"},
		{name: "explicit multi override", cfg: multi, args: []string{"--file", "direct.json"}, want: "direct.json"},
		{name: "explicit legacy override", cfg: legacy, args: []string{"--file", "direct.json"}, want: "direct.json"},
		{name: "explicit without configuration", args: []string{"--file", "direct.json"}, want: "direct.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, err := publicLoginTarget(tc.args, tc.cfg)
			require.NoError(t, err)
			require.Equal(t, tc.want, path)
		})
	}
	for _, tc := range []struct {
		name string
		cfg  config.PublicConfig
		args []string
	}{
		{name: "multi requires selection", cfg: multi},
		{name: "unknown account", cfg: multi, args: []string{"--account", "unconfigured"}},
		{name: "empty account", cfg: multi, args: []string{"--account="}},
		{name: "legacy has no named account", cfg: legacy, args: []string{"--account", "default"}},
		{name: "both flags", cfg: multi, args: []string{"--account", "primary", "--file", "direct.json"}},
		{name: "empty explicit file", cfg: legacy, args: []string{"--file="}},
		{name: "whitespace explicit file", cfg: legacy, args: []string{"--file", " "}},
		{name: "positional", cfg: legacy, args: []string{"unexpected"}},
		{name: "trailing positional", cfg: multi, args: []string{"--account", "primary", "unexpected"}},
		{name: "unknown flag", cfg: legacy, args: []string{"--unknown"}},
		{name: "missing flag value", cfg: multi, args: []string{"--account"}},
		{name: "missing target"},
		{name: "invalid accounts", cfg: config.PublicConfig{Accounts: "invalid"}, args: []string{"--account", "primary"}},
		{name: "ambiguous configuration", cfg: config.PublicConfig{Accounts: string(raw), SessionFile: "legacy.json"}, args: []string{"--account", "primary"}},
		{name: "explicit file rejects invalid configuration", cfg: config.PublicConfig{Accounts: "invalid"}, args: []string{"--file", "direct.json"}},
		{name: "explicit file rejects ambiguous configuration", cfg: config.PublicConfig{Accounts: string(raw), SessionFile: "legacy.json"}, args: []string{"--file", "direct.json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := publicLoginTarget(tc.args, tc.cfg)
			require.Error(t, err)
		})
	}
}
