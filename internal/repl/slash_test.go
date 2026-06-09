package repl

import "testing"

func TestParseSlash_KnownCommands(t *testing.T) {
	cases := map[string]SlashCommand{
		"/clear":  SlashClear,
		"/exit":   SlashExit,
		"/bye":    SlashExit,
		"/tools":  SlashTools,
		"/help":   SlashHelp,
		"/info":   SlashInfo,
		"/skills": SlashSkills,
	}
	for in, want := range cases {
		if got, ok := ParseSlash(in); !ok || got != want {
			t.Errorf("ParseSlash(%q) = (%v, %v), want (%v, true)", in, got, ok, want)
		}
	}
}

func TestParseSlash_NonSlashReturnsFalse(t *testing.T) {
	if _, ok := ParseSlash("hello"); ok {
		t.Error("non-slash input recognized as command")
	}
}

func TestParseSlash_UnknownSlashReturnsFalse(t *testing.T) {
	if _, ok := ParseSlash("/wat"); ok {
		t.Error("unknown slash input recognized")
	}
}
