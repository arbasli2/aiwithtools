package llm

import (
	"encoding/json"
	"fmt"

	"github.com/ollama/ollama/api"
)

// Tool is our internal representation of an MCP tool exposed to the
// model. InputSchema is a JSON Schema (object).
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ToOllamaTool converts one MCP-style tool definition to api.Tool.
// Returns an error if InputSchema can't be unmarshalled into the
// Ollama parameter struct — callers should log and skip rather than
// silently registering a broken tool.
func ToOllamaTool(t Tool) (api.Tool, error) {
	var fn api.ToolFunction
	fn.Name = t.Name
	fn.Description = t.Description
	if err := json.Unmarshal(t.InputSchema, &fn.Parameters); err != nil {
		return api.Tool{}, fmt.Errorf("decode schema for %q: %w", t.Name, err)
	}
	return api.Tool{Type: "function", Function: fn}, nil
}

// ToOllamaTools converts a batch, skipping tools whose schema fails to
// decode. The returned errors slice has one entry per skipped tool.
func ToOllamaTools(in []Tool) (api.Tools, []error) {
	out := make(api.Tools, 0, len(in))
	var errs []error
	for _, t := range in {
		tool, err := ToOllamaTool(t)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, tool)
	}
	return out, errs
}
