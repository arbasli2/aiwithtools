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

	res, err := c.Chat(context.Background(), "test",
		[]api.Message{{Role: "user", Content: "hi"}},
		nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if res.Message.Role != "assistant" || res.Message.Content != "hello!" {
		t.Errorf("msg = %+v", res.Message)
	}
}

func TestChat_ReturnsDoneReasonAndTokenCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		resp := api.ChatResponse{
			Model:      "test",
			Message:    api.Message{Role: "assistant", Content: "truncated..."},
			Done:       true,
			DoneReason: "length",
		}
		resp.PromptEvalCount = 1024
		resp.EvalCount = 256
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := New(api.NewClient(u, http.DefaultClient))

	res, err := c.Chat(context.Background(), "test",
		[]api.Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.DoneReason != "length" {
		t.Errorf("reason = %q, want length", res.DoneReason)
	}
	if res.PromptTokens != 1024 || res.EvalTokens != 256 {
		t.Errorf("tokens = (%d, %d), want (1024, 256)", res.PromptTokens, res.EvalTokens)
	}
}

func TestChat_StreamsChunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		enc := json.NewEncoder(w)
		// Three deltas: "Hello", " ", "world"; then a final Done chunk.
		for _, d := range []string{"Hello", " ", "world"} {
			_ = enc.Encode(api.ChatResponse{
				Model:   "test",
				Message: api.Message{Role: "assistant", Content: d},
				Done:    false,
			})
		}
		final := api.ChatResponse{Model: "test", Done: true, DoneReason: "stop"}
		final.PromptEvalCount = 4
		final.EvalCount = 3
		_ = enc.Encode(final)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := New(api.NewClient(u, http.DefaultClient))

	var seen []string
	res, err := c.Chat(context.Background(), "test",
		[]api.Message{{Role: "user", Content: "hi"}}, nil,
		func(delta string) { seen = append(seen, delta) },
	)
	if err != nil {
		t.Fatal(err)
	}
	wantDeltas := []string{"Hello", " ", "world"}
	if len(seen) != len(wantDeltas) {
		t.Fatalf("seen=%v, want %v", seen, wantDeltas)
	}
	for i := range seen {
		if seen[i] != wantDeltas[i] {
			t.Errorf("seen[%d] = %q, want %q", i, seen[i], wantDeltas[i])
		}
	}
	if res.Message.Content != "Hello world" {
		t.Errorf("accumulated content = %q", res.Message.Content)
	}
	if res.PromptTokens != 4 || res.EvalTokens != 3 {
		t.Errorf("tokens = (%d, %d)", res.PromptTokens, res.EvalTokens)
	}
}

func TestChat_NilChunkCallbackStillAccumulates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		enc := json.NewEncoder(w)
		_ = enc.Encode(api.ChatResponse{
			Model:   "test",
			Message: api.Message{Role: "assistant", Content: "ab"},
		})
		_ = enc.Encode(api.ChatResponse{
			Model:   "test",
			Message: api.Message{Role: "assistant", Content: "cd"},
			Done:    true,
		})
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := New(api.NewClient(u, http.DefaultClient))

	res, err := c.Chat(context.Background(), "test",
		[]api.Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Message.Content != "abcd" {
		t.Errorf("content = %q, want abcd", res.Message.Content)
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

	res, err := c.Chat(context.Background(), "test",
		[]api.Message{{Role: "user", Content: "weather?"}},
		nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d", len(res.Message.ToolCalls))
	}
	if res.Message.ToolCalls[0].Function.Name != "weather__forecast" {
		t.Errorf("name = %q", res.Message.ToolCalls[0].Function.Name)
	}
}
