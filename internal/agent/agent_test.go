package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/ollama/ollama/api"
)

type fakeLLM struct {
	responses []api.Message
	reasons   []string // optional per-response done_reason; default ""
	calls     int
}

func (f *fakeLLM) Chat(ctx context.Context, model string, msgs []api.Message, tools api.Tools) (*api.Message, string, error) {
	if f.calls >= len(f.responses) {
		return nil, "", errors.New("fakeLLM exhausted")
	}
	r := f.responses[f.calls]
	reason := ""
	if f.calls < len(f.reasons) {
		reason = f.reasons[f.calls]
	}
	f.calls++
	return &r, reason, nil
}

type fakeMCP struct {
	out map[string]string
	err map[string]error
}

func (f *fakeMCP) Tools() api.Tools { return nil }
func (f *fakeMCP) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	if e, ok := f.err[name]; ok {
		return "", e
	}
	return f.out[name], nil
}

type captureDisplay struct {
	starts []string
	ends   []string
	texts []string
}

func (c *captureDisplay) ToolCallStart(n string, _ map[string]any) {
	c.starts = append(c.starts, n)
}
func (c *captureDisplay) ToolCallEnd(n, _ string, _ error) { c.ends = append(c.ends, n) }
func (c *captureDisplay) AssistantText(s string)          { c.texts = append(c.texts, s) }

type fakeSession struct {
	msgs []api.Message
}

func (f *fakeSession) AppendUser(c string) error {
	f.msgs = append(f.msgs, api.Message{Role: "user", Content: c})
	return nil
}
func (f *fakeSession) AppendAssistant(c string, tcs []api.ToolCall) error {
	f.msgs = append(f.msgs, api.Message{Role: "assistant", Content: c, ToolCalls: tcs})
	return nil
}
func (f *fakeSession) AppendTool(name, c string) error {
	f.msgs = append(f.msgs, api.Message{Role: "tool", Content: c, ToolName: name})
	return nil
}
func (f *fakeSession) Messages() []api.Message { return f.msgs }

func newAgent(llm LLM, mcp MCP, sess Session, disp Display, maxIter int) *Agent {
	return &Agent{LLM: llm, MCP: mcp, Sess: sess, Display: disp, Model: "m", MaxIter: maxIter}
}

func toolCallWith(name string, args map[string]any) api.ToolCall {
	a := api.NewToolCallFunctionArguments()
	for k, v := range args {
		a.Set(k, v)
	}
	return api.ToolCall{Function: api.ToolCallFunction{Name: name, Arguments: a}}
}

func TestRun_NoToolCallsReturnsFinal(t *testing.T) {
	llm := &fakeLLM{responses: []api.Message{
		{Role: "assistant", Content: "hi!"},
	}}
	disp := &captureDisplay{}
	a := newAgent(llm, &fakeMCP{}, &fakeSession{}, disp, 5)

	if err := a.Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if len(disp.texts) != 1 || disp.texts[0] != "hi!" {
		t.Errorf("texts = %v", disp.texts)
	}
}

func TestRun_ToolCallThenFinal(t *testing.T) {
	llm := &fakeLLM{responses: []api.Message{
		{Role: "assistant", ToolCalls: []api.ToolCall{toolCallWith("echo", map[string]any{"text": "hi"})}},
		{Role: "assistant", Content: "done"},
	}}
	mcp := &fakeMCP{out: map[string]string{"echo": "echo: hi"}}
	disp := &captureDisplay{}
	a := newAgent(llm, mcp, &fakeSession{}, disp, 5)

	if err := a.Run(context.Background(), "say hi"); err != nil {
		t.Fatal(err)
	}
	if len(disp.starts) != 1 || disp.starts[0] != "echo" {
		t.Errorf("starts = %v", disp.starts)
	}
	if len(disp.texts) != 1 || disp.texts[0] != "done" {
		t.Errorf("texts = %v", disp.texts)
	}
}

