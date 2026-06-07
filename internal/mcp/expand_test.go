package mcp

import (
	"testing"
)

func TestExpand_Tilde(t *testing.T) {
	got := expandPath("~/foo/bar", "/home/amir", "amir")
	if got != "/home/amir/foo/bar" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_HomeVar(t *testing.T) {
	got := expandPath("$HOME/x", "/home/amir", "amir")
	if got != "/home/amir/x" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_UserVar(t *testing.T) {
	got := expandPath("/Users/$USER/x", "/home/amir", "amir")
	if got != "/Users/amir/x" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_OtherVarsPassThrough(t *testing.T) {
	got := expandPath("$FOO/x", "/home/amir", "amir")
	if got != "$FOO/x" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_NoExpansionWhenNothingToDo(t *testing.T) {
	got := expandPath("/abs/path", "/home/amir", "amir")
	if got != "/abs/path" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_WordBoundaryRespected(t *testing.T) {
	got := expandPath("$HOMEY/x", "/home/amir", "amir")
	if got != "$HOMEY/x" {
		t.Errorf("got %q, want $HOMEY/x", got)
	}
}
