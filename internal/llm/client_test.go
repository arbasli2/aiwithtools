package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ollama/ollama/api"
)

func TestChat_ReturnsFinalAssistantMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_ = json.NewEncoder(w).Encode(api.ChatResponse{
			Model:   "test",
			Message: api.Message{Role: "assistant", Content: "hello!"},
			Done:    true,
		})
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := New(api.NewClient(u, http.DefaultClient))

	msg, _, err := c.Chat(context.Background(), "test",
		[]api.Message{{Role: "user", Content: "hi"}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Role != "assistant" || msg.Content != "hello!" {
		t.Errorf("msg = %+v", msg)
	}
}

func TestChat_ReturnsDoneReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_ = json.NewEncoder(w).Encode(api.ChatResponse{
			Model:      "test",
			Message:    api.Message{Role: "assistant", Content: "truncated..."},
			Done:       true,
			DoneReason: "length",
		})
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := New(api.NewClient(u, http.DefaultClient))

	_, reason, err := c.Chat(context.Background(), "test",
		[]api.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reason != "length" {
		t.Errorf("reason = %q, want length", reason)
	}
}

func TestChat_SurfacesToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		args := api.NewToolCallFunctionArguments()
		args.Set("city", "Tokyo")
		_ = json.NewEncoder(w).Encode(api.ChatResponse{
			Model: "test",
			Message: api.Message{
				Role: "assistant",
				ToolCalls: []api.ToolCall{
					{Function: api.ToolCallFunction{Name: "weather__forecast", Arguments: args}},
				},
			},
			Done: true,
		})
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := New(api.NewClient(u, http.DefaultClient))

	msg, _, err := c.Chat(context.Background(), "test",
		[]api.Message{{Role: "user", Content: "weather?"}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Function.Name != "weather__forecast" {
		t.Errorf("name = %q", msg.ToolCalls[0].Function.Name)
	}
}
