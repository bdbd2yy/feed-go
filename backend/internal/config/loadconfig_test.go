package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFeedThresholdConfig(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, env string
		want            int64
	}{
		{"default", "server:\n  port: 8080\n", "", 10000},
		{"yaml", "feed:\n  big_v_threshold: 500\n", "", 500},
		{"env overrides yaml", "feed:\n  big_v_threshold: 500\n", "25", 25},
		{"malformed env preserves yaml", "feed:\n  big_v_threshold: 500\n", "bad", 500},
		{"zero env", "", "0", 10000},
		{"negative env", "", "-1", 10000},
		{"overflow env", "", "999999999999999999999999", 10000},
		{"nonpositive yaml", "feed:\n  big_v_threshold: -3\n", "", 10000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FEED_BIG_V_THRESHOLD", tc.env)
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Feed.BigVThreshold != tc.want {
				t.Fatalf("got %d want %d", cfg.Feed.BigVThreshold, tc.want)
			}
		})
	}
}

func TestMissingConfigAppliesFeedEnv(t *testing.T) {
	t.Setenv("FEED_BIG_V_THRESHOLD", "42")
	cfg, usedDefault, err := LoadLocalDev(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil || !usedDefault || cfg.Feed.BigVThreshold != 42 {
		t.Fatalf("cfg=%+v default=%v err=%v", cfg.Feed, usedDefault, err)
	}
	t.Setenv("FEED_BIG_V_THRESHOLD", "")
	if got := DefaultLocalConfig().Feed.BigVThreshold; got != DefaultBigVThreshold {
		t.Fatalf("default = %d", got)
	}
}
