package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/ollama/ollama/api"
)

type Client struct {
	ollama *api.Client
}

func New(c *api.Client) *Client { return &Client{ollama: c} }

// ChatResult is the synchronous result of a non-streaming chat call.
// PromptTokens / EvalTokens come from the embedded Metrics on the
// final ChatResponse; both may be 0 if Ollama didn't report them.
type ChatResult struct {
	Message      *api.Message
	DoneReason   string
	PromptTokens int // input (prompt) tokens
	EvalTokens   int // output (generated) tokens
}

// Chat sends a single non-streaming chat request and returns the
// assistant message, Ollama's done_reason, and per-turn token counts.
func (c *Client) Chat(ctx context.Context, model string, messages []api.Message, tools api.Tools) (*ChatResult, error) {
	streamFalse := false
	req := &api.ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   &streamFalse,
		Tools:    tools,
	}

	var res ChatResult
	var last api.Message
	err := c.ollama.Chat(ctx, req, func(resp api.ChatResponse) error {
		// With Stream=false the callback fires once with the final
		// response, so all of these fields are the terminal values.
		last = resp.Message
		res.DoneReason = resp.DoneReason
		res.PromptTokens = resp.PromptEvalCount
		res.EvalTokens = resp.EvalCount
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ollama chat: %w", err)
	}
	res.Message = &last
	return &res, nil
}

// ContextLength returns the model's maximum context window in tokens,
// or 0 if it can't be determined (older Ollama versions, cloud models
// without /api/show support, etc.). Errors from Show are returned but
// the caller can treat a 0 result as "unknown" and proceed.
func (c *Client) ContextLength(ctx context.Context, model string) (int, error) {
	show, err := c.ollama.Show(ctx, &api.ShowRequest{Model: model})
	if err != nil {
		return 0, fmt.Errorf("ollama show: %w", err)
	}
	// ModelInfo keys are architecture-prefixed, e.g. "llama.context_length",
	// "qwen2.context_length", "nemotron.context_length". Find any key
	// ending in ".context_length" and return its int value.
	for k, v := range show.ModelInfo {
		if !strings.HasSuffix(k, ".context_length") {
			continue
		}
		switch n := v.(type) {
		case float64:
			return int(n), nil
		case int:
			return n, nil
		case int64:
			return int(n), nil
		}
	}
	return 0, nil
}

// FormatTokens renders a token count compactly:
// 1234 -> "1.2K", 12345 -> "12K", 1234567 -> "1.2M".
// Shared so both the agent's context-warning text and the cmd layer's
// startup banner / /info table use identical formatting.
func FormatTokens(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 10_000:
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	case n < 1_000_000:
		return fmt.Sprintf("%dK", n/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}
