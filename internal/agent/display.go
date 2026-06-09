package agent

// Display receives events as the agent runs. Implementations live in
// the repl package (terminal) and tests (no-op or capture).
type Display interface {
	ToolCallStart(name string, args map[string]any)
	ToolCallEnd(name, output string, err error)
	// AssistantText shows assistant-authored text as a single block.
	// Used for synthetic / atomic messages: placeholders like
	// "(model returned no content)" and notices like the context
	// warning. May be called more than once per turn.
	AssistantText(content string)
	// AssistantStreamDelta is called with each content chunk as the
	// model streams a response. Implementations write deltas raw, with
	// no trailing newline; AssistantStreamEnd is called once after the
	// last delta to emit a single newline.
	AssistantStreamDelta(delta string)
	AssistantStreamEnd()
}

type NopDisplay struct{}

func (NopDisplay) ToolCallStart(string, map[string]any) {}
func (NopDisplay) ToolCallEnd(string, string, error)    {}
func (NopDisplay) AssistantText(string)                 {}
func (NopDisplay) AssistantStreamDelta(string)          {}
func (NopDisplay) AssistantStreamEnd()                  {}
