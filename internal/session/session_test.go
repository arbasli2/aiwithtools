package session

import (
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreate_PersistsSession(t *testing.T) {
	s := newStore(t)
	sess, err := s.Create("qwen3:cloud", "you are helpful")
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID == "" {
		t.Fatal("ID is empty")
	}
	if sess.Model != "qwen3:cloud" {
		t.Errorf("Model = %q, want qwen3:cloud", sess.Model)
	}
	if sess.System != "you are helpful" {
		t.Errorf("System = %q", sess.System)
	}
	if sess.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
}

func TestAppend_StoresInOrder(t *testing.T) {
	s := newStore(t)
	sess, err := s.Create("m", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := sess.AppendUser("hello"); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendAssistant("hi", nil); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendTool("weather__forecast", "sunny"); err != nil {
		t.Fatal(err)
	}

	msgs, err := sess.Messages()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hi" {
		t.Errorf("msgs[1] = %+v", msgs[1])
	}
	if msgs[2].Role != "tool" || msgs[2].ToolName != "weather__forecast" {
		t.Errorf("msgs[2] = %+v", msgs[2])
	}
}

func TestAppend_BumpsUpdatedAt(t *testing.T) {
	s := newStore(t)
	sess, err := s.Create("m", "")
	if err != nil {
		t.Fatal(err)
	}
	before := sess.UpdatedAt.UnixNano()
	time.Sleep(2 * time.Millisecond)
	if err := sess.AppendUser("hi"); err != nil {
		t.Fatal(err)
	}

	var updated int64
	if err := s.db.QueryRow(`SELECT updated_at FROM sessions WHERE id = ?`, sess.ID).Scan(&updated); err != nil {
		t.Fatal(err)
	}
	if updated <= before {
		t.Errorf("updated_at not bumped: was %d, now %d", before, updated)
	}
}

func TestAppendAssistant_PersistsToolCallsJSON(t *testing.T) {
	s := newStore(t)
	sess, err := s.Create("m", "")
	if err != nil {
		t.Fatal(err)
	}
	tcs := []ToolCall{{Name: "weather__forecast", Arguments: map[string]any{"city": "Tokyo"}}}
	if err := sess.AppendAssistant("", tcs); err != nil {
		t.Fatal(err)
	}
	msgs, err := sess.Messages()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs[0].ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d", len(msgs[0].ToolCalls))
	}
	if msgs[0].ToolCalls[0].Name != "weather__forecast" {
		t.Errorf("got name %q", msgs[0].ToolCalls[0].Name)
	}
}
