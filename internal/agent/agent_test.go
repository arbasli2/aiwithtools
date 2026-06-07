package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/ollama/ollama/api"
)

type fakeLLM struct {
	responses []api.Message
	calls     int
}

func (f *fakeLLM) Chat(ctx context.Context, model string, msgs []api.Message, tools api.Tools) (*api.Message, error) {
	if f.calls >= len(f.responses) {
		return nil, errors.New("fakeLLM exhausted")
	}
	r := f.responses[f.calls]
	f.calls++
	return &r, nil
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
	finals []string
}

func (c *captureDisplay) ToolCallStart(n string, _ map[string]any) {
	c.starts = append(c.starts, n)
}
func (c *captureDisplay) ToolCallEnd(n, _ string, _ error) { c.ends = append(c.ends, n) }
func (c *captureDisplay) AssistantFinal(s string)          { c.finals = append(c.finals, s) }

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
	if len(disp.finals) != 1 || disp.finals[0] != "hi!" {
		t.Errorf("finals = %v", disp.finals)
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
	if len(disp.finals) != 1 || disp.finals[0] != "done" {
		t.Errorf("finals = %v", disp.finals)
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
