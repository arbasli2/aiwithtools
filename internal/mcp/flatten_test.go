package mcp

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestFlatten_JoinsTextBlocks(t *testing.T) {
	res := &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent("hello"),
			mcp.NewTextContent("world"),
		},
	}
	got := flattenResult(res)
	if got != "hello\nworld" {
		t.Errorf("got %q", got)
	}
}

func TestFlatten_ImagePlaceholder(t *testing.T) {
	res := &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent("see this:"),
			mcp.NewImageContent("base64data", "image/png"),
		},
	}
	got := flattenResult(res)
	if !strings.Contains(got, "[image: image/png") {
		t.Errorf("missing placeholder: %q", got)
	}
}

func TestFlatten_NilSafe(t *testing.T) {
	if flattenResult(nil) != "" {
		t.Error("nil result should flatten to empty string")
	}
}
