package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer("fakeserver", "0.0.1")

	s.AddTool(mcp.NewTool("echo",
		mcp.WithDescription("Echo back the text argument"),
		mcp.WithString("text", mcp.Required(), mcp.Description("text to echo")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text := req.GetString("text", "")
		return mcp.NewToolResultText(fmt.Sprintf("echo: %s", text)), nil
	})

	if err := server.ServeStdio(s); err != nil {
		panic(err)
	}
}
