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
