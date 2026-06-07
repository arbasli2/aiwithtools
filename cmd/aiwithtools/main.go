package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "aiwithtools",
		Short: "Ollama-backed CLI with MCP tools and ReAct agent loop",
	}
	root.AddCommand(newRunCmd())
	root.AddCommand(newSessionsCmd())

	// SIGTERM only — SIGINT is owned by the REPL so Ctrl-C clears the
	// line at the prompt and only cancels the in-flight turn during
	// OnUser. See internal/repl/repl.go.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
