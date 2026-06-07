# aiwithtools — Ollama-backed CLI with MCP tools and ReAct agent loop

**Status:** design approved, ready for implementation plan
**Date:** 2026-06-07

## Summary

`aiwithtools` is a Go CLI that wraps Ollama with MCP-based tool calling and a ReAct agent loop. Invocation mirrors Ollama: `aiwithtools run <model>` opens a REPL backed by the local Ollama daemon (including `:cloud` models). MCP servers configured in `~/.config/aiwithtools/mcp.json` are spawned at startup; their tools are exposed to the model. When the model emits `tool_calls`, the agent routes them to the appropriate MCP server, returns results to the model, and continues until the model produces a final answer or hits an iteration cap. Sessions persist to SQLite so users can resume with `--continue` or pick with `--resume`.

## Goals

- Drop-in feel for users familiar with `ollama run`.
- Multi-step (ReAct) tool use across one or more MCP servers in a single user turn.
- Persistent sessions, resumable across process restarts.
- Stay close to Ollama's native tool format; no unnecessary abstraction layer.

## Non-goals (v1)

- Streaming token output. Non-streaming `/api/chat` returns the whole message + `tool_calls` cleanly; streaming + tool calls is added later if needed.
- Image inputs, tool-result images surfaced to the user.
- Tool whitelisting / per-tool enable flags.
- Model swapping mid-session, editing past messages.
- Multiple LLM providers (OpenAI, Anthropic). Ollama only.
- Token / cost tracking.

## CLI surface

```
aiwithtools run <model>                  # new session
aiwithtools run <model> --continue       # resume most recent session for that model
aiwithtools run <model> --resume         # interactive picker among that model's sessions
aiwithtools run <model> --system FILE    # override system prompt for this session
aiwithtools run <model> --max-iterations N   # default 25

aiwithtools sessions                     # list all sessions (id, model, last-used, msg count, first user msg preview)
aiwithtools sessions --model <model>     # filter list to one model
aiwithtools sessions rm <id>             # delete one
aiwithtools sessions rm --all            # delete all
```

