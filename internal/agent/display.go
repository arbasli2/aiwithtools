package agent

// Display receives events as the agent runs. Implementations live in
// the repl package (terminal) and tests (no-op or capture).
type Display interface {
	ToolCallStart(name string, args map[string]any)
	ToolCallEnd(name, output string, err error)
	AssistantFinal(content string)
}

type NopDisplay struct{}

func (NopDisplay) ToolCallStart(string, map[string]any) {}
func (NopDisplay) ToolCallEnd(string, string, error)    {}
func (NopDisplay) AssistantFinal(string)                {}
