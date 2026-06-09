package main

import (
	"context"
	"encoding/json"
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
	"aiwithtools/internal/skills"
)

func newRunCmd() *cobra.Command {
	var (
		cont       bool
		resume     bool
		systemFile string
		maxIter    int
		verbose    bool
		stream     bool
	)
	cmd := &cobra.Command{
		Use:   "run <model>",
		Short: "Start a chat session with the given model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd.Context(), args[0], cont, resume, systemFile, maxIter, verbose, stream)
		},
	}
	cmd.Flags().BoolVar(&cont, "continue", false, "resume the most recent session for this model")
	cmd.Flags().BoolVar(&resume, "resume", false, "pick a session for this model to resume")
	cmd.Flags().StringVar(&systemFile, "system", "", "override system prompt file (default: ~/.config/aiwithtools/system.md)")
	cmd.Flags().IntVar(&maxIter, "max-iterations", 25, "maximum ReAct iterations per turn")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "print full tool outputs in the REPL")
	cmd.Flags().BoolVar(&stream, "stream", true, "stream assistant responses as they arrive (--stream=false for atomic output)")
	cmd.MarkFlagsMutuallyExclusive("continue", "resume")
	return cmd
}

func runRun(ctx context.Context, model string, cont, resume bool, systemFile string, maxIter int, verbose, stream bool) error {
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
	return runReplForSession(ctx, sess, maxIter, verbose, stream)
}

// runRootResume implements `aiwithtools --continue` / `aiwithtools --resume`
// at the root command level. The session's model is used for the agent;
// no positional model argument is needed.
func runRootResume(ctx context.Context, cont, resume bool, maxIter int, verbose, stream bool) error {
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
	return runReplForSession(ctx, sess, maxIter, verbose, stream)
}