func TestRun_ToolErrorIsSerializedNotFatal(t *testing.T) {
	llm := &fakeLLM{responses: []api.Message{
		{Role: "assistant", ToolCalls: []api.ToolCall{toolCallWith("broken", map[string]any{})}},
		{Role: "assistant", Content: "recovered"},
	}}
	mcp := &fakeMCP{err: map[string]error{"broken": errors.New("bang")}}
	a := newAgent(llm, mcp, &fakeSession{}, &captureDisplay{}, 5)

	if err := a.Run(context.Background(), "go"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestRun_AbnormalDoneReasonAppendedToContent(t *testing.T) {
	llm := &fakeLLM{
		responses: []api.Message{{Role: "assistant", Content: "partial answer"}},
		reasons:   []string{"length"},
	}
	disp := &captureDisplay{}
	a := newAgent(llm, &fakeMCP{}, &fakeSession{}, disp, 5)
	if err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if len(disp.texts) != 2 {
		t.Fatalf("texts = %q, want [content, stopped-note]", disp.texts)
	}
	if disp.texts[0] != "partial answer" {
		t.Errorf("texts[0] = %q", disp.texts[0])
	}
	if disp.texts[1] != "(model stopped: length)" {
		t.Errorf("texts[1] = %q", disp.texts[1])
	}
}

func TestRun_AbnormalDoneReasonWithThinkingOnly(t *testing.T) {
	llm := &fakeLLM{
		responses: []api.Message{{Role: "assistant", Content: "", Thinking: "let me think"}},
		reasons:   []string{"length"},
	}
	disp := &captureDisplay{}
	a := newAgent(llm, &fakeMCP{}, &fakeSession{}, disp, 5)
	if err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	want := "(thinking only — no final answer, stopped: length)\nlet me think"
	if len(disp.texts) != 1 || disp.texts[0] != want {
		t.Errorf("texts = %q, want [%q]", disp.texts, want)
	}
}

func TestRun_AbnormalDoneReasonWithEmptyContent(t *testing.T) {
	llm := &fakeLLM{
		responses: []api.Message{{Role: "assistant", Content: ""}},
		reasons:   []string{"content_filter"},
	}
	disp := &captureDisplay{}
	a := newAgent(llm, &fakeMCP{}, &fakeSession{}, disp, 5)
	if err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if len(disp.texts) != 1 || disp.texts[0] != "(model returned no content — stopped: content_filter)" {
		t.Errorf("texts = %q", disp.texts)
	}
}

func TestRun_EmptyContentNoToolCallsShowsPlaceholder(t *testing.T) {
	llm := &fakeLLM{responses: []api.Message{
		{Role: "assistant", Content: ""},
	}}
	disp := &captureDisplay{}
	a := newAgent(llm, &fakeMCP{}, &fakeSession{}, disp, 5)
	if err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if len(disp.texts) != 1 || disp.texts[0] != "(model returned no content)" {
		t.Errorf("texts = %q, want placeholder", disp.texts)
	}
}

func TestRun_ThinkingOnlyShowsThinking(t *testing.T) {
	llm := &fakeLLM{responses: []api.Message{
		{Role: "assistant", Content: "", Thinking: "let me think..."},
	}}
	disp := &captureDisplay{}
	a := newAgent(llm, &fakeMCP{}, &fakeSession{}, disp, 5)
	if err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if len(disp.texts) != 1 || disp.texts[0] != "(thinking only — no final answer)\nlet me think..." {
		t.Errorf("texts = %q, want thinking surface", disp.texts)
	}
}

func TestRun_CommentaryAlongsideToolCallShowsBoth(t *testing.T) {
	llm := &fakeLLM{responses: []api.Message{
		{Role: "assistant", Content: "Let me check.", ToolCalls: []api.ToolCall{toolCallWith("echo", map[string]any{})}},
		{Role: "assistant", Content: "Done."},
	}}
	mcp := &fakeMCP{out: map[string]string{"echo": "ok"}}
	disp := &captureDisplay{}
	a := newAgent(llm, mcp, &fakeSession{}, disp, 5)
	if err := a.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if len(disp.texts) != 2 || disp.texts[0] != "Let me check." || disp.texts[1] != "Done." {
		t.Errorf("texts = %q, want [\"Let me check.\", \"Done.\"]", disp.texts)
	}
}

func TestRun_NilArgsNormalizedToEmptyMap(t *testing.T) {
	// Tool call with NO arguments (zero-value ToolCallFunctionArguments).
	tc := api.ToolCall{Function: api.ToolCallFunction{Name: "echo"}}
	llm := &fakeLLM{responses: []api.Message{
		{Role: "assistant", ToolCalls: []api.ToolCall{tc}},
		{Role: "assistant", Content: "done"},
	}}
	captured := &argCapturingMCP{out: map[string]string{"echo": "ok"}}
	a := newAgent(llm, captured, &fakeSession{}, &captureDisplay{}, 5)
	if err := a.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if captured.lastArgs == nil {
		t.Error("MCP.Call received nil args; agent should normalize to empty map")
	}
}

type argCapturingMCP struct {
	out      map[string]string
	lastArgs map[string]any
}

func (f *argCapturingMCP) Tools() api.Tools { return nil }
func (f *argCapturingMCP) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	f.lastArgs = args
	return f.out[name], nil
}

func TestRun_MaxIterReachedErrors(t *testing.T) {
	looping := api.Message{Role: "assistant", ToolCalls: []api.ToolCall{toolCallWith("echo", map[string]any{})}}
	llm := &fakeLLM{responses: []api.Message{looping, looping, looping}}
	mcp := &fakeMCP{out: map[string]string{"echo": ""}}
	a := newAgent(llm, mcp, &fakeSession{}, &captureDisplay{}, 2)

	err := a.Run(context.Background(), "x")
	if err == nil || !errors.Is(err, ErrMaxIterations) {
		t.Errorf("err = %v, want ErrMaxIterations", err)
	}
}
