# aiwithtools — User Manual

`aiwithtools` is a CLI that wraps the local Ollama daemon, adds MCP-based
tool calling, and runs a multi-step ReAct agent loop. It works with any
model Ollama can run, including the free `:cloud` models.

This manual covers installation, configuration, daily use, all REPL
commands, the colour scheme, environment variables, and troubleshooting.

---

## Contents

1. [Installation](#installation)
2. [First-time setup](#first-time-setup)
3. [Running the CLI](#running-the-cli)
4. [Sessions](#sessions)
5. [Slash commands inside the REPL](#slash-commands-inside-the-repl)
6. [MCP servers](#mcp-servers)
7. [System prompt](#system-prompt)
8. [Context window management](#context-window-management)
9. [Colour output](#colour-output)
10. [Environment variables](#environment-variables)
11. [File locations](#file-locations)
12. [Building from source](#building-from-source)
13. [Troubleshooting](#troubleshooting)

---

## Installation

### From source

You need Go 1.26 or newer and a working Ollama installation
([ollama.com/download](https://ollama.com/download)). Older Go
toolchains may auto-upgrade; if `go build` complains about a Go
version requirement, install a newer Go from
[go.dev/dl](https://go.dev/dl/).

```bash
git clone https://github.com/arbasli2/aiwithtools.git
cd aiwithtools
go install ./cmd/aiwithtools
```

`go install` puts the binary in `$GOPATH/bin` (defaults to `~/go/bin`).
Make sure that directory is on your `PATH`.

To build locally without installing:

```bash
go build -o aiwithtools ./cmd/aiwithtools
./aiwithtools --help
```

---

## First-time setup

Three things have to be true before you can use a model:

1. **The Ollama daemon must be running.**
   ```bash
   ollama serve         # foreground
   # or
   brew services start ollama   # macOS background service
   ```
2. **For cloud models** (`<name>:cloud`), you must be signed in.
   ```bash
   ollama signin
   ```
3. **For tool calling, MCP servers must be configured.** See
   [MCP servers](#mcp-servers).

Without (3) the agent runs fine but only has the model itself — no tools.

---

## Running the CLI

```bash
# Start a new session with a model
aiwithtools run nemotron-3-nano:30b-cloud

# Resume the most recent session for ANY model (recommended shortcut)
aiwithtools --continue

# Pick a session to resume across all models
aiwithtools --resume

# Resume the most recent session for ONE specific model
aiwithtools run nemotron-3-nano:30b-cloud --continue

# Interactive picker filtered to one model
aiwithtools run nemotron-3-nano:30b-cloud --resume

# Override the system prompt file for this session
aiwithtools run <model> --system /path/to/prompt.md

# Cap the agent's tool-loop iterations (default 25)
aiwithtools run <model> --max-iterations 10

# Print full tool outputs in the REPL (default: only summary)
aiwithtools run <model> --verbose

# Streaming is on by default; disable for atomic output
aiwithtools run <model> --stream=false
```

The `--stream`, `--max-iterations`, `--verbose`, and `--system` flags
also work at the root level alongside `--continue` / `--resume`:

```bash
aiwithtools --continue --stream=false
```

Exit the REPL with `/exit`, `/bye`, or `Ctrl-D` at an empty prompt.
`Ctrl-C` at the prompt clears the current line and re-prompts;
`Ctrl-C` during a running turn cancels just that turn (it does not
kill the REPL).

---

## Sessions

Every conversation is a session, stored in SQLite at
`~/.local/share/aiwithtools/sessions.db`. Sessions are durable: closing
the REPL doesn't lose them.

```bash
aiwithtools sessions                      # list all sessions
aiwithtools sessions --model <name>       # filter to one model
aiwithtools sessions rm <id-prefix>       # delete one (8 chars is enough)
aiwithtools sessions rm --all             # delete every session
```

Session IDs are 26-character ULIDs but the list shows only the first 8
characters. Use that prefix with `sessions rm`.

A session's **system prompt is frozen at creation time** — editing
`system.md` later does not change resumed sessions, only new ones.
The current date is injected fresh on every request, so resuming a
multi-day-old session does not confuse the model about what "today" is.

---

## Slash commands inside the REPL

| Command           | Effect                                                           |
|-------------------|------------------------------------------------------------------|
| `/info`           | Show model, context size, used tokens (count + %), session id, message count, last-turn tokens. |
| `/tools`          | List every connected MCP server and the tools it exposes. Run this if a tool isn't being called — it's the fastest way to check the model has access. |
| `/skills`         | List discovered skills with their descriptions.                  |
| `/<name> [args]`  | Invoke a skill by name (e.g. `/translate French`). Args after the name are substituted into `{{ARGUMENTS}}`. |
| `/clear`          | Drop all messages from the current session. The session row stays, so `--continue` still finds it (with an empty history). |
| `/exit` or `/bye` | Quit the REPL.                                                   |
| `/help`           | Show the list of slash commands.                                 |

---

## MCP servers

MCP servers expose tools the model can call. Configure them in
`~/.config/aiwithtools/mcp.json` — the format matches Claude Code and
Claude Desktop.

```json
{
  "mcpServers": {
    "text-saver": {
      "command": "uv",
      "args": ["run", "~/src/mcp-servers/text-saver.py"]
    },
    "tavily": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "https://mcp.tavily.com/mcp/?tavilyApiKey=YOUR_KEY_HERE"]
    }
  }
}
```

**Path expansion**: `~`, `$HOME`, and `$USER` are expanded only inside
MCP server `command` and `args` (this CLI handles them itself so the
JSON file is portable across machines). Other environment variables in
`command`/`args` are **not** expanded — write them literally, hard-code
the path, or move them into the `env` map. Flags like `--system <path>`
pass through your shell's normal expansion only — if you write
`--system $HOME/foo.md` your shell expands it, the CLI does not.

**Name collisions**: tools are exposed to the model as
`<server>__<toolname>` (e.g. `text-saver__save_text`). Two servers can
both expose a `search` tool with no conflict.

**Server start-up budget**: each MCP server has 15 seconds to
initialise. A server that hangs is dropped with a warning and the rest
of the REPL keeps working.

**Per-server env**: anything you put in the optional `env` map is
merged on top of the parent process's environment (so `PATH`, `HOME`,
etc. always flow through).

---

## Skills

Skills are reusable instruction packages — a folder per skill — that
implement the
[Anthropic Agent Skills standard](https://claude.com/blog/skills).
Each skill is a directory under `~/.config/aiwithtools/skills/`
containing a `SKILL.md` file plus optional supporting files.

### Layout

```
~/.config/aiwithtools/skills/
└── translate/
    ├── SKILL.md
    └── references/         (optional)
        └── conventions.md
```

`SKILL.md` has YAML frontmatter (optional `name` defaults to the
directory name, optional `description` is shown to the model and in
`/skills`) followed by a Markdown body:

```markdown
---
name: translate
description: Translate text to a target language, preserving tone
---
Translate the following to {{ARGUMENTS}}, keeping formatting intact.
Refer to the conventions:

{{include: references/conventions.md}}
```

### Two ways to invoke

1. **Explicitly, by the user.** Type `/<name> [args]` in the REPL. The
   skill body is rendered (with `{{ARGUMENTS}}` replaced and any
   `{{include: …}}` expanded) and sent as your next message.
2. **By the model.** Skill names + descriptions are exposed to the
   model via a synthetic `load_skill` tool. The model may decide
   "this task matches the `review` skill" and call `load_skill(name=
   "review")`. The body is returned as the tool result and the model
   follows the instructions. This implements the standard's
   *progressive disclosure*: short descriptions are always visible,
   full bodies load only when needed.

### Template syntax

| Placeholder              | Meaning                                            |
|--------------------------|----------------------------------------------------|
| `{{ARGUMENTS}}`          | Args after `/<name> ` (empty for model invocation) |
| `{{include: <relpath>}}` | Contents of a file under the skill folder          |

Includes are sandboxed to the skill folder — leading `/` or `..` in
the path is rejected. Includes nest (an included file may itself use
`{{include: …}}`) up to a depth of 8.

### Practical notes

- A directory without a `SKILL.md` is silently ignored.
- A missing `~/.config/aiwithtools/skills/` directory is also fine —
  you just have no skills.
- Skills with no description are still loaded but get a
  `(no description)` placeholder in `/skills`.
- The model's ability to choose the right skill depends on the
  model. Larger models pick well; small models may need explicit
  user invocation (`/<name>`).

## System prompt

Write a default system prompt to `~/.config/aiwithtools/system.md`:

```markdown
You are a careful Go engineer working in the aiwithtools project.
Prefer pure stdlib solutions. Quote file paths as `path:line`.
```

Override per session with `--system /path/to/other.md`. The contents are
trimmed and frozen at session creation. The date line — `Today is
2026-06-08.` — is appended fresh on every request and is never stored,
so it stays correct across days.

---

## Context window management

The CLI probes the model's context window on startup via
`/api/show` and shows it on the banner:

```
Model: nemotron-3-nano:30b-cloud (262K ctx)
```

When a turn ends with token usage above 80% of the window, you get a
notice like:

```
(context 86%: 28K / 32K — Ollama will start dropping oldest messages above 100%)
```

`/info` shows current usage explicitly:

```
Session:  01KTKBPRGPNSXN4049PXFEQ486
Model:    nemotron-3-nano:30b-cloud
Context:  14.2K / 262K tokens (5%)
Messages: 12
Last turn: 14215 prompt + 87 reply tokens
```

**Past the limit, Ollama truncates automatically** (its `truncate=true`
default — see the
[Ollama API docs](https://github.com/ollama/ollama/blob/main/docs/api.md)).
Oldest messages are dropped from the prompt, the system message is
preserved. The CLI does not need to do anything; older context just
silently disappears once you cross 100%.

---

## Colour output

The REPL uses colour to distinguish parts of the output:

| Part                                | Colour          |
|-------------------------------------|-----------------|
| Prompt (`>>>`)                      | Cyan            |
| Startup `Model:` label              | Bold            |
| Startup `<name> (<ctx>)`            | Cyan            |
| `Resuming <id> [<model>]` line      | Green           |
| `Type /help for commands.` hint     | Dim             |
| Tool call request (`→ tool(args)`)  | Dim             |
| Tool call response (`← tool …`)     | Dim             |
| Tool call error (`← tool ERROR …`)  | Red             |
| `error: …` / `clear: …` lines       | Red             |
| Assistant text                      | Default (no colour) |

Colours are automatically **disabled** when stdout is not a terminal
(for example when you pipe output to a file, `tee`, or another program).
Pipe-safe scripting works without any flag.

You can also force colour off with the
[`NO_COLOR`](https://no-color.org) environment variable:

```bash
NO_COLOR=1 aiwithtools --continue
```

---

## Environment variables

| Variable           | Effect                                                                 |
|--------------------|------------------------------------------------------------------------|
| `NO_COLOR`         | Disable all colour output when set (any value). Standard convention.   |
| `OLLAMA_HOST`      | URL of the Ollama daemon. Defaults to `http://127.0.0.1:11434`. Use to point at a remote daemon. |
| `XDG_CONFIG_HOME`  | Override the config directory base. Default: `~/.config`.              |
| `XDG_DATA_HOME`    | Override the data directory base. Default: `~/.local/share`.           |
| `HOME` / `USER`    | Used to expand `~`, `$HOME`, `$USER` in MCP server paths.              |

---

## File locations

| Path                                                | Purpose                                                |
|-----------------------------------------------------|--------------------------------------------------------|
| `~/.config/aiwithtools/mcp.json`                    | MCP server configuration.                              |
| `~/.config/aiwithtools/system.md`                   | Default system prompt (optional).                      |
| `~/.local/share/aiwithtools/sessions.db`            | SQLite session store (mode `0600`).                    |

The config directory is created at mode `0700` and the database file at
`0600` — only your user can read them.

---

## Building from source

For development you usually want a local binary in the repo:

```bash
git clone https://github.com/arbasli2/aiwithtools.git
cd aiwithtools

# fast iterative build (writes ./aiwithtools)
go build -o aiwithtools ./cmd/aiwithtools

# stripped, reproducible binary
go build -trimpath -ldflags="-s -w" -o aiwithtools ./cmd/aiwithtools

# cross-compile (Linux amd64)
GOOS=linux GOARCH=amd64 go build -o aiwithtools-linux-amd64 ./cmd/aiwithtools

# put a copy in $GOPATH/bin so it's on PATH
go install ./cmd/aiwithtools

# run all tests
go test ./...
```

The repo is pure Go (modernc.org/sqlite gives us SQLite without CGO),
so cross-compilation Just Works.

---

## Troubleshooting

**"unknown flag: --continue" at the root.** You probably typed
`aiwithtools run --continue` without a model. Either drop the `run`
(`aiwithtools --continue` works at the root) or add the model name
(`aiwithtools run <model> --continue`).

**The REPL opens but `/tools` is empty.** Your MCP config is missing or
all your servers failed to start. Run the CLI and watch stderr — failed
servers log `WARN mcp server failed server=<name> err=<…>`. Common
causes: command not on `PATH`, file path uses `~` but Bash didn't
expand it inside the JSON (`aiwithtools` does — make sure you're not
double-escaping), or the server requires an env var.

**The model returns nothing and the prompt comes back immediately.** The
CLI is supposed to show a placeholder in that case. If it doesn't, you
hit a real bug — open an issue with the model name and reproduction.

**The model says `(stopped: length)`.** Your prompt + previous turns
filled the context window. Either start a fresh session (`/clear` or
exit and `aiwithtools run <model>`) or move to a model with a larger
context. Ollama will keep truncating but the model loses earlier
context.

**Cloud model says auth missing.** Run `ollama signin` and retry.

**Colours show up as garbage characters.** Your terminal doesn't
support ANSI escapes. Set `NO_COLOR=1` permanently in your shell rc.

**`aiwithtools sessions rm <prefix>` says `prefix matches multiple
sessions`.** Use more characters of the ULID. Two characters is rarely
enough; four to eight is usually unique.

---

## Reference

- Source: [github.com/arbasli2/aiwithtools](https://github.com/arbasli2/aiwithtools)
- Design spec: `docs/superpowers/specs/2026-06-07-aiwithtools-cli-design.md`
- Implementation plan: `docs/superpowers/plans/2026-06-07-aiwithtools-implementation.md`