`<model>` is whatever the local Ollama daemon accepts, including `:cloud` suffixed cloud models (e.g. `nemotron-3-nano:30b-cloud`, used as the default smoke-test model because it's free).

### Slash commands in the REPL

- `/clear` — drop all messages from the current session (session row stays, so `--continue` still finds it but with empty history)
- `/exit`, `/bye` — quit
- `/tools` — print each connected MCP server and its tools
- `/help` — list slash commands

## Architecture

```
cmd/aiwithtools  (cobra: run, sessions)
        │
   internal/repl     (readline loop, slash-command dispatch)
        │
   internal/agent    (ReAct loop, Display interface)
        │     │
   internal/llm    internal/mcp
   (Ollama HTTP)  (spawn servers, tool registry)
        │             │
        └──── internal/session (SQLite store)
```

Package boundaries:

- **`internal/llm`** wraps `github.com/ollama/ollama/api`. Exposes `Chat(ctx, model, msgs, tools) (*api.Message, error)`. Owns date-suffix injection into the system message at request time.
- **`internal/mcp`** is the MCP host. Reads config, spawns servers via `github.com/mark3labs/mcp-go/client` (stdio transport), aggregates tools into a prefixed registry. Exposes `Tools() []api.Tool` and `Call(ctx, prefixedName, args) (string, error)`.
- **`internal/agent`** owns the ReAct loop. No direct I/O — takes an injected `Display` interface.
- **`internal/repl`** owns terminal I/O via `github.com/chzyer/readline` and slash-command dispatch. Implements `Display`.
- **`internal/session`** is the only package that talks to SQLite. Uses `modernc.org/sqlite` (pure Go, no CGO).

## ReAct loop

```go
// Every Append* call inside this function is a single transaction that
// inserts the message AND bumps sessions.updated_at = now. The session
// package owns that invariant; the agent just calls Append*.
func (a *Agent) Run(ctx context.Context, userInput string) error {
    a.sess.AppendUser(userInput)
    for i := 0; i < a.maxIter; i++ {
        resp, err := a.llm.Chat(ctx, a.model, a.sess.Messages(), a.mcp.Tools())
        if err != nil { return err }
        a.sess.AppendAssistant(resp)

        if len(resp.ToolCalls) == 0 {
            a.display.AssistantFinal(resp.Content)
            return nil
        }
        for _, tc := range resp.ToolCalls {
            a.display.ToolCallStart(tc.Name, tc.Arguments)
            out, err := a.mcp.Call(ctx, tc.Name, tc.Arguments)
            content := out
            if err != nil {
                content = fmt.Sprintf("ERROR: %s", err) // serialize for model recovery
            }
            a.display.ToolCallEnd(tc.Name, content, err)
            a.sess.AppendTool(tc.Name, content)
        }
    }
    return fmt.Errorf("max iterations (%d) reached", a.maxIter)
}
```

Key rules:

- **Tool errors do not abort the loop.** They're serialized into a `tool`-role message so the model can adapt.
- **Context cancellation** (Ctrl-C) cancels the in-flight LLM/MCP call. Anything already persisted stays; any dangling tool_calls without responses are detected on next resume and dropped with a warning.
- **One INSERT per message**, no multi-message transactions. Everything in the DB is real.
- **Display** is an interface; the REPL prints `→ server__tool(args)` and `← server__tool (240 chars)` summaries inline. Full tool output goes to model but is truncated in the UI unless `--verbose`.
- **MCP `[]Content` flattening**: text blocks joined with newlines; image/resource blocks become `"[image: <mime>, <bytes> omitted]"` text placeholders. The model gets text; users see text.

## MCP host

Config path: `~/.config/aiwithtools/mcp.json`, same schema as Claude Code / Claude Desktop (`mcpServers` object, each value has `command`, `args`, `env`).

Lifecycle:

1. Read and parse config at startup.
2. For each server, spawn via mcp-go stdio transport with the configured `command`/`args`/`env`. `command` and any path-shaped `args` may contain `~` and `$HOME`/`$USER` references; the host expands these before exec'ing (other env-var expansion is not performed — keeps behavior predictable). Relative paths in `command` resolve against `$PATH` per the normal exec rules. Per-server failures log a warning and continue (other servers still load).
3. Complete the MCP `initialize` handshake on each spawned server, then call `tools/list`. Build the tool registry with `server__rawname` keys. Cache `inputSchema` as `json.RawMessage` and pass through unchanged when building the Ollama `tools` array (both are JSON Schema).
4. Each server has a **15-second startup budget** covering spawn + initialize + tools/list. If a server doesn't complete the handshake in that window, treat it as a spawn failure (log warning, skip). This prevents a broken server from blocking the REPL forever.
5. `Tools()` returns an `[]api.Tool` snapshot.
6. `Call(ctx, prefixedName, args)` routes to the right client's `CallTool`, flattens the response content blocks to a single string.
7. Shutdown: REPL exit or signal triggers `Close()` on every client (terminates child processes). Wired in `main`'s defer.

Name collisions across servers are resolved by the `server__` prefix. No raw tool names are exposed.

## Session storage

SQLite at `~/.local/share/aiwithtools/sessions.db`. Directory created mode 0700, DB file created mode 0600 (chat content may be sensitive — keep it user-only).

```sql
CREATE TABLE sessions (
  id          TEXT PRIMARY KEY,           -- ULID, sortable by creation time
  model       TEXT NOT NULL,
  system      TEXT NOT NULL DEFAULT '',   -- user's system prompt only, frozen at creation
  created_at  INTEGER NOT NULL,           -- unix seconds
  updated_at  INTEGER NOT NULL
);
CREATE INDEX idx_sessions_model_updated ON sessions(model, updated_at DESC);

CREATE TABLE messages (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  seq         INTEGER NOT NULL,
  role        TEXT NOT NULL,              -- user | assistant | tool | system
  content     TEXT NOT NULL,
  tool_calls  TEXT,                       -- JSON, NULL if none
  tool_name   TEXT,                       -- non-NULL only for role=tool
  created_at  INTEGER NOT NULL,
  UNIQUE(session_id, seq)
);
CREATE INDEX idx_messages_session_seq ON messages(session_id, seq);

CREATE TABLE schema_version (version INTEGER PRIMARY KEY);
```

Behaviors:

- `--continue`: `SELECT id FROM sessions WHERE model = ? ORDER BY updated_at DESC LIMIT 1`. Per-model most-recent.
- `--resume`: list this model's sessions ordered by `updated_at DESC` showing id, last-used, msg count, and first-user-message preview; prompt for selection by number.
- `/clear`: `DELETE FROM messages WHERE session_id = ?`. Session row stays so `--continue` still finds it.
- `sessions rm <id>`: cascade delete via FK.
- Dangling tool_calls on resume: if the last assistant message in the session has `tool_calls` and fewer following `tool`-role messages than there are calls (zero or partial), print a warning and discard the assistant message AND any partial tool responses. The session resumes as if the prior turn had just received its user message and never been answered, so the next request re-runs the LLM cleanly with no half-answered tool calls in context.

## System prompt and date injection

User's system prompt lives in `~/.config/aiwithtools/system.md` (default empty), or `--system FILE` overrides per session. The prompt text alone is stored in `sessions.system` at session creation. It is frozen for the lifetime of the session — changing `system.md` later does not affect resumed sessions.

At every request to Ollama, the system message sent over the wire is built fresh:

```
<stored system prompt>

Tools are available via function calling. Today is YYYY-MM-DD.
```

If the session's first message and now span ≥24 hours, the date line becomes:

```
Tools are available via function calling. Today is YYYY-MM-DD. This conversation includes earlier messages from prior days.
```

The date suffix is never stored in the messages table — it's request-time only. This makes resume-across-days behave correctly: previous turns retain their original meaning, and the model always sees the correct current date.

Documented limitation: a single same-day session that crosses midnight mid-turn won't get the multi-day note. The model handles this gracefully in practice, and the `time-space` MCP server (already in the user's config) lets the model ask for the current time when it matters.

