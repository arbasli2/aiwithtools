package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/ollama/ollama/api"
)

var ErrMaxIterations = errors.New("max iterations reached")

type LLM interface {
	Chat(ctx context.Context, model string, msgs []api.Message, tools api.Tools) (*api.Message, error)
}

type MCP interface {
	Tools() api.Tools
	Call(ctx context.Context, name string, args map[string]any) (string, error)
}

type Session interface {
	AppendUser(content string) error
	AppendAssistant(content string, toolCalls []api.ToolCall) error
	AppendTool(name, content string) error
	Messages() []api.Message
}

type Agent struct {
	LLM     LLM
	MCP     MCP
	Sess    Session
	Display Display
	Model   string
	MaxIter int
}

func (a *Agent) Run(ctx context.Context, userInput string) error {
	if err := a.Sess.AppendUser(userInput); err != nil {
		return fmt.Errorf("append user: %w", err)
	}

	for i := 0; i < a.MaxIter; i++ {
		resp, err := a.LLM.Chat(ctx, a.Model, a.Sess.Messages(), a.MCP.Tools())
		if err != nil {
			return fmt.Errorf("chat: %w", err)
		}

		if err := a.Sess.AppendAssistant(resp.Content, resp.ToolCalls); err != nil {
			return fmt.Errorf("append assistant: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			a.Display.AssistantFinal(resp.Content)
			return nil
		}

		for _, tc := range resp.ToolCalls {
			args := tc.Function.Arguments.ToMap()
			if args == nil {
				// Some MCP servers reject JSON `null` for arguments and
				// expect at least `{}`. Normalize so empty-args tool
				// calls work regardless of server strictness.
				args = map[string]any{}
			}
			a.Display.ToolCallStart(tc.Function.Name, args)

			out, callErr := a.MCP.Call(ctx, tc.Function.Name, args)
			content := out
			if callErr != nil {
				content = fmt.Sprintf("ERROR: %s", callErr)
			}
			a.Display.ToolCallEnd(tc.Function.Name, content, callErr)

			if err := a.Sess.AppendTool(tc.Function.Name, content); err != nil {
				return fmt.Errorf("append tool: %w", err)
			}
		}
	}
	return fmt.Errorf("%w (limit=%d)", ErrMaxIterations, a.MaxIter)
}
