package llm

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"
)

type Client struct {
	ollama *api.Client
}

func New(c *api.Client) *Client { return &Client{ollama: c} }

// Chat sends a single non-streaming chat request and returns the final
// assistant message plus the Ollama "done_reason" (e.g. "stop",
// "length", "content_filter"). Callers use the reason to explain
// unexpectedly short or empty outputs to the user.
func (c *Client) Chat(ctx context.Context, model string, messages []api.Message, tools api.Tools) (*api.Message, string, error) {
	streamFalse := false
	req := &api.ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   &streamFalse,
		Tools:    tools,
	}

	var last api.Message
	var doneReason string
	err := c.ollama.Chat(ctx, req, func(resp api.ChatResponse) error {
		// With Stream=false the callback fires once with the final
		// response, so DoneReason here is the terminal one.
		last = resp.Message
		doneReason = resp.DoneReason
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("ollama chat: %w", err)
	}
	return &last, doneReason, nil
}