## Error handling

| Failure | Behavior |
|---|---|
| Ollama daemon unreachable | Fatal at startup. Print actionable error referencing `ollama serve`. |
| Model not pulled / not found (local) | Surface Ollama error verbatim. No auto-pull. |
| Cloud model auth missing/expired (`:cloud` suffix → 401 from daemon) | Fatal at startup. Print `aiwithtools: cloud model "<name>" requires authentication — run \`ollama signin\` and retry.` We don't manage cloud credentials; the daemon does. |
| MCP server fails to spawn | Warn, continue. Other servers still load. |
| MCP tool call fails | Serialize error as `tool`-role message content. Loop continues. |
| Ctrl-C mid-turn | Cancel in-flight call. Persist what completed. Drop dangling tool_calls on next resume. Return to prompt. |
| `maxIter` hit | Error message, return to prompt. Session intact. |
| SQLite write fails | Fatal. Cannot continue safely. |
| Invalid config | Fatal at startup with parser error. |

Principle: boundary errors are fatal; in-loop errors are recoverable via the model.

## Dependencies

- `github.com/ollama/ollama/api` — official Ollama Go client
- `github.com/mark3labs/mcp-go` — MCP client (stdio transport)
- `github.com/spf13/cobra` — CLI parsing
- `github.com/chzyer/readline` — REPL line editing
- `modernc.org/sqlite` — pure-Go SQLite (no CGO)
- `github.com/oklog/ulid/v2` — session IDs

## Testing strategy

**Unit (table-driven, no I/O):**

- `internal/llm`: tool-format translation (internal ↔ `api.Tool`), date-suffix builder including the multi-day branch.
- `internal/mcp`: prefix logic, content-block flattening, registry lookup. Fake mcp-go client behind an interface.
- `internal/agent`: ReAct loop with mock LLM/MCP/Display. Cases: no tool calls → return; tool error → loop continues with error in content; max iterations → returns error; context cancel → returns error without further calls.
- `internal/session`: schema migration, `/clear` keeps row but drops messages, `--continue` query is per-model latest, dangling tool_calls detected on load.

**Integration:**

- A minimal MCP server written in Go and checked into `internal/mcp/testdata/fakeserver/` is built at test time and spawned over stdio. The test exercises `initialize` → `tools/list` → `CallTool` end-to-end. No dependency on Python, `uv`, or files outside the repo — runs cleanly on CI and any contributor's machine.
- Open temp SQLite, write a session, run `--continue` lookup, verify message replay. Also covers the dangling-tool-calls recovery path by inserting a session with an assistant message whose `tool_calls` are partially answered, then asserting the loader drops the right messages.

**Manual smoke (documented in README, not in CI):**

`aiwithtools run nemotron-3-nano:30b-cloud` (the default smoke-test model — `:cloud` and free; requires `ollama signin` first). Ask: "save the text 'hello' using the text-saver tool, then tell me what you did." Assumes the user's MCP config includes `text-saver` (see `etc/example/mcp.json`). Verify the model calls `text-saver__save_text`, sees the result, and produces a final answer. This is the v1 acceptance test for the full ReAct loop against a real model.

CI does not run Ollama; the manual smoke is the only model-in-the-loop check.

## Open questions

None blocking implementation. Future considerations (out of v1 scope):

- Streaming output once tool-call streaming UX is figured out.
- Tool whitelisting once users add many servers.
- A `--provider` flag if there's ever demand for non-Ollama backends. The `agent`, `mcp`, `session`, and `repl` packages don't need to change; only `llm` would grow an interface.
