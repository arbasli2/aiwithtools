package main

import "testing"

func TestConfigDir_DefaultsToHomeConfig(t *testing.T) {
	got := configDir("/home/amir", "")
	if got != "/home/amir/.config/aiwithtools" {
		t.Errorf("got %q", got)
	}
}

func TestConfigDir_XDGOverride(t *testing.T) {
	got := configDir("/home/amir", "/tmp/xdg")
	if got != "/tmp/xdg/aiwithtools" {
		t.Errorf("got %q", got)
	}
}

func TestDataDir_DefaultsToHomeShare(t *testing.T) {
	got := dataDir("/home/amir", "")
	if got != "/home/amir/.local/share/aiwithtools" {
		t.Errorf("got %q", got)
	}
}

func TestMCPConfigPath_DefaultsToConfigDir(t *testing.T) {
	got := mcpConfigPath("/home/amir/.config/aiwithtools", "")
	if got != "/home/amir/.config/aiwithtools/mcp.json" {
		t.Errorf("got %q", got)
	}
}

func TestMCPConfigPath_OverrideWinsVerbatim(t *testing.T) {
	got := mcpConfigPath("/home/amir/.config/aiwithtools", "/tmp/other/mcp.json")
	if got != "/tmp/other/mcp.json" {
		t.Errorf("got %q", got)
	}
}

// A relative --mcp path is passed through as-is, so it resolves against
// the process working directory rather than the config dir.
func TestMCPConfigPath_RelativeOverrideNotJoinedToConfigDir(t *testing.T) {
	got := mcpConfigPath("/home/amir/.config/aiwithtools", "etc/example/mcp.json")
	if got != "etc/example/mcp.json" {
		t.Errorf("got %q", got)
	}
}
