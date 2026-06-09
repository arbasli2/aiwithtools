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
	var (
		rootCont    bool
		rootResume  bool
		rootMaxIter int
		rootVerbose bool
		rootStream  bool
	)
	root := &cobra.Command{
		Use:   "aiwithtools",
		Short: "Ollama-backed CLI with MCP tools and ReAct agent loop",
		Long: `aiwithtools wraps Ollama with MCP-based tool calling and a ReAct agent loop.

Common forms:
  aiwithtools run <model>           # new session
  aiwithtools --continue            # resume the most recent session (any model)
  aiwithtools --resume              # pick a session to resume (any model)
  aiwithtools sessions              # list sessions`,
		Args:         cobra.NoArgs,
		SilenceUsage: true, // don't print usage on RunE errors
		RunE: func(cmd *cobra.Command, args []string) error {
			if !rootCont && !rootResume {
				return cmd.Help()
			}
			return runRootResume(cmd.Context(), rootCont, rootResume, rootMaxIter, rootVerbose, rootStream)
		},
	}
	root.Flags().BoolVar(&rootCont, "continue", false, "resume the most recent session (any model)")
	root.Flags().BoolVar(&rootResume, "resume", false, "pick a session to resume (any model)")
	root.Flags().IntVar(&rootMaxIter, "max-iterations", 25, "maximum ReAct iterations per turn")
	root.Flags().BoolVar(&rootVerbose, "verbose", false, "print full tool outputs in the REPL")
	root.Flags().BoolVar(&rootStream, "stream", true, "stream assistant responses as they arrive (--stream=false for atomic output)")
	root.MarkFlagsMutuallyExclusive("continue", "resume")

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
