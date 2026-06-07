package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"

	"aiwithtools/internal/agent"
	"aiwithtools/internal/llm"
	"aiwithtools/internal/mcp"
	"aiwithtools/internal/repl"
	"aiwithtools/internal/session"
)

func newRunCmd() *cobra.Command {
	var (
		cont       bool
		resume     bool
		systemFile string
		maxIter    int
		verbose    bool
	)
	cmd := &cobra.Command{
		Use:   "run <model>",
		Short: "Start a chat session with the given model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd.Context(), args[0], cont, resume, systemFile, maxIter, verbose)
		},
	}
	cmd.Flags().BoolVar(&cont, "continue", false, "resume the most recent session for this model")
	cmd.Flags().BoolVar(&resume, "resume", false, "pick a session for this model to resume")
	cmd.Flags().StringVar(&systemFile, "system", "", "override system prompt file (default: ~/.config/aiwithtools/system.md)")
	cmd.Flags().IntVar(&maxIter, "max-iterations", 25, "maximum ReAct iterations per turn")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "print full tool outputs in the REPL")
	cmd.MarkFlagsMutuallyExclusive("continue", "resume")
	return cmd
}

func runRun(ctx context.Context, model string, cont, resume bool, systemFile string, maxIter int, verbose bool) error {
	home, _ := os.UserHomeDir()
	cfgDir := configDir(home, os.Getenv("XDG_CONFIG_HOME"))
	dataD := dataDir(home, os.Getenv("XDG_DATA_HOME"))

	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(dataD, 0700); err != nil {
		return err
	}

	systemPrompt, err := readSystemPrompt(cfgDir, systemFile)
	if err != nil {
		return err
	}

	store, err := session.Open(filepath.Join(dataD, "sessions.db"))
	if err != nil {
		return err
	}
	defer store.Close()

	sess, err := pickSession(store, model, cont, resume)
	if err != nil {
		return err
	}
	if sess == nil {
		sess, err = store.Create(model, systemPrompt)
		if err != nil {
			return err
		}
	}
	return runReplForSession(ctx, sess, maxIter, verbose)
}

// runRootResume implements `aiwithtools --continue` / `aiwithtools --resume`
// at the root command level. The session's model is used for the agent;
// no positional model argument is needed.
func runRootResume(ctx context.Context, cont, resume bool, maxIter int, verbose bool) error {
	home, _ := os.UserHomeDir()
	dataD := dataDir(home, os.Getenv("XDG_DATA_HOME"))
	if err := os.MkdirAll(dataD, 0700); err != nil {
		return err
	}

	store, err := session.Open(filepath.Join(dataD, "sessions.db"))
	if err != nil {
		return err
	}
	defer store.Close()

	sess, err := pickGlobalSession(store, cont, resume)
	if err != nil {
		return err
	}
	return runReplForSession(ctx, sess, maxIter, verbose)
}

