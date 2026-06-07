package session

import (
	"encoding/json"
	"testing"
)

func TestLoad_DropsDanglingAssistantToolCalls(t *testing.T) {
	s := newStore(t)
	sess, _ := s.Create("m", "")
	_ = sess.AppendUser("hi")
	// assistant emits 2 tool_calls, only 1 is answered
	_ = sess.AppendAssistant("", []ToolCall{
		{Name: "a", Arguments: map[string]any{}},
		{Name: "b", Arguments: map[string]any{}},
	})
	_ = sess.AppendTool("a", "result-a")

	loaded, err := s.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := loaded.Messages()
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 1 {
		for _, m := range msgs {
			b, _ := json.Marshal(m)
			t.Logf("kept: %s", b)
		}
		t.Fatalf("after recovery len = %d, want 1 (user only)", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("kept[0].Role = %q", msgs[0].Role)
	}
}

func TestLoad_KeepsCompleteAssistantToolCalls(t *testing.T) {
	s := newStore(t)
	sess, _ := s.Create("m", "")
	_ = sess.AppendUser("hi")
	_ = sess.AppendAssistant("", []ToolCall{
		{Name: "a", Arguments: map[string]any{}},
	})
	_ = sess.AppendTool("a", "result-a")

	loaded, err := s.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	msgs, _ := loaded.Messages()
	if len(msgs) != 3 {
		t.Errorf("len = %d, want 3", len(msgs))
	}
}

func TestLoad_NoToolCallsIsNoop(t *testing.T) {
	s := newStore(t)
	sess, _ := s.Create("m", "")
	_ = sess.AppendUser("hi")
	_ = sess.AppendAssistant("answer", nil)

	loaded, err := s.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	msgs, _ := loaded.Messages()
	if len(msgs) != 2 {
		t.Errorf("len = %d, want 2", len(msgs))
	}
}
