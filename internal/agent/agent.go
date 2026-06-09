package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/ollama/ollama/api"

	"aiwithtools/internal/llm"
)

var ErrMaxIterations = errors.New("max iterations reached")

type LLM interface {
	// Chat returns the assistant message, Ollama's done_reason, and
	// per-turn token counts. If onChunk is non-nil it is called for
	// each content delta as the stream arrives; pass nil to receive
	// only the final accumulated result.
	Chat(ctx context.Context, model string, msgs []api.Message, tools api.Tools, onChunk llm.ChunkFunc) (*llm.ChatResult, error)
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

	// ContextLength is the model's maximum context window in tokens.
	// Zero means unknown — no warning will be shown.
	ContextLength int
	// WarnAtPercent is the threshold above which a context-usage
	// warning is shown after a turn. Zero defaults to 80.
	WarnAtPercent int
	// Stream controls whether assistant content is forwarded to the
	// Display as it arrives. When false, the model's content is still
	// streamed under the hood but only emitted as a single block via
	// AssistantText after the turn completes.
	Stream bool

	// LastPromptTokens / LastEvalTokens hold the most recent Chat's
	// token counts so /info can report them without re-querying.
	LastPromptTokens int
	LastEvalTokens   int
}

func (a *Agent) Run(ctx context.Context, userInput string) error {
	if err := a.Sess.AppendUser(userInput); err != nil {
		return fmt.Errorf("append user: %w", err)
	}

	for i := 0; i < a.MaxIter; i++ {
		// streamed tracks whether the chunk callback actually fired —
		// the closure mutates it on the first delta. We need this signal
		// after Chat returns so we know whether to (a) emit the closing
		// newline via AssistantStreamEnd and (b) skip the otherwise-
		// duplicate AssistantText call below.
		var onChunk llm.ChunkFunc
		streamed := false
		if a.Stream {
			onChunk = func(delta string) {
				a.Display.AssistantStreamDelta(delta)
				streamed = true
			}
		}
		result, err := a.LLM.Chat(ctx, a.Model, a.Sess.Messages(), a.MCP.Tools(), onChunk)
		if streamed {
			a.Display.AssistantStreamEnd()
		}
		if err != nil {
			return fmt.Errorf("chat: %w", err)
		}
		resp := result.Message
		doneReason := result.DoneReason
		a.LastPromptTokens = result.PromptTokens
		a.LastEvalTokens = result.EvalTokens

		if err := a.Sess.AppendAssistant(resp.Content, resp.ToolCalls); err != nil {
			return fmt.Errorf("append assistant: %w", err)
		}

		// Show any text the model wrote, even if it also wants to call
		// tools — otherwise commentary like "Let me check the weather"
		// disappears. If streaming was on, we already emitted the
		// content live; only AssistantText again in non-streaming mode.
		if !streamed && resp.Content != "" {
			a.Display.AssistantText(resp.Content)
		}

		if len(resp.ToolCalls) == 0 {
			// Turn ends here. Make sure the user sees something, and
			// surface non-"stop" finish reasons (length, content_filter,
			// etc.) so unexpectedly short or empty outputs are explained
			// rather than appearing as a silent re-prompt.
			abnormal := doneReason != "" && doneReason != "stop"
			switch {
			case resp.Content != "" && abnormal:
				a.Display.AssistantText(fmt.Sprintf("(model stopped: %s)", doneReason))
			case resp.Content == "" && resp.Thinking != "" && abnormal:
				a.Display.AssistantText(fmt.Sprintf("(thinking only — no final answer, stopped: %s)\n%s", doneReason, resp.Thinking))
			case resp.Content == "" && resp.Thinking != "":
				a.Display.AssistantText("(thinking only — no final answer)\n" + resp.Thinking)
			case resp.Content == "" && abnormal:
				a.Display.AssistantText(fmt.Sprintf("(model returned no content — stopped: %s)", doneReason))
			case resp.Content == "":
				a.Display.AssistantText("(model returned no content)")
			}
			a.warnIfContextHigh()
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

// warnIfContextHigh shows a context-usage notice when the most recent
// turn's used tokens (prompt + reply) exceed WarnAtPercent of the
// model's context window. Called once per turn that ends without tool
// calls. Silently does nothing if ContextLength is unknown.
func (a *Agent) warnIfContextHigh() {
	if a.ContextLength <= 0 {
		return
	}
	used := a.LastPromptTokens + a.LastEvalTokens
	if used <= 0 {
		return
	}
	threshold := a.WarnAtPercent
	if threshold <= 0 {
		threshold = 80
	}
	pct := used * 100 / a.ContextLength
	if pct < threshold {
		return
	}
	a.Display.AssistantText(fmt.Sprintf(
		"(context %d%%: %s / %s — Ollama will start dropping oldest messages above 100%%)",
		pct, llm.FormatTokens(used), llm.FormatTokens(a.ContextLength),
	))
}
