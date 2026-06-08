package repl

import (
	"os"
	"testing"
)

func TestDetectColor_NonTTYDisabled(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if detectColor(f) {
		t.Error("regular file should not enable colour")
	}
}

func TestDetectColor_NoColorEnvDisables(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	f, _ := os.CreateTemp(t.TempDir(), "out")
	defer f.Close()
	if detectColor(f) {
		t.Error("NO_COLOR=1 should disable colour")
	}
}

func TestColorize_DisabledIsIdentity(t *testing.T) {
	saved := colorEnabled
	t.Cleanup(func() { colorEnabled = saved })
	colorEnabled = false
	if got := Colorize(AnsiRed, "hello"); got != "hello" {
		t.Errorf("got %q, want plain hello", got)
	}
}

func TestColorize_EnabledWrapsWithEscape(t *testing.T) {
	saved := colorEnabled
	t.Cleanup(func() { colorEnabled = saved })
	colorEnabled = true
	got := Colorize(AnsiCyan, "x")
	want := "\x1b[36mx\x1b[0m"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
