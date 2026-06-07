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

		// Show any text the model wrote, even if it also wants to call
		// tools — otherwise commentary like "Let me check the weather"
		// disappears.
		if resp.Content != "" {
			a.Display.AssistantText(resp.Content)
		}

		if len(resp.ToolCalls) == 0 {
			// Turn ends here. If the model returned nothing visible at
			// all, surface a placeholder rather than printing a blank
			// line and re-prompting — otherwise the user can't tell
			// whether the model errored, refused, or just thought.
			if resp.Content == "" {
				if resp.Thinking != "" {
					a.Display.AssistantText("(thinking only — no final answer)\n" + resp.Thinking)
				} else {
					a.Display.AssistantText("(model returned no content)")
				}
			}
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
