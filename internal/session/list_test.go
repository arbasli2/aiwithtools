package session

import (
	"testing"
)

func TestLastForModel_ReturnsMostRecent(t *testing.T) {
	s := newStore(t)
	a, _ := s.Create("model-a", "")
	_, _ = s.Create("model-b", "")
	c, _ := s.Create("model-a", "")
	if err := c.AppendUser("hi"); err != nil {
		t.Fatal(err)
	}

	id, err := s.LastForModel("model-a")
	if err != nil {
		t.Fatal(err)
	}
	if id != c.ID {
		t.Errorf("LastForModel = %q, want %q (created after %q)", id, c.ID, a.ID)
	}
}

func TestLastForModel_NoneReturnsEmpty(t *testing.T) {
	s := newStore(t)
	id, err := s.LastForModel("model-z")
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Errorf("LastForModel = %q, want empty", id)
	}
}

func TestList_ReturnsSummaries(t *testing.T) {
	s := newStore(t)
	a, _ := s.Create("model-a", "")
	_ = a.AppendUser("first message")
	_, _ = s.Create("model-b", "")

	items, err := s.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("List len = %d, want 2", len(items))
	}
	var aSum *Summary
	for i := range items {
		if items[i].ID == a.ID {
			aSum = &items[i]
		}
	}
	if aSum == nil {
		t.Fatal("a not in list")
	}
	if aSum.FirstUserMessage != "first message" {
		t.Errorf("FirstUserMessage = %q", aSum.FirstUserMessage)
	}
	if aSum.MessageCount != 1 {
		t.Errorf("MessageCount = %d", aSum.MessageCount)
	}
}

func TestList_FilterByModel(t *testing.T) {
	s := newStore(t)
	_, _ = s.Create("model-a", "")
	_, _ = s.Create("model-b", "")
	items, err := s.List("model-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(items))
	}
	if items[0].Model != "model-a" {
		t.Errorf("Model = %q", items[0].Model)
	}
}

func TestClear_DropsMessagesKeepsSession(t *testing.T) {
	s := newStore(t)
	sess, _ := s.Create("m", "sys")
	_ = sess.AppendUser("hi")

	if err := sess.Clear(); err != nil {
		t.Fatal(err)
	}

	msgs, _ := sess.Messages()
	if len(msgs) != 0 {
		t.Errorf("after Clear, msgs len = %d", len(msgs))
	}
	id, _ := s.LastForModel("m")
	if id != sess.ID {
		t.Error("session row gone after Clear")
	}
}

func TestDelete_RemovesSessionAndMessages(t *testing.T) {
	s := newStore(t)
	sess, _ := s.Create("m", "")
	_ = sess.AppendUser("hi")

	if err := s.Delete(sess.ID); err != nil {
		t.Fatal(err)
	}

	id, _ := s.LastForModel("m")
	if id != "" {
		t.Error("session still present after Delete")
	}
}
