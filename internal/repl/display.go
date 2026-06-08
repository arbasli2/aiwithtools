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
	fmt.Fprintln(t.Out, Dim(fmt.Sprintf("→ %s(%s)", name, string(b))))
}

func (t Terminal) ToolCallEnd(name, output string, err error) {
	if err != nil {
		fmt.Fprintln(t.Out, Red(fmt.Sprintf("← %s ERROR: %s", name, err)))
		return
	}
	if t.Verbose {
		fmt.Fprintln(t.Out, Dim(fmt.Sprintf("← %s:\n%s", name, output)))
	} else {
		fmt.Fprintln(t.Out, Dim(fmt.Sprintf("← %s (%d chars)", name, len(output))))
	}
}

func (t Terminal) AssistantText(content string) {
	fmt.Fprintln(t.Out, content)
}
