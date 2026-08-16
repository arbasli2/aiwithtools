# Recommended MCP servers

aiwithtools reads `~/.config/aiwithtools/mcp.json` in the same format as
Claude Desktop and Claude Code, so any MCP server documented for those
clients works here unchanged. This page is a starting point — a short
list of well-maintained servers that cover common needs.

> **Bring your own**: this is a catalog, not a whitelist. If a server
> isn't listed, paste its `mcpServers` block in anyway — the format is
> identical.

## Prerequisites

Most MCP servers ship via one of three package managers. Install
whichever the servers you want need:

| Runtime | Used by                            | Install                         |
| ------- | ---------------------------------- | ------------------------------- |
| Node    | `npx -y …`                         | <https://nodejs.org/> (≥ 20)    |
| Python  | `uvx …`                            | <https://docs.astral.sh/uv/>    |
| Go      | locally built / `go install`       | <https://go.dev/dl/>            |

`npx` and `uvx` fetch and run the server on demand — nothing to install
ahead of time once Node / uv are present.

## Catalog

| Server               | What it does                                            | Runtime         | Notes              |
| -------------------- | ------------------------------------------------------- | --------------- | ------------------ |
| filesystem           | Read/write files inside one or more allowed directories | Node            | reference server   |
| fetch                | HTTP GET a URL and return its content                   | Python (uvx)    | reference server   |
| git                  | Inspect a git repository                                | Python (uvx)    | reference server   |
| time                 | Current time, timezone conversion                       | Python (uvx)    | reference server   |
| memory               | Persistent knowledge graph across sessions              | Node            | reference server   |
| sequential-thinking  | Structured chain-of-thought scratchpad                  | Node            | reference server   |
| tavily               | Web search                                              | Node + API key  | needs `TAVILY_API_KEY` from <https://tavily.com> |

"reference server" means it lives in the official
[modelcontextprotocol/servers](https://github.com/modelcontextprotocol/servers)
repository.

## Drop-in config

Save (or merge) into `~/.config/aiwithtools/mcp.json`. Delete the
servers you don't want — each entry is independent.

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "~/projects"
      ]
    },
    "fetch": {
      "command": "uvx",
      "args": ["mcp-server-fetch"]
    },
    "git": {
      "command": "uvx",
      "args": ["mcp-server-git", "--repository", "~/src/aiwithtools"]
    },
    "time": {
      "command": "uvx",
      "args": ["mcp-server-time"]
    },
    "memory": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-memory"]
    },
    "sequential-thinking": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-sequential-thinking"]
    },
    "tavily": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "https://mcp.tavily.com/mcp/?tavilyApiKey=YOUR_KEY_HERE"
      ]
    }
  }
}
```

After editing the file, restart aiwithtools and run `/tools` in the
REPL to confirm each server connected. Tools appear as
`<server>__<toolname>` (e.g. `filesystem__read_text_file`).

`~/…`, `$HOME` and `$USER` in `command` and `args` are expanded by
aiwithtools before the server is spawned. That's a convenience this
client adds — other MCP clients may want absolute paths instead, so
spell paths out in full if you share a config between clients.

## Security notes

- **`filesystem` exposes everything under each allowed root** — pick
  the narrowest directory that does the job. Don't pass `~` or `/`.
- **API keys belong in `env`** when the server reads them as
  environment variables. The `mcp-remote` proxy embeds Tavily's key in
  the URL because that's what its hosted endpoint expects — keep that
  config file out of source control.
- **Untrusted servers run as you.** An MCP server is an arbitrary
  subprocess; vet anything you didn't write yourself the same way you
  would a CLI tool.

## Where to find more

- Official reference servers: <https://github.com/modelcontextprotocol/servers>
- Community directory: <https://github.com/modelcontextprotocol/servers#community-servers>
- Roll your own: any stdio MCP server in any language works.
