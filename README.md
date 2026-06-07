# aiwithtools

A drop-in-feel Ollama wrapper with MCP-based tool calling and a ReAct agent loop.

## Install

```bash
go install ./cmd/aiwithtools
```

Or build locally:

```bash
go build -o aiwithtools ./cmd/aiwithtools
```

## Configure

Create `~/.config/aiwithtools/mcp.json` — same format as Claude Code:

```json
{
  "mcpServers": {
    "text-saver": {
      "command": "uv",
      "args": ["run", "~/src/mcp-servers/text-saver.py"]
    }
  }
}
```

Optional: `~/.config/aiwithtools/system.md` with your default system prompt.

## Use

```bash
# new session
aiwithtools run nemotron-3-nano:30b-cloud

# resume the most recent session for this model
aiwithtools run nemotron-3-nano:30b-cloud --continue

# pick a session interactively
aiwithtools run nemotron-3-nano:30b-cloud --resume

# list / delete sessions
aiwithtools sessions
aiwithtools sessions --model nemotron-3-nano:30b-cloud
aiwithtools sessions rm <id-prefix>
aiwithtools sessions rm --all
```

In the REPL:

- `/clear` — drop messages from this session (session row stays)
- `/tools` — list connected MCP servers and their tools
- `/exit` or `/bye` — quit
- `/help` — list commands

## Smoke test

Default smoke-test model is `nemotron-3-nano:30b-cloud` — `:cloud` and free. You must `ollama signin` first to use cloud models.

```bash
aiwithtools run nemotron-3-nano:30b-cloud
>>> /tools
>>> save the text 'hello' using the text-saver tool, then tell me what you did
```

Expected behavior: the model calls `text-saver__<name>` (where `<name>` is whatever your text-saver server exposes — confirm with `/tools` first; you'll see the `→` line in the REPL), the result comes back (`←` line), then a final summary. This exercises the full ReAct loop end-to-end.

## Architecture

See `docs/superpowers/specs/2026-06-07-aiwithtools-cli-design.md`.
