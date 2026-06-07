package llm

import (
	"strings"
	"testing"
	"time"
)

func TestBuildSystemMessage_IncludesUserPromptAndDate(t *testing.T) {
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	out := BuildSystemMessage("be helpful", now, now)
	if !strings.Contains(out, "be helpful") {
		t.Errorf("missing user prompt: %q", out)
	}
	if !strings.Contains(out, "Today is 2026-06-07") {
		t.Errorf("missing date line: %q", out)
	}
}

func TestBuildSystemMessage_EmptyUserPromptOK(t *testing.T) {
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	out := BuildSystemMessage("", now, now)
	if !strings.Contains(out, "Today is 2026-06-07") {
		t.Errorf("missing date line in empty case: %q", out)
	}
}

func TestBuildSystemMessage_MultiDayAddsNote(t *testing.T) {
	start := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	out := BuildSystemMessage("be helpful", start, now)
	if !strings.Contains(out, "earlier messages from prior days") {
		t.Errorf("missing multi-day note: %q", out)
	}
}

func TestBuildSystemMessage_SameDayNoMultiDayNote(t *testing.T) {
	start := time.Date(2026, 6, 7, 0, 1, 0, 0, time.UTC)
	now := time.Date(2026, 6, 7, 23, 59, 0, 0, time.UTC)
	out := BuildSystemMessage("", start, now)
	if strings.Contains(out, "earlier messages from prior days") {
		t.Errorf("multi-day note should not appear within 24h: %q", out)
	}
}
