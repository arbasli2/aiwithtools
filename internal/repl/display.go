package repl

import (
	"encoding/json"
	"fmt"
	"io"
)

// Terminal implements agent.Display by writing to an io.Writer.
type Terminal struct {
	Out     io.Writer
	Verbose bool
}

func (t Terminal) ToolCallStart(name string, args map[string]any) {
	b, _ := json.Marshal(args)
	fmt.Fprintf(t.Out, "\x1b[2m→ %s(%s)\x1b[0m\n", name, string(b))
}

func (t Terminal) ToolCallEnd(name, output string, err error) {
	if err != nil {
		fmt.Fprintf(t.Out, "\x1b[31m← %s ERROR: %s\x1b[0m\n", name, err)
		return
	}
	if t.Verbose {
		fmt.Fprintf(t.Out, "\x1b[2m← %s:\n%s\x1b[0m\n", name, output)
	} else {
		fmt.Fprintf(t.Out, "\x1b[2m← %s (%d chars)\x1b[0m\n", name, len(output))
	}
}

func (t Terminal) AssistantText(content string) {
	fmt.Fprintln(t.Out, content)
}