// runReplForSession is the shared core: set up MCP host, LLM client,
// agent, and REPL given an already-resolved session.
func runReplForSession(ctx context.Context, sess *session.Session, maxIter int, verbose bool) error {
	home, _ := os.UserHomeDir()
	cfgDir := configDir(home, os.Getenv("XDG_CONFIG_HOME"))
	user := os.Getenv("USER")

	mcfg, err := mcp.LoadConfig(filepath.Join(cfgDir, "mcp.json"), home, user)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if mcfg == nil {
		mcfg = &mcp.Config{}
	}
	host, err := mcp.OpenHost(ctx, mcfg)
	if err != nil {
		return err
	}
	defer host.Close()

	ollamaClient, err := api.ClientFromEnvironment()
	if err != nil {
		return fmt.Errorf("ollama client: %w", err)
	}
	llmClient := llm.New(ollamaClient)

	var tools []llm.Tool
	for _, t := range host.Tools() {
		tools = append(tools, llm.Tool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	ollamaTools, convErrs := llm.ToOllamaTools(tools)
	for _, e := range convErrs {
		slog.Warn("skipping tool with invalid schema", "err", e)
	}

	llmAdapter := &llmAdapter{c: llmClient, sessSystem: sess.System, sessStart: sess.CreatedAt, now: time.Now}
	sessAdapter := &sessionAdapter{s: sess}
	mcpAdapter := &mcpAdapter{h: host, tools: ollamaTools}

	a := &agent.Agent{
		LLM:     llmAdapter,
		MCP:     mcpAdapter,
		Sess:    sessAdapter,
		Display: repl.Terminal{Out: os.Stdout, Verbose: verbose},
		Model:   sess.Model,
		MaxIter: maxIter,
	}

	runner := &repl.Runner{
		Prompt:  ">>> ",
		Out:     os.Stdout,
		OnUser:  func(ctx context.Context, line string) error { return a.Run(ctx, line) },
		OnClear: func() error { return sess.Clear() },
		OnTools: func() string { return formatTools(host.Tools()) },
		OnExit:  func() error { return nil },
	}
	return runner.Run(ctx)
}

func readSystemPrompt(cfgDir, systemFile string) (string, error) {
	if systemFile == "" {
		systemFile = filepath.Join(cfgDir, "system.md")
	}
	b, err := os.ReadFile(systemFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read system prompt: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

func pickSession(store *session.Store, model string, cont, resume bool) (*session.Session, error) {
	if cont {
		id, err := store.LastForModel(model)
		if err != nil {
			return nil, err
		}
		if id == "" {
			return nil, fmt.Errorf("no prior session for model %q", model)
		}
		return store.Load(id)
	}
	if resume {
		items, err := store.List(model)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("no sessions for model %q", model)
		}
		for i, it := range items {
			fmt.Printf("%2d. %s  msgs=%d  last=%s  %q\n",
				i+1, it.ID[:8], it.MessageCount, it.UpdatedAt.Format(time.RFC3339), truncate(it.FirstUserMessage, 60))
		}
		fmt.Print("pick a number: ")
		var n int
		if _, err := fmt.Scanln(&n); err != nil {
			return nil, err
		}
		if n < 1 || n > len(items) {
			return nil, fmt.Errorf("invalid selection")
		}
		return store.Load(items[n-1].ID)
	}
	return nil, nil
}

// pickGlobalSession resolves a session for the root-level --continue /
// --resume flow. Unlike pickSession, it searches across all models.
func pickGlobalSession(store *session.Store, cont, resume bool) (*session.Session, error) {
	items, err := store.List("")
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no sessions to resume")
	}
	if cont {
		it := items[0]
		fmt.Printf("Resuming %s [%s] (%d msgs, last %s)\n",
			it.ID[:8], it.Model, it.MessageCount, it.UpdatedAt.Format("2006-01-02 15:04"))
		return store.Load(it.ID)
	}
	// resume: interactive picker
	for i, it := range items {
		fmt.Printf("%2d. %s  %-30s  msgs=%d  last=%s  %q\n",
			i+1, it.ID[:8], it.Model, it.MessageCount,
			it.UpdatedAt.Format(time.RFC3339), truncate(it.FirstUserMessage, 50))
	}
	fmt.Print("pick a number: ")
	var n int
	if _, err := fmt.Scanln(&n); err != nil {
		return nil, err
	}
	if n < 1 || n > len(items) {
		return nil, fmt.Errorf("invalid selection")
	}
	return store.Load(items[n-1].ID)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func formatTools(tools []mcp.HostTool) string {
	if len(tools) == 0 {
		return "(no MCP tools loaded)"
	}
	var b strings.Builder
	b.WriteString("Available MCP tools:\n")
	for _, t := range tools {
		fmt.Fprintf(&b, "  %s — %s\n", t.Name, t.Description)
	}
	return b.String()
}

// ===== adapters =====

type llmAdapter struct {
	c          *llm.Client
	sessSystem string
	sessStart  time.Time
	now        func() time.Time
}

func (a *llmAdapter) Chat(ctx context.Context, model string, msgs []api.Message, tools api.Tools) (*api.Message, error) {
	built := llm.BuildSystemMessage(a.sessSystem, a.sessStart, a.now())
	withSystem := make([]api.Message, 0, len(msgs)+1)
	withSystem = append(withSystem, api.Message{Role: "system", Content: built})
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		withSystem = append(withSystem, m)
	}
	return a.c.Chat(ctx, model, withSystem, tools)
}

type sessionAdapter struct{ s *session.Session }

func (a *sessionAdapter) AppendUser(c string) error { return a.s.AppendUser(c) }

func (a *sessionAdapter) AppendAssistant(c string, tcs []api.ToolCall) error {
	var out []session.ToolCall
	for _, tc := range tcs {
		out = append(out, session.ToolCall{
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments.ToMap(),
		})
	}
	return a.s.AppendAssistant(c, out)
}

func (a *sessionAdapter) AppendTool(name, c string) error { return a.s.AppendTool(name, c) }

func (a *sessionAdapter) Messages() []api.Message {
	msgs, _ := a.s.Messages()
	out := make([]api.Message, 0, len(msgs))
	for _, m := range msgs {
		am := api.Message{Role: m.Role, Content: m.Content, ToolName: m.ToolName}
		for _, tc := range m.ToolCalls {
			args := api.NewToolCallFunctionArguments()
			for k, v := range tc.Arguments {
				args.Set(k, v)
			}
			am.ToolCalls = append(am.ToolCalls, api.ToolCall{
				Function: api.ToolCallFunction{Name: tc.Name, Arguments: args},
			})
		}
		out = append(out, am)
	}
	return out
}

type mcpAdapter struct {
	h     *mcp.Host
	tools api.Tools
}

func (a *mcpAdapter) Tools() api.Tools { return a.tools }
func (a *mcpAdapter) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	return a.h.Call(ctx, name, args)
}
