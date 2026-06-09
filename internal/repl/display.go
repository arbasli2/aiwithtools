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

// AssistantStreamDelta writes a streaming chunk raw with no trailing
// newline. Stdout writes are flushed implicitly by os.Stdout so the
// user sees tokens as they arrive.
func (t Terminal) AssistantStreamDelta(delta string) {
	fmt.Fprint(t.Out, delta)
}

// AssistantStreamEnd emits the single newline that closes a streamed
// turn — separated from delta writes so the stream renders as one line
// in the terminal without per-chunk newlines.
func (t Terminal) AssistantStreamEnd() {
	fmt.Fprintln(t.Out)
}
