package llm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ollama/ollama/api"
)

type Client struct {
	ollama *api.Client
}

func New(c *api.Client) *Client { return &Client{ollama: c} }

// Chat sends a single non-streaming chat request and returns the final
// assistant message. Tools may be empty.
//
// If the response's DoneReason is anything other than "stop" (e.g.
// "length" for context-limit truncation, or "content_filter"), it's
// logged at warn-level so the cause of unexpectedly short or empty
// outputs is visible to the user via stderr.
func (c *Client) Chat(ctx context.Context, model string, messages []api.Message, tools api.Tools) (*api.Message, error) {
	streamFalse := false
	req := &api.ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   &streamFalse,
		Tools:    tools,
	}

	var last api.Message
	err := c.ollama.Chat(ctx, req, func(resp api.ChatResponse) error {
		last = resp.Message
		if resp.Done && resp.DoneReason != "" && resp.DoneReason != "stop" {
			slog.Warn("ollama chat finished with non-stop reason",
				"model", model, "reason", resp.DoneReason)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ollama chat: %w", err)
	}
	return &last, nil
}
