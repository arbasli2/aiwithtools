package mcp

import (
	"fmt"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// flattenResult turns an MCP CallToolResult into a single string for
// the Ollama tool-role message. Text blocks are joined with newlines;
// image / resource blocks become short placeholders. Returns "" for nil.
func flattenResult(r *mcpgo.CallToolResult) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	for i, c := range r.Content {
		if i > 0 {
			b.WriteByte('\n')
		}
		switch v := c.(type) {
		case mcpgo.TextContent:
			b.WriteString(v.Text)
		case mcpgo.ImageContent:
			fmt.Fprintf(&b, "[image: %s, omitted]", v.MIMEType)
		case mcpgo.EmbeddedResource:
			b.WriteString("[embedded resource, omitted]")
		default:
			fmt.Fprintf(&b, "[unknown content type %T]", v)
		}
	}
	return b.String()
}