// runReplForSession is the shared core: set up MCP host, LLM client,
// agent, and REPL given an already-resolved session.
func runReplForSession(ctx context.Context, sess *session.Session, maxIter int, verbose, stream bool) error {
	home, _ := os.UserHomeDir()
	cfgDir := configDir(home, os.Getenv("XDG_CONFIG_HOME"))
	user := os.Getenv("USER")

	mcfg, err := mcp.LoadConfig(filepath.Join(cfgDir, "mcp.json"), home, user)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("mcp config: %w", err)
	}
	if mcfg == nil {
		mcfg = &mcp.Config{}
	}
	host, err := mcp.OpenHost(ctx, mcfg)
	if err != nil {
		return fmt.Errorf("mcp host: %w", err)
	}
	defer host.Close()

	ollamaClient, err := api.ClientFromEnvironment()
	if err != nil {
		return fmt.Errorf("ollama client: %w", err)
	}
	llmClient := llm.New(ollamaClient)

	// Load skills from ~/.config/aiwithtools/skills/. Missing dir is fine
	// (returns an empty manager). Any other error degrades to empty manager
	// + a warning so the REPL still starts.
	skillMgr, err := skills.Load(filepath.Join(cfgDir, "skills"))
	if err != nil {
		slog.Warn("could not load skills", "err", err)
		skillMgr, _ = skills.Load(filepath.Join(cfgDir, "skills-missing-marker-noexist"))
	}

	var tools []llm.Tool
	for _, t := range host.Tools() {
		tools = append(tools, llm.Tool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	// Add the synthetic load_skill tool when any skills are configured.
	if skillTool, ok := buildLoadSkillTool(skillMgr); ok {
		tools = append(tools, skillTool)
	}
	ollamaTools, convErrs := llm.ToOllamaTools(tools)
	for _, e := range convErrs {
		slog.Warn("skipping tool with invalid schema", "err", e)
	}

	// Best-effort context-window probe. Cloud models may not return
	// model_info; we treat ctxLen=0 as "unknown" and just don't surface
	// the warning or "/info" context line in that case.
	ctxLen, ctxErr := llmClient.ContextLength(ctx, sess.Model)
	if ctxErr != nil {
		slog.Warn("could not determine context length", "model", sess.Model, "err", ctxErr)
	}

	llmAdapter := &llmAdapter{c: llmClient, sessSystem: sess.System, sessStart: sess.CreatedAt, now: time.Now}
	sessAdapter := &sessionAdapter{s: sess}
	mcpAdapter := &mcpAdapter{h: host, tools: ollamaTools, skills: skillMgr}

	a := &agent.Agent{
		LLM:           llmAdapter,
		MCP:           mcpAdapter,
		Sess:          sessAdapter,
		Display:       repl.Terminal{Out: os.Stdout, Verbose: verbose},
		Model:         sess.Model,
		MaxIter:       maxIter,
		ContextLength: ctxLen,
		Stream:        stream,
	}

	// Startup banner: model + context window, before the first prompt.
	if ctxLen > 0 {
		fmt.Printf("%s %s\n", repl.Bold("Model:"), repl.Cyan(fmt.Sprintf("%s (%s ctx)", sess.Model, llm.FormatTokens(ctxLen))))
	} else {
		fmt.Printf("%s %s\n", repl.Bold("Model:"), repl.Cyan(fmt.Sprintf("%s (context size unknown)", sess.Model)))
	}
	fmt.Println(repl.Dim("Type /help for commands."))

	runner := &repl.Runner{
		Prompt:   repl.Cyan(">>> "),
		Out:      os.Stdout,
		OnUser:   func(ctx context.Context, line string) error { return a.Run(ctx, line) },
		OnClear:  func() error { return sess.Clear() },
		OnTools:  func() string { return formatTools(host.Tools()) },
		OnInfo:   func() string { return formatInfo(sess, a, ctxLen) },
		OnSkills: func() string { return formatSkills(skillMgr) },
		OnSkill:  func(line string) (string, bool, error) { return resolveSkillCommand(skillMgr, line) },
		OnExit:   func() error { return nil },
	}
	return runner.Run(ctx)
}

// buildLoadSkillTool returns a synthetic llm.Tool that the model can
// call to load any of the discovered skills. The tool description
// embeds the list of skills so the model knows which names are valid
// and what each one does — matching the "progressive disclosure"
// pattern from the Anthropic Agent Skills standard.
//
// Returns ok=false when no skills are configured.
func buildLoadSkillTool(mgr *skills.Manager) (llm.Tool, bool) {
	all := mgr.List()
	if len(all) == 0 {
		return llm.Tool{}, false
	}

	var desc strings.Builder
	desc.WriteString("Load detailed instructions for a specialised task. ")
	desc.WriteString("Use this when the user's request matches one of these skills:\n")
	for _, s := range all {
		fmt.Fprintf(&desc, "- %s: %s\n", s.Name, s.Description)
	}
	desc.WriteString("Call with the skill's name; the response is the full instruction set to follow.")

	// Build a JSON Schema with `name` as a required enum of skill names.
	enum := make([]string, 0, len(all))
	for _, s := range all {
		enum = append(enum, s.Name)
	}
	enumJSON, _ := json.Marshal(enum)
	schema := fmt.Sprintf(
		`{"type":"object","properties":{"name":{"type":"string","enum":%s,"description":"Skill to load"}},"required":["name"]}`,
		string(enumJSON),
	)
	return llm.Tool{
		Name:        "load_skill",
		Description: desc.String(),
		InputSchema: json.RawMessage(schema),
	}, true
}

// resolveSkillCommand handles a slash input that wasn't a built-in
// (e.g. "/translate French"). Returns (rendered body, true, nil) if
// the first token is a known skill, (...) ok=false otherwise so the
// REPL can fall back to treating the line as ordinary text.
func resolveSkillCommand(mgr *skills.Manager, line string) (string, bool, error) {
	if !strings.HasPrefix(line, "/") {
		return "", false, nil
	}
	rest := strings.TrimPrefix(line, "/")
	parts := strings.SplitN(rest, " ", 2)
	name := parts[0]
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	if mgr.Get(name) == nil {
		return "", false, nil
	}
	body, err := mgr.Render(name, args)
	if err != nil {
		return "", true, err
	}
	return body, true, nil
}

func formatSkills(mgr *skills.Manager) string {
	all := mgr.List()
	if len(all) == 0 {
		return "(no skills loaded — drop a SKILL.md folder under ~/.config/aiwithtools/skills/)"
	}
	var b strings.Builder
	b.WriteString("Available skills:\n")
	for _, s := range all {
		desc := s.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Fprintf(&b, "  /%s — %s\n", s.Name, desc)
	}
	b.WriteString("Invoke with `/<name> [args]`; the model may also call load_skill.")
	return b.String()
}

// formatInfo renders the /info report.
func formatInfo(sess *session.Session, a *agent.Agent, ctxLen int) string {
	msgs, _ := sess.Messages()
	var b strings.Builder
	fmt.Fprintf(&b, "Session:  %s\n", sess.ID)
	fmt.Fprintf(&b, "Model:    %s\n", sess.Model)
	if ctxLen > 0 {
		used := a.LastPromptTokens + a.LastEvalTokens
		if used > 0 {
			pct := used * 100 / ctxLen
			fmt.Fprintf(&b, "Context:  %s / %s tokens (%d%%)\n",
				llm.FormatTokens(used), llm.FormatTokens(ctxLen), pct)
		} else {
			fmt.Fprintf(&b, "Context:  %s tokens max (no turn yet — usage unknown)\n", llm.FormatTokens(ctxLen))
		}
	} else {
		fmt.Fprintln(&b, "Context:  size unknown (cloud model or older Ollama)")
	}
	fmt.Fprintf(&b, "Messages: %d\n", len(msgs))
	if a.LastPromptTokens > 0 || a.LastEvalTokens > 0 {
		fmt.Fprintf(&b, "Last turn: %d prompt + %d reply tokens",
			a.LastPromptTokens, a.LastEvalTokens)
	} else {
		fmt.Fprint(&b, "Last turn: (none yet)")
	}
	return b.String()
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
		sess, err := store.Load(it.ID)
		if err != nil {
			return nil, err
		}
		fmt.Println(repl.Green(fmt.Sprintf("Resuming %s [%s] (%d msgs, last %s)",
			it.ID[:8], it.Model, it.MessageCount, it.UpdatedAt.Format("2006-01-02 15:04"))))
		return sess, nil
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

func (a *llmAdapter) Chat(ctx context.Context, model string, msgs []api.Message, tools api.Tools, onChunk llm.ChunkFunc) (*llm.ChatResult, error) {
	built := llm.BuildSystemMessage(a.sessSystem, a.sessStart, a.now())
	withSystem := make([]api.Message, 0, len(msgs)+1)
	withSystem = append(withSystem, api.Message{Role: "system", Content: built})
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		withSystem = append(withSystem, m)
	}
	return a.c.Chat(ctx, model, withSystem, tools, onChunk)
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
	h      *mcp.Host
	tools  api.Tools
	skills *skills.Manager
}

func (a *mcpAdapter) Tools() api.Tools { return a.tools }

func (a *mcpAdapter) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	// load_skill is a synthetic, app-side tool that reads SKILL.md
	// rather than going out to MCP. It exists only when at least one
	// skill is configured (see buildLoadSkillTool).
	if name == "load_skill" {
		skillName, _ := args["name"].(string)
		if skillName == "" {
			return "", fmt.Errorf("load_skill requires a 'name' argument")
		}
		body, err := a.skills.Render(skillName, "")
		if err != nil {
			return "", err
		}
		return body, nil
	}
	return a.h.Call(ctx, name, args)
}
