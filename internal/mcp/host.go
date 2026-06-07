package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

const defaultStartupBudget = 15 * time.Second

type Host struct {
	clients map[string]*client.Client
	tools   []HostTool
	lookup  map[string]hostToolLookup
	mu      sync.Mutex
}

type HostTool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type hostToolLookup struct {
	server  string
	rawName string
}

func OpenHost(ctx context.Context, cfg *Config) (*Host, error) {
	return openHostWithBudget(ctx, cfg, defaultStartupBudget)
}

func openHostWithBudget(ctx context.Context, cfg *Config, budget time.Duration) (*Host, error) {
	h := &Host{
		clients: map[string]*client.Client{},
		lookup:  map[string]hostToolLookup{},
	}
	for _, spec := range cfg.Servers {
		if err := h.connect(ctx, spec, budget); err != nil {
			slog.Warn("mcp server failed", "server", spec.Name, "err", err)
			continue
		}
	}
	return h, nil
}

func (h *Host) connect(parentCtx context.Context, spec ServerSpec, budget time.Duration) error {
	ctx, cancel := context.WithTimeout(parentCtx, budget)
	defer cancel()

	// Always start from the parent's env so PATH/HOME/etc. flow through;
	// mcp-go's stdio transport does not document whether nil/empty env
	// means "inherit" or "empty," so we make it explicit. Config-provided
	// env entries override or add to the parent env.
	env := os.Environ()
	for k, v := range spec.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	c, err := client.NewStdioMCPClient(spec.Command, env, spec.Args...)
	if err != nil {
		return fmt.Errorf("spawn: %w", err)
	}

	initReq := mcpgo.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcpgo.Implementation{Name: "aiwithtools", Version: "0.1"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		c.Close()
		return fmt.Errorf("initialize: %w", err)
	}

	listRes, err := c.ListTools(ctx, mcpgo.ListToolsRequest{})
	if err != nil {
		c.Close()
		return fmt.Errorf("list tools: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[spec.Name] = c
	for _, t := range listRes.Tools {
		prefixed := spec.Name + "__" + t.Name
		schema, _ := json.Marshal(t.InputSchema)
		h.tools = append(h.tools, HostTool{
			Name:        prefixed,
			Description: t.Description,
			InputSchema: schema,
		})
		h.lookup[prefixed] = hostToolLookup{server: spec.Name, rawName: t.Name}
	}
	return nil
}

func (h *Host) Tools() []HostTool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]HostTool, len(h.tools))
	copy(out, h.tools)
	return out
}

func (h *Host) Call(ctx context.Context, prefixed string, args map[string]any) (string, error) {
	h.mu.Lock()
	lkp, ok := h.lookup[prefixed]
	c := h.clients[lkp.server]
	h.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("unknown tool %q", prefixed)
	}

	req := mcpgo.CallToolRequest{}
	req.Params.Name = lkp.rawName
	req.Params.Arguments = args

	res, err := c.CallTool(ctx, req)
	if err != nil {
		return "", fmt.Errorf("call %s: %w", prefixed, err)
	}

	out := flattenResult(res)
	if res.IsError {
		// Return the bare content as the error so callers (the agent)
		// can format it once. The agent already prefixes "ERROR:".
		return "", fmt.Errorf("%s", out)
	}
	return out, nil
}

func (h *Host) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var firstErr error
	for _, c := range h.clients {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
