# aiwithtools

A drop-in-feel Ollama wrapper with MCP-based tool calling and a ReAct agent loop.

> Full **[User Manual](docs/USER_MANUAL.md)** covers every flag, slash
> command, colour, environment variable, file location, and the
> troubleshooting guide.

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

# resume the most recent session across all models
aiwithtools --continue

# pick a session across all models
aiwithtools --resume

# resume the most recent session for ONE specific model
aiwithtools run nemotron-3-nano:30b-cloud --continue

# pick a session for ONE specific model
aiwithtools run nemotron-3-nano:30b-cloud --resume

# list / delete sessions
aiwithtools sessions
aiwithtools sessions --model nemotron-3-nano:30b-cloud
aiwithtools sessions rm <id-prefix>
aiwithtools sessions rm --all
```

In the REPL:

- `/info` — show model, context window, session id, message count, last-turn tokens
- `/tools` — list connected MCP servers and their tools
- `/clear` — drop messages from this session (session row stays)
- `/exit` or `/bye` — quit
- `/help` — list commands

When a turn ends with context usage above 80%, a notice is shown like
`(context 86%: 28K / 32K — Ollama will start dropping oldest messages above 100%)`.
Ollama auto-truncates oldest messages once the window overflows
(`truncate=true` by default) so the conversation keeps working — but
older context is silently dropped.

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
