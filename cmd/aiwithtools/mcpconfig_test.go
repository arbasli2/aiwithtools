package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCfg lays out <dir>/aiwithtools/mcp.json and points XDG_CONFIG_HOME
// at dir, so loadMCPConfig resolves the default path there.
func writeCfg(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "aiwithtools")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.WriteFile(filepath.Join(cfgDir, "mcp.json"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
}

// A missing default config is not an error: MCP servers are optional.
func TestLoadMCPConfig_MissingDefaultIsTolerated(t *testing.T) {
	writeCfg(t, "")
	cfg, err := loadMCPConfig("")
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if cfg == nil {
		t.Fatal("want non-nil empty config, got nil")
	}
	if len(cfg.Servers) != 0 {
		t.Errorf("want 0 servers, got %d", len(cfg.Servers))
	}
}

// A missing explicit --mcp path is a typo — fail rather than start with
// no tools. This is the whole point of the flag's error handling.
func TestLoadMCPConfig_MissingExplicitPathIsFatal(t *testing.T) {
	writeCfg(t, "")
	_, err := loadMCPConfig(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("want an error for a missing --mcp path, got nil")
	}
	if !strings.Contains(err.Error(), "mcp config") {
		t.Errorf("want error mentioning %q, got %v", "mcp config", err)
	}
}

// Malformed JSON is fatal even at the default path — it means the user
// has a config and it's broken, which is not the same as having none.
func TestLoadMCPConfig_MalformedDefaultIsFatal(t *testing.T) {
	writeCfg(t, "{not json")
	if _, err := loadMCPConfig(""); err == nil {
		t.Fatal("want an error for malformed default config, got nil")
	}
}

func TestLoadMCPConfig_ExplicitPathWins(t *testing.T) {
	writeCfg(t, `{"mcpServers":{"fromdefault":{"command":"true"}}}`)

	other := filepath.Join(t.TempDir(), "other.json")
	if err := os.WriteFile(other, []byte(`{"mcpServers":{"fromflag":{"command":"true"}}}`), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadMCPConfig(other)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 1 || cfg.Servers[0].Name != "fromflag" {
		t.Errorf("want the --mcp file to win, got %+v", cfg.Servers)
	}
}
