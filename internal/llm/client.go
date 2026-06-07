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
// assistant message. Tools may be empty.
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
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ollama chat: %w", err)
	}
	return &last, nil
}
