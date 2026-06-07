# aiwithtools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI that wraps Ollama with MCP-based tool calling and a ReAct agent loop, plus persistent SQLite sessions and a REPL.

**Architecture:** Five focused internal packages (`session`, `llm`, `mcp`, `agent`, `repl`) behind a cobra-based `cmd/aiwithtools` entry point. Pure-Go everywhere (no CGO). The spec at `docs/superpowers/specs/2026-06-07-aiwithtools-cli-design.md` is the source of truth — when this plan and the spec disagree, the spec wins.

**Tech Stack:** Go 1.22+, cobra (CLI), modernc.org/sqlite (DB, pure-Go), ollama/api (LLM client), mark3labs/mcp-go (MCP client), chzyer/readline (REPL), oklog/ulid/v2 (session IDs), slog (logging).

---

## Conventions used in every task

- **TDD discipline.** Each behavior-bearing task: write failing test → run it → implement → run again → commit. Pure-wiring tasks (cobra glue, main) get manual smoke checks instead.
- **Go testing.** Standard library `testing` only. No testify. Use table tests where natural. Use `t.TempDir()` for filesystem isolation.
- **Errors.** Wrap with `fmt.Errorf("context: %w", err)`. No sentinel errors in v1.
- **Logging.** `log/slog` to stderr. `slog.Warn`/`slog.Error` for non-fatal events the user should see (MCP server failures, dangling tool_calls).
- **Commits.** One commit per task unless noted. Conventional commit prefixes: `feat`, `test`, `chore`, `docs`.
- **No streaming.** The Ollama `Chat` callback is invoked once with the final message because `Stream: &false` is always set in v1.

---

## Task 1: Project scaffold

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `cmd/aiwithtools/main.go`

- [ ] **Step 1.1: Initialize module**

```bash
cd /Users/amir/src/aiwithtools
go mod init github.com/amir/aiwithtools
```

- [ ] **Step 1.2: Add dependencies**

```bash
go get github.com/spf13/cobra@latest
go get github.com/ollama/ollama/api@latest
go get github.com/mark3labs/mcp-go@latest
go get github.com/chzyer/readline@latest
go get modernc.org/sqlite@latest
go get github.com/oklog/ulid/v2@latest
```

- [ ] **Step 1.3: Write `.gitignore`**

```
/aiwithtools
/dist/
*.test
*.out
.DS_Store
```

- [ ] **Step 1.4: Write minimal `cmd/aiwithtools/main.go`**

```go
package main

import "fmt"

func main() {
    fmt.Println("aiwithtools (scaffold)")
}
```

- [ ] **Step 1.5: Verify build**

```bash
go build ./...
go vet ./...
```

Expected: both succeed silently.

- [ ] **Step 1.6: Commit**

```bash
git add go.mod go.sum .gitignore cmd/
git commit -m "chore: project scaffold"
```

---

## Task 2: Session store — open + migrate

**Files:**
- Create: `internal/session/schema.go`
- Create: `internal/session/store.go`
- Create: `internal/session/store_test.go`

- [ ] **Step 2.1: Write `internal/session/schema.go`**

```go
package session

const schemaVersion = 1

const schemaSQL = `
CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    model       TEXT NOT NULL,
    system      TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_model_updated ON sessions(model, updated_at DESC);

CREATE TABLE IF NOT EXISTS messages (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    role        TEXT NOT NULL,
    content     TEXT NOT NULL,
    tool_calls  TEXT,
    tool_name   TEXT,
    created_at  INTEGER NOT NULL,
    UNIQUE(session_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_messages_session_seq ON messages(session_id, seq);

CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY);
`
```

- [ ] **Step 2.2: Write failing test `internal/session/store_test.go`**

```go
package session

import (
    "path/filepath"
    "testing"
)

func TestOpen_CreatesSchemaOnFreshDB(t *testing.T) {
    path := filepath.Join(t.TempDir(), "sessions.db")
    s, err := Open(path)
    if err != nil {
        t.Fatalf("Open failed: %v", err)
    }
    defer s.Close()

    var v int
    if err := s.db.QueryRow("SELECT version FROM schema_version").Scan(&v); err != nil {
        t.Fatalf("schema_version query failed: %v", err)
    }
    if v != schemaVersion {
        t.Errorf("schema_version = %d, want %d", v, schemaVersion)
    }
}

func TestOpen_ReusesExistingDB(t *testing.T) {
    path := filepath.Join(t.TempDir(), "sessions.db")
    s, err := Open(path)
    if err != nil { t.Fatal(err) }
    s.Close()
    s2, err := Open(path)
    if err != nil { t.Fatalf("reopen failed: %v", err) }
    s2.Close()
}

func TestOpen_FilePermIs0600(t *testing.T) {
    path := filepath.Join(t.TempDir(), "sessions.db")
    s, err := Open(path)
    if err != nil { t.Fatal(err) }
    defer s.Close()
    info, err := osStat(path)
    if err != nil { t.Fatal(err) }
    if perm := info.Mode().Perm(); perm != 0600 {
        t.Errorf("perm = %o, want 0600", perm)
    }
}

// indirection so test compiles before we import os in store.go test scope
func osStat(path string) (fileInfo, error) { return statShim(path) }
```

Note: the `osStat`/`statShim` indirection is awkward. Simplify by importing `os` directly:

```go
package session

import (
    "os"
    "path/filepath"
    "testing"
)

func TestOpen_FilePermIs0600(t *testing.T) {
    path := filepath.Join(t.TempDir(), "sessions.db")
    s, err := Open(path)
    if err != nil { t.Fatal(err) }
    defer s.Close()
    info, err := os.Stat(path)
    if err != nil { t.Fatal(err) }
    if perm := info.Mode().Perm(); perm != 0600 {
        t.Errorf("perm = %o, want 0600", perm)
    }
}
```

Use the `os.Stat` version. Delete the indirection code from the snippet above when writing the file.

- [ ] **Step 2.3: Run tests, see failures**

```bash
go test ./internal/session/...
```

Expected: compile error (no `Open`).

- [ ] **Step 2.4: Write `internal/session/store.go`**

```go
package session

import (
    "database/sql"
    "fmt"
    "os"

    _ "modernc.org/sqlite"
)

type Store struct {
    db *sql.DB
}

func Open(path string) (*Store, error) {
    // Ensure the file (if newly created) gets 0600 perms. SQLite creates
    // the file on first open with mode 0644 (umask-dependent), so we
    // explicitly create-or-touch it ourselves first with 0600.
    f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
    if err != nil {
        return nil, fmt.Errorf("create db file: %w", err)
    }
    f.Close()
    if err := os.Chmod(path, 0600); err != nil {
        return nil, fmt.Errorf("chmod db file: %w", err)
    }

    db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
    if err != nil {
        return nil, fmt.Errorf("open sqlite: %w", err)
    }

    if _, err := db.Exec(schemaSQL); err != nil {
        db.Close()
        return nil, fmt.Errorf("apply schema: %w", err)
    }
    // INSERT OR IGNORE so reopen is a no-op
    if _, err := db.Exec(`INSERT OR IGNORE INTO schema_version(version) VALUES (?)`, schemaVersion); err != nil {
        db.Close()
        return nil, fmt.Errorf("seed schema_version: %w", err)
    }

    return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }
```

- [ ] **Step 2.5: Run tests, verify pass**

```bash
go test ./internal/session/... -v
```

Expected: all three tests pass.

- [ ] **Step 2.6: Commit**

```bash
git add internal/session/
git commit -m "feat(session): SQLite store with schema migration"
```

---

## Task 3: Session — create + append messages

**Files:**
- Create: `internal/session/session.go`
- Create: `internal/session/session_test.go`

- [ ] **Step 3.1: Write failing test `internal/session/session_test.go`**

```go
package session

import (
    "path/filepath"
    "testing"
    "time"
)

func newStore(t *testing.T) *Store {
    t.Helper()
    s, err := Open(filepath.Join(t.TempDir(), "sessions.db"))
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { s.Close() })
    return s
}

func TestCreate_PersistsSession(t *testing.T) {
    s := newStore(t)
    sess, err := s.Create("qwen3:cloud", "you are helpful")
    if err != nil { t.Fatal(err) }
    if sess.ID == "" {
        t.Fatal("ID is empty")
    }
    if sess.Model != "qwen3:cloud" {
        t.Errorf("Model = %q, want qwen3:cloud", sess.Model)
    }
    if sess.System != "you are helpful" {
        t.Errorf("System = %q", sess.System)
    }
    if sess.CreatedAt.IsZero() {
        t.Error("CreatedAt is zero")
    }
}

func TestAppend_StoresInOrder(t *testing.T) {
    s := newStore(t)
    sess, err := s.Create("m", "")
    if err != nil { t.Fatal(err) }

    if err := sess.AppendUser("hello"); err != nil { t.Fatal(err) }
    if err := sess.AppendAssistant("hi", nil); err != nil { t.Fatal(err) }
    if err := sess.AppendTool("weather__forecast", "sunny"); err != nil { t.Fatal(err) }

    msgs, err := sess.Messages()
    if err != nil { t.Fatal(err) }
    if len(msgs) != 3 {
        t.Fatalf("len(msgs) = %d, want 3", len(msgs))
    }
    if msgs[0].Role != "user" || msgs[0].Content != "hello" {
        t.Errorf("msgs[0] = %+v", msgs[0])
    }
    if msgs[1].Role != "assistant" || msgs[1].Content != "hi" {
        t.Errorf("msgs[1] = %+v", msgs[1])
    }
    if msgs[2].Role != "tool" || msgs[2].ToolName != "weather__forecast" {
        t.Errorf("msgs[2] = %+v", msgs[2])
    }
}

func TestAppend_BumpsUpdatedAt(t *testing.T) {
    s := newStore(t)
    sess, err := s.Create("m", "")
    if err != nil { t.Fatal(err) }
    before := sess.UpdatedAt
    time.Sleep(1100 * time.Millisecond) // updated_at is unix seconds
    if err := sess.AppendUser("hi"); err != nil { t.Fatal(err) }

    var updated int64
    if err := s.db.QueryRow(`SELECT updated_at FROM sessions WHERE id = ?`, sess.ID).Scan(&updated); err != nil {
        t.Fatal(err)
    }
    if updated <= before.Unix() {
        t.Errorf("updated_at not bumped: was %d, now %d", before.Unix(), updated)
    }
}

func TestAppendAssistant_PersistsToolCallsJSON(t *testing.T) {
    s := newStore(t)
    sess, err := s.Create("m", "")
    if err != nil { t.Fatal(err) }
    tcs := []ToolCall{{Name: "weather__forecast", Arguments: map[string]any{"city": "Tokyo"}}}
    if err := sess.AppendAssistant("", tcs); err != nil { t.Fatal(err) }
    msgs, err := sess.Messages()
    if err != nil { t.Fatal(err) }
    if len(msgs[0].ToolCalls) != 1 {
        t.Fatalf("ToolCalls len = %d", len(msgs[0].ToolCalls))
    }
    if msgs[0].ToolCalls[0].Name != "weather__forecast" {
        t.Errorf("got name %q", msgs[0].ToolCalls[0].Name)
    }
}
```

- [ ] **Step 3.2: Run tests, see compile failures**

```bash
go test ./internal/session/...
```

Expected: compile errors — `Create`, `Session`, `ToolCall`, `AppendUser`, etc. are undefined.

- [ ] **Step 3.3: Write `internal/session/session.go`**

```go
package session

import (
    "crypto/rand"
    "database/sql"
    "encoding/json"
    "fmt"
    "time"

    "github.com/oklog/ulid/v2"
)

type Session struct {
    ID        string
    Model     string
    System    string
    CreatedAt time.Time
    UpdatedAt time.Time
    store     *Store
}

type Message struct {
    Role      string
    Content   string
    ToolCalls []ToolCall
    ToolName  string
}

type ToolCall struct {
    Name      string         `json:"name"`
    Arguments map[string]any `json:"arguments"`
}

func newULID() string {
    return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}

func (s *Store) Create(model, system string) (*Session, error) {
    id := newULID()
    now := time.Now()
    _, err := s.db.Exec(
        `INSERT INTO sessions(id, model, system, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
        id, model, system, now.Unix(), now.Unix(),
    )
    if err != nil {
        return nil, fmt.Errorf("insert session: %w", err)
    }
    return &Session{
        ID: id, Model: model, System: system,
        CreatedAt: now, UpdatedAt: now,
        store: s,
    }, nil
}

func (s *Session) nextSeq() (int, error) {
    var seq sql.NullInt64
    err := s.store.db.QueryRow(`SELECT MAX(seq) FROM messages WHERE session_id = ?`, s.ID).Scan(&seq)
    if err != nil { return 0, err }
    if !seq.Valid { return 0, nil }
    return int(seq.Int64) + 1, nil
}

func (s *Session) appendRaw(role, content, toolName string, toolCalls []ToolCall) error {
    seq, err := s.nextSeq()
    if err != nil { return fmt.Errorf("next seq: %w", err) }

    var tcJSON sql.NullString
    if len(toolCalls) > 0 {
        b, err := json.Marshal(toolCalls)
        if err != nil { return fmt.Errorf("marshal tool_calls: %w", err) }
        tcJSON = sql.NullString{String: string(b), Valid: true}
    }
    var tn sql.NullString
    if toolName != "" {
        tn = sql.NullString{String: toolName, Valid: true}
    }

    now := time.Now()
    tx, err := s.store.db.Begin()
    if err != nil { return err }
    defer tx.Rollback()

    if _, err := tx.Exec(
        `INSERT INTO messages(session_id, seq, role, content, tool_calls, tool_name, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
        s.ID, seq, role, content, tcJSON, tn, now.Unix(),
    ); err != nil {
        return fmt.Errorf("insert message: %w", err)
    }
    if _, err := tx.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`, now.Unix(), s.ID); err != nil {
        return fmt.Errorf("bump updated_at: %w", err)
    }
    if err := tx.Commit(); err != nil { return err }

    s.UpdatedAt = now
    return nil
}

func (s *Session) AppendUser(content string) error {
    return s.appendRaw("user", content, "", nil)
}
func (s *Session) AppendAssistant(content string, toolCalls []ToolCall) error {
    return s.appendRaw("assistant", content, "", toolCalls)
}
func (s *Session) AppendTool(toolName, content string) error {
    return s.appendRaw("tool", content, toolName, nil)
}

func (s *Session) Messages() ([]Message, error) {
    rows, err := s.store.db.Query(
        `SELECT role, content, tool_calls, tool_name FROM messages WHERE session_id = ? ORDER BY seq ASC`,
        s.ID,
    )
    if err != nil { return nil, err }
    defer rows.Close()

    var out []Message
    for rows.Next() {
        var m Message
        var tcJSON, tn sql.NullString
        if err := rows.Scan(&m.Role, &m.Content, &tcJSON, &tn); err != nil {
            return nil, err
        }
        if tcJSON.Valid {
            if err := json.Unmarshal([]byte(tcJSON.String), &m.ToolCalls); err != nil {
                return nil, fmt.Errorf("unmarshal tool_calls: %w", err)
            }
        }
        if tn.Valid { m.ToolName = tn.String }
        out = append(out, m)
    }
    return out, rows.Err()
}
```

- [ ] **Step 3.4: Run tests, verify pass**

```bash
go test ./internal/session/... -v
```

Expected: all tests pass.

- [ ] **Step 3.5: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat(session): Create + Append* + Messages with updated_at bump"
```

---

## Task 4: Session — list, last-for-model, clear, delete

**Files:**
- Create: `internal/session/list.go`
- Create: `internal/session/list_test.go`

- [ ] **Step 4.1: Write failing tests `internal/session/list_test.go`**

```go
package session

import (
    "testing"
)

func TestLastForModel_ReturnsMostRecent(t *testing.T) {
    s := newStore(t)
    a, _ := s.Create("model-a", "")
    _, _ = s.Create("model-b", "")
    c, _ := s.Create("model-a", "")
    if err := c.AppendUser("hi"); err != nil { t.Fatal(err) }

    id, err := s.LastForModel("model-a")
    if err != nil { t.Fatal(err) }
    if id != c.ID {
        t.Errorf("LastForModel = %q, want %q (created after %q)", id, c.ID, a.ID)
    }
}

func TestLastForModel_NoneReturnsEmpty(t *testing.T) {
    s := newStore(t)
    id, err := s.LastForModel("model-z")
    if err != nil { t.Fatal(err) }
    if id != "" {
        t.Errorf("LastForModel = %q, want empty", id)
    }
}

func TestList_ReturnsSummaries(t *testing.T) {
    s := newStore(t)
    a, _ := s.Create("model-a", "")
    _ = a.AppendUser("first message")
    _, _ = s.Create("model-b", "")

    items, err := s.List("")
    if err != nil { t.Fatal(err) }
    if len(items) != 2 {
        t.Fatalf("List len = %d, want 2", len(items))
    }
    var aSum *Summary
    for i := range items {
        if items[i].ID == a.ID { aSum = &items[i] }
    }
    if aSum == nil { t.Fatal("a not in list") }
    if aSum.FirstUserMessage != "first message" {
        t.Errorf("FirstUserMessage = %q", aSum.FirstUserMessage)
    }
    if aSum.MessageCount != 1 {
        t.Errorf("MessageCount = %d", aSum.MessageCount)
    }
}

func TestList_FilterByModel(t *testing.T) {
    s := newStore(t)
    _, _ = s.Create("model-a", "")
    _, _ = s.Create("model-b", "")
    items, err := s.List("model-a")
    if err != nil { t.Fatal(err) }
    if len(items) != 1 {
        t.Fatalf("filtered len = %d, want 1", len(items))
    }
    if items[0].Model != "model-a" {
        t.Errorf("Model = %q", items[0].Model)
    }
}

func TestClear_DropsMessagesKeepsSession(t *testing.T) {
    s := newStore(t)
    sess, _ := s.Create("m", "sys")
    _ = sess.AppendUser("hi")

    if err := sess.Clear(); err != nil { t.Fatal(err) }

    msgs, _ := sess.Messages()
    if len(msgs) != 0 {
        t.Errorf("after Clear, msgs len = %d", len(msgs))
    }
    id, _ := s.LastForModel("m")
    if id != sess.ID {
        t.Error("session row gone after Clear")
    }
}

func TestDelete_RemovesSessionAndMessages(t *testing.T) {
    s := newStore(t)
    sess, _ := s.Create("m", "")
    _ = sess.AppendUser("hi")

    if err := s.Delete(sess.ID); err != nil { t.Fatal(err) }

    id, _ := s.LastForModel("m")
    if id != "" {
        t.Error("session still present after Delete")
    }
}
```

- [ ] **Step 4.2: Run tests, see compile failures**

```bash
go test ./internal/session/...
```

Expected: undefined `Summary`, `List`, `LastForModel`, `Clear`, `Delete`.

- [ ] **Step 4.3: Write `internal/session/list.go`**

```go
package session

import (
    "database/sql"
    "fmt"
)

type Summary struct {
    ID               string
    Model            string
    UpdatedAt        int64 // unix seconds
    MessageCount     int
    FirstUserMessage string
}

func (s *Store) LastForModel(model string) (string, error) {
    var id string
    err := s.db.QueryRow(
        `SELECT id FROM sessions WHERE model = ? ORDER BY updated_at DESC LIMIT 1`,
        model,
    ).Scan(&id)
    if err == sql.ErrNoRows { return "", nil }
    if err != nil { return "", fmt.Errorf("last for model: %w", err) }
    return id, nil
}

func (s *Store) List(modelFilter string) ([]Summary, error) {
    q := `
        SELECT
            s.id, s.model, s.updated_at,
            (SELECT COUNT(*) FROM messages m WHERE m.session_id = s.id) AS msg_count,
            COALESCE((
                SELECT content FROM messages m
                WHERE m.session_id = s.id AND m.role = 'user'
                ORDER BY seq ASC LIMIT 1
            ), '') AS first_user
        FROM sessions s
    `
    var args []any
    if modelFilter != "" {
        q += ` WHERE s.model = ?`
        args = append(args, modelFilter)
    }
    q += ` ORDER BY s.updated_at DESC`

    rows, err := s.db.Query(q, args...)
    if err != nil { return nil, err }
    defer rows.Close()

    var out []Summary
    for rows.Next() {
        var sum Summary
        if err := rows.Scan(&sum.ID, &sum.Model, &sum.UpdatedAt, &sum.MessageCount, &sum.FirstUserMessage); err != nil {
            return nil, err
        }
        out = append(out, sum)
    }
    return out, rows.Err()
}

func (s *Session) Clear() error {
    _, err := s.store.db.Exec(`DELETE FROM messages WHERE session_id = ?`, s.ID)
    return err
}

func (s *Store) Delete(id string) error {
    _, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
    return err
}
```

- [ ] **Step 4.4: Run tests, verify pass**

```bash
go test ./internal/session/... -v
```

Expected: all pass.

- [ ] **Step 4.5: Commit**

```bash
git add internal/session/list.go internal/session/list_test.go
git commit -m "feat(session): List, LastForModel, Clear, Delete"
```

---

## Task 5: Session — load with dangling tool_calls recovery

When resuming, if the last assistant message had `tool_calls` and the number of following `tool`-role messages is less than the number of calls, we discard the assistant message AND any partial tool messages.

**Files:**
- Modify: `internal/session/session.go` (add `Load`)
- Create: `internal/session/recovery_test.go`

- [ ] **Step 5.1: Write failing test `internal/session/recovery_test.go`**

```go
package session

import (
    "encoding/json"
    "testing"
)

func TestLoad_DropsDanglingAssistantToolCalls(t *testing.T) {
    s := newStore(t)
    sess, _ := s.Create("m", "")
    _ = sess.AppendUser("hi")
    // assistant emits 2 tool_calls, only 1 is answered
    _ = sess.AppendAssistant("", []ToolCall{
        {Name: "a", Arguments: map[string]any{}},
        {Name: "b", Arguments: map[string]any{}},
    })
    _ = sess.AppendTool("a", "result-a")

    loaded, err := s.Load(sess.ID)
    if err != nil { t.Fatal(err) }
    msgs, err := loaded.Messages()
    if err != nil { t.Fatal(err) }

    if len(msgs) != 1 {
        for _, m := range msgs {
            b, _ := json.Marshal(m)
            t.Logf("kept: %s", b)
        }
        t.Fatalf("after recovery len = %d, want 1 (user only)", len(msgs))
    }
    if msgs[0].Role != "user" {
        t.Errorf("kept[0].Role = %q", msgs[0].Role)
    }
}

func TestLoad_KeepsCompleteAssistantToolCalls(t *testing.T) {
    s := newStore(t)
    sess, _ := s.Create("m", "")
    _ = sess.AppendUser("hi")
    _ = sess.AppendAssistant("", []ToolCall{
        {Name: "a", Arguments: map[string]any{}},
    })
    _ = sess.AppendTool("a", "result-a")

    loaded, err := s.Load(sess.ID)
    if err != nil { t.Fatal(err) }
    msgs, _ := loaded.Messages()
    if len(msgs) != 3 {
        t.Errorf("len = %d, want 3", len(msgs))
    }
}

func TestLoad_NoToolCallsIsNoop(t *testing.T) {
    s := newStore(t)
    sess, _ := s.Create("m", "")
    _ = sess.AppendUser("hi")
    _ = sess.AppendAssistant("answer", nil)

    loaded, err := s.Load(sess.ID)
    if err != nil { t.Fatal(err) }
    msgs, _ := loaded.Messages()
    if len(msgs) != 2 {
        t.Errorf("len = %d, want 2", len(msgs))
    }
}
```

- [ ] **Step 5.2: Run tests, see compile failure (Load undefined)**

```bash
go test ./internal/session/...
```

- [ ] **Step 5.3: Add `Load` to `internal/session/session.go`**

```go
func (s *Store) Load(id string) (*Session, error) {
    var sess Session
    var createdAt, updatedAt int64
    err := s.db.QueryRow(
        `SELECT id, model, system, created_at, updated_at FROM sessions WHERE id = ?`, id,
    ).Scan(&sess.ID, &sess.Model, &sess.System, &createdAt, &updatedAt)
    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("session %q not found", id)
    }
    if err != nil { return nil, err }
    sess.CreatedAt = time.Unix(createdAt, 0)
    sess.UpdatedAt = time.Unix(updatedAt, 0)
    sess.store = s

    if err := sess.repairDanglingToolCalls(); err != nil {
        return nil, fmt.Errorf("recovery: %w", err)
    }
    return &sess, nil
}

// repairDanglingToolCalls deletes the last assistant message + any
// following tool messages if the assistant emitted more tool_calls
// than were answered. Logs a warning to stderr when this happens.
func (s *Session) repairDanglingToolCalls() error {
    msgs, err := s.Messages()
    if err != nil { return err }
    if len(msgs) == 0 { return nil }

    // Find the last assistant message with tool_calls.
    lastAssistant := -1
    for i := len(msgs) - 1; i >= 0; i-- {
        if msgs[i].Role == "assistant" {
            lastAssistant = i
            break
        }
    }
    if lastAssistant == -1 { return nil }
    asst := msgs[lastAssistant]
    if len(asst.ToolCalls) == 0 { return nil }

    // Count following tool messages.
    following := 0
    for _, m := range msgs[lastAssistant+1:] {
        if m.Role == "tool" { following++ }
    }
    if following >= len(asst.ToolCalls) { return nil }

    slog.Warn("dropping dangling tool_calls on resume",
        "session", s.ID,
        "expected", len(asst.ToolCalls),
        "got", following,
    )
    _, err = s.store.db.Exec(
        `DELETE FROM messages
         WHERE session_id = ?
         AND seq >= (SELECT seq FROM messages WHERE session_id = ? AND role = 'assistant' ORDER BY seq DESC LIMIT 1)`,
        s.ID, s.ID,
    )
    return err
}
```

Add `"log/slog"` to the imports in `session.go`.

- [ ] **Step 5.4: Run tests, verify pass**

```bash
go test ./internal/session/... -v
```

- [ ] **Step 5.5: Commit**

```bash
git add internal/session/session.go internal/session/recovery_test.go
git commit -m "feat(session): repair dangling tool_calls on resume"
```

---

## Task 6: LLM — system prompt builder (date + multi-day)

Pure function. Takes the user's prompt + the session start time + now, returns the system message text.

**Files:**
- Create: `internal/llm/prompt.go`
- Create: `internal/llm/prompt_test.go`

- [ ] **Step 6.1: Write failing test `internal/llm/prompt_test.go`**

```go
package llm

import (
    "strings"
    "testing"
    "time"
)

func TestBuildSystemMessage_IncludesUserPromptAndDate(t *testing.T) {
    now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
    out := BuildSystemMessage("be helpful", now, now)
    if !strings.Contains(out, "be helpful") {
        t.Errorf("missing user prompt: %q", out)
    }
    if !strings.Contains(out, "Today is 2026-06-07") {
        t.Errorf("missing date line: %q", out)
    }
}

func TestBuildSystemMessage_EmptyUserPromptOK(t *testing.T) {
    now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
    out := BuildSystemMessage("", now, now)
    if !strings.Contains(out, "Today is 2026-06-07") {
        t.Errorf("missing date line in empty case: %q", out)
    }
}

func TestBuildSystemMessage_MultiDayAddsNote(t *testing.T) {
    start := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
    now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
    out := BuildSystemMessage("be helpful", start, now)
    if !strings.Contains(out, "earlier messages from prior days") {
        t.Errorf("missing multi-day note: %q", out)
    }
}

func TestBuildSystemMessage_SameDayNoMultiDayNote(t *testing.T) {
    start := time.Date(2026, 6, 7, 0, 1, 0, 0, time.UTC)
    now := time.Date(2026, 6, 7, 23, 59, 0, 0, time.UTC)
    out := BuildSystemMessage("", start, now)
    if strings.Contains(out, "earlier messages from prior days") {
        t.Errorf("multi-day note should not appear within 24h: %q", out)
    }
}
```

- [ ] **Step 6.2: Run tests, see compile failure**

```bash
go test ./internal/llm/...
```

- [ ] **Step 6.3: Write `internal/llm/prompt.go`**

```go
package llm

import (
    "strings"
    "time"
)

// BuildSystemMessage returns the final system message text to send to
// Ollama for this request. The date suffix is NEVER stored in the
// messages table — it is request-time only.
//
// userPrompt is the (potentially empty) user-supplied prompt frozen at
// session creation. sessionStart is the session's creation time. now is
// the current time. If now - sessionStart >= 24h, a multi-day note is
// appended.
func BuildSystemMessage(userPrompt string, sessionStart, now time.Time) string {
    var b strings.Builder
    if userPrompt != "" {
        b.WriteString(userPrompt)
        b.WriteString("\n\n")
    }
    b.WriteString("Tools are available via function calling. Today is ")
    b.WriteString(now.Format("2006-01-02"))
    b.WriteString(".")
    if now.Sub(sessionStart) >= 24*time.Hour {
        b.WriteString(" This conversation includes earlier messages from prior days.")
    }
    return b.String()
}
```

- [ ] **Step 6.4: Run tests, verify pass**

```bash
go test ./internal/llm/... -v
```

- [ ] **Step 6.5: Commit**

```bash
git add internal/llm/prompt.go internal/llm/prompt_test.go
git commit -m "feat(llm): system prompt builder with date and multi-day note"
```

---

## Task 7: LLM — tool conversion helpers

Bridge between our internal tool representation and Ollama's `api.Tool`.

**Files:**
- Create: `internal/llm/tools.go`
- Create: `internal/llm/tools_test.go`

- [ ] **Step 7.1: Write failing test `internal/llm/tools_test.go`**

```go
package llm

import (
    "encoding/json"
    "testing"
)

func TestToOllamaTool_PassesThroughSchema(t *testing.T) {
    in := Tool{
        Name:        "weather__forecast",
        Description: "Get the weather forecast",
        InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
    }
    out := ToOllamaTool(in)
    if out.Type != "function" {
        t.Errorf("Type = %q, want function", out.Type)
    }
    if out.Function.Name != "weather__forecast" {
        t.Errorf("Function.Name = %q", out.Function.Name)
    }
    if out.Function.Description != "Get the weather forecast" {
        t.Errorf("Function.Description = %q", out.Function.Description)
    }
}

func TestToOllamaTools_ConvertsAll(t *testing.T) {
    in := []Tool{
        {Name: "a", InputSchema: json.RawMessage(`{"type":"object"}`)},
        {Name: "b", InputSchema: json.RawMessage(`{"type":"object"}`)},
    }
    out := ToOllamaTools(in)
    if len(out) != 2 {
        t.Fatalf("len = %d, want 2", len(out))
    }
    if out[0].Function.Name != "a" || out[1].Function.Name != "b" {
        t.Errorf("names: %q %q", out[0].Function.Name, out[1].Function.Name)
    }
}
```

- [ ] **Step 7.2: Run tests, see compile failure**

```bash
go test ./internal/llm/...
```

- [ ] **Step 7.3: Write `internal/llm/tools.go`**

```go
package llm

import (
    "encoding/json"

    "github.com/ollama/ollama/api"
)

// Tool is our internal representation of an MCP tool exposed to the
// model. InputSchema is a JSON Schema (object).
type Tool struct {
    Name        string
    Description string
    InputSchema json.RawMessage
}

func ToOllamaTool(t Tool) api.Tool {
    // api.Tool.Function takes a parameters struct; the simplest robust
    // path is to round-trip the inputSchema through json.RawMessage so
    // anything valid is accepted. We construct via map and re-marshal
    // because api.ToolFunction shape evolves.
    var fn api.ToolFunction
    fn.Name = t.Name
    fn.Description = t.Description
    // Parameters is itself a struct in api; the safe path is to
    // Unmarshal our raw schema into it.
    _ = json.Unmarshal(t.InputSchema, &fn.Parameters)
    return api.Tool{
        Type:     "function",
        Function: fn,
    }
}

func ToOllamaTools(in []Tool) api.Tools {
    out := make(api.Tools, len(in))
    for i := range in {
        out[i] = ToOllamaTool(in[i])
    }
    return out
}
```

If `api.ToolFunction.Parameters` proves not to be JSON-unmarshallable in your installed version, fall back to manually walking the schema. The expected shape (per ollama/api current source) is `ToolFunctionParameters` with `Type`, `Properties`, `Required` — a direct unmarshal works.

- [ ] **Step 7.4: Run tests, verify pass**

```bash
go test ./internal/llm/... -v
```

- [ ] **Step 7.5: Commit**

```bash
git add internal/llm/tools.go internal/llm/tools_test.go
git commit -m "feat(llm): Tool struct + ollama/api conversion"
```

---

## Task 8: LLM — Chat wrapper

Wraps `*api.Client` with a synchronous, non-streaming `Chat` method that returns the final assistant message.

**Files:**
- Create: `internal/llm/client.go`
- Create: `internal/llm/client_test.go`

- [ ] **Step 8.1: Write failing test `internal/llm/client_test.go`**

We test the wrapper against an `httptest.Server` that mocks `/api/chat`. This avoids depending on a real Ollama process while still exercising the wrapper end-to-end.

```go
package llm

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "net/url"
    "testing"

    "github.com/ollama/ollama/api"
)

func TestChat_ReturnsFinalAssistantMessage(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/api/chat" {
            t.Errorf("path = %s", r.URL.Path)
        }
        w.Header().Set("Content-Type", "application/x-ndjson")
        json.NewEncoder(w).Encode(api.ChatResponse{
            Model: "test",
            Message: api.Message{Role: "assistant", Content: "hello!"},
            Done: true,
        })
    }))
    defer srv.Close()

    u, _ := url.Parse(srv.URL)
    c := New(api.NewClient(u, http.DefaultClient))

    msg, err := c.Chat(context.Background(), "test",
        []api.Message{{Role: "user", Content: "hi"}},
        nil,
    )
    if err != nil { t.Fatal(err) }
    if msg.Role != "assistant" || msg.Content != "hello!" {
        t.Errorf("msg = %+v", msg)
    }
}

func TestChat_SurfacesToolCalls(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/x-ndjson")
        json.NewEncoder(w).Encode(api.ChatResponse{
            Model: "test",
            Message: api.Message{
                Role: "assistant",
                ToolCalls: []api.ToolCall{
                    {Function: api.ToolCallFunction{Name: "weather__forecast", Arguments: api.ToolCallFunctionArguments{"city": "Tokyo"}}},
                },
            },
            Done: true,
        })
    }))
    defer srv.Close()

    u, _ := url.Parse(srv.URL)
    c := New(api.NewClient(u, http.DefaultClient))

    msg, err := c.Chat(context.Background(), "test",
        []api.Message{{Role: "user", Content: "weather?"}},
        nil,
    )
    if err != nil { t.Fatal(err) }
    if len(msg.ToolCalls) != 1 {
        t.Fatalf("ToolCalls len = %d", len(msg.ToolCalls))
    }
    if msg.ToolCalls[0].Function.Name != "weather__forecast" {
        t.Errorf("name = %q", msg.ToolCalls[0].Function.Name)
    }
}
```

- [ ] **Step 8.2: Run tests, see compile failure**

```bash
go test ./internal/llm/...
```

- [ ] **Step 8.3: Write `internal/llm/client.go`**

```go
package llm

import (
    "context"
    "fmt"

    "github.com/ollama/ollama/api"
)

type Client struct {
    ollama *api.Client
}

func New(c *api.Client) *Client { return &Client{ollama: c} }

// Chat sends a single non-streaming chat request and returns the final
// assistant message. Tools may be empty.
func (c *Client) Chat(ctx context.Context, model string, messages []api.Message, tools api.Tools) (*api.Message, error) {
    streamFalse := false
    req := &api.ChatRequest{
        Model:    model,
        Messages: messages,
        Stream:   &streamFalse,
        Tools:    tools,
    }

    var last api.Message
    err := c.ollama.Chat(ctx, req, func(resp api.ChatResponse) error {
        // Non-streaming: callback fires once with Done=true.
        last = resp.Message
        return nil
    })
    if err != nil {
        return nil, fmt.Errorf("ollama chat: %w", err)
    }
    return &last, nil
}
```

- [ ] **Step 8.4: Run tests, verify pass**

```bash
go test ./internal/llm/... -v
```

- [ ] **Step 8.5: Commit**

```bash
git add internal/llm/client.go internal/llm/client_test.go
git commit -m "feat(llm): non-streaming Chat wrapper around ollama/api"
```

---

## Task 9: MCP — config parsing + path expansion

Reads the JSON file format used by Claude Code. Expands `~`, `$HOME`, `$USER` in `command` and `args`. No other env-var expansion.

**Files:**
- Create: `internal/mcp/config.go`
- Create: `internal/mcp/expand.go`
- Create: `internal/mcp/config_test.go`
- Create: `internal/mcp/expand_test.go`

- [ ] **Step 9.1: Write failing test `internal/mcp/expand_test.go`**

```go
package mcp

import (
    "testing"
)

func TestExpand_Tilde(t *testing.T) {
    got := expandPath("~/foo/bar", "/home/amir", "amir")
    if got != "/home/amir/foo/bar" {
        t.Errorf("got %q", got)
    }
}

func TestExpand_HomeVar(t *testing.T) {
    got := expandPath("$HOME/x", "/home/amir", "amir")
    if got != "/home/amir/x" {
        t.Errorf("got %q", got)
    }
}

func TestExpand_UserVar(t *testing.T) {
    got := expandPath("/Users/$USER/x", "/home/amir", "amir")
    if got != "/Users/amir/x" {
        t.Errorf("got %q", got)
    }
}

func TestExpand_OtherVarsPassThrough(t *testing.T) {
    got := expandPath("$FOO/x", "/home/amir", "amir")
    if got != "$FOO/x" {
        t.Errorf("got %q", got)
    }
}

func TestExpand_NoExpansionWhenNothingToDo(t *testing.T) {
    got := expandPath("/abs/path", "/home/amir", "amir")
    if got != "/abs/path" {
        t.Errorf("got %q", got)
    }
}
```

- [ ] **Step 9.2: Write `internal/mcp/expand.go`**

```go
package mcp

import "strings"

// expandPath performs the limited set of substitutions documented in
// the spec: leading `~/`, `$HOME`, `$USER`. No other env-var expansion
// is done. The substitutions are applied verbatim and idempotent.
func expandPath(s, home, user string) string {
    if strings.HasPrefix(s, "~/") {
        s = home + s[1:]
    } else if s == "~" {
        s = home
    }
    s = strings.ReplaceAll(s, "$HOME", home)
    s = strings.ReplaceAll(s, "$USER", user)
    return s
}
```

- [ ] **Step 9.3: Run expand tests, verify pass**

```bash
go test ./internal/mcp/... -run TestExpand -v
```

- [ ] **Step 9.4: Write failing test `internal/mcp/config_test.go`**

```go
package mcp

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoadConfig_ParsesExample(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "mcp.json")
    data := []byte(`{
      "mcpServers": {
        "weather": {
          "command": "uv",
          "args": ["run", "~/src/mcp-servers/weather.py"]
        },
        "tavily": {
          "command": "npx",
          "args": ["-y", "mcp-remote", "https://example.com"],
          "env": {"KEY": "x"}
        }
      }
    }`)
    if err := os.WriteFile(p, data, 0600); err != nil { t.Fatal(err) }

    cfg, err := LoadConfig(p, "/home/amir", "amir")
    if err != nil { t.Fatal(err) }
    if len(cfg.Servers) != 2 {
        t.Fatalf("server count = %d, want 2", len(cfg.Servers))
    }

    var weather, tavily *ServerSpec
    for i, s := range cfg.Servers {
        switch s.Name {
        case "weather": weather = &cfg.Servers[i]
        case "tavily":  tavily  = &cfg.Servers[i]
        }
    }
    if weather == nil || tavily == nil {
        t.Fatalf("missing servers: %+v", cfg.Servers)
    }

    if weather.Args[1] != "/home/amir/src/mcp-servers/weather.py" {
        t.Errorf("expand failed: %q", weather.Args[1])
    }
    if tavily.Env["KEY"] != "x" {
        t.Errorf("env not parsed: %v", tavily.Env)
    }
}

func TestLoadConfig_MissingFile(t *testing.T) {
    _, err := LoadConfig("/no/such/file", "/h", "u")
    if err == nil {
        t.Fatal("expected error for missing file")
    }
}
```

- [ ] **Step 9.5: Write `internal/mcp/config.go`**

```go
package mcp

import (
    "encoding/json"
    "fmt"
    "os"
    "sort"
)

type Config struct {
    Servers []ServerSpec
}

type ServerSpec struct {
    Name    string
    Command string
    Args    []string
    Env     map[string]string
}

type rawConfig struct {
    MCPServers map[string]rawServer `json:"mcpServers"`
}
type rawServer struct {
    Command string            `json:"command"`
    Args    []string          `json:"args"`
    Env     map[string]string `json:"env"`
}

func LoadConfig(path, home, user string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read mcp config: %w", err)
    }
    var rc rawConfig
    if err := json.Unmarshal(data, &rc); err != nil {
        return nil, fmt.Errorf("parse mcp config: %w", err)
    }

    cfg := &Config{}
    for name, rs := range rc.MCPServers {
        spec := ServerSpec{
            Name:    name,
            Command: expandPath(rs.Command, home, user),
            Env:     rs.Env,
        }
        for _, a := range rs.Args {
            spec.Args = append(spec.Args, expandPath(a, home, user))
        }
        cfg.Servers = append(cfg.Servers, spec)
    }
    // Deterministic ordering so startup logs and `/tools` are stable.
    sort.Slice(cfg.Servers, func(i, j int) bool { return cfg.Servers[i].Name < cfg.Servers[j].Name })
    return cfg, nil
}
```

- [ ] **Step 9.6: Run tests, verify pass**

```bash
go test ./internal/mcp/... -v
```

- [ ] **Step 9.7: Commit**

```bash
git add internal/mcp/config.go internal/mcp/expand.go internal/mcp/config_test.go internal/mcp/expand_test.go
git commit -m "feat(mcp): config parsing + path expansion"
```

---

## Task 10: MCP — content flattening

MCP tool results are `[]Content` blocks; we flatten to a single string for the Ollama tool-role message.

**Files:**
- Create: `internal/mcp/flatten.go`
- Create: `internal/mcp/flatten_test.go`

- [ ] **Step 10.1: Write failing test `internal/mcp/flatten_test.go`**

```go
package mcp

import (
    "strings"
    "testing"

    "github.com/mark3labs/mcp-go/mcp"
)

func TestFlatten_JoinsTextBlocks(t *testing.T) {
    res := &mcp.CallToolResult{
        Content: []mcp.Content{
            mcp.NewTextContent("hello"),
            mcp.NewTextContent("world"),
        },
    }
    got := flattenResult(res)
    if got != "hello\nworld" {
        t.Errorf("got %q", got)
    }
}

func TestFlatten_ImagePlaceholder(t *testing.T) {
    res := &mcp.CallToolResult{
        Content: []mcp.Content{
            mcp.NewTextContent("see this:"),
            mcp.NewImageContent("base64data", "image/png"),
        },
    }
    got := flattenResult(res)
    if !strings.Contains(got, "[image: image/png") {
        t.Errorf("missing placeholder: %q", got)
    }
}

func TestFlatten_NilSafe(t *testing.T) {
    if flattenResult(nil) != "" {
        t.Error("nil result should flatten to empty string")
    }
}
```

If the exact constructor names in `mcp-go` differ in your installed version (e.g. `NewTextContent` vs `TextContent{}` struct literal), adjust the test to construct the content directly. Check `go doc github.com/mark3labs/mcp-go/mcp.TextContent` first; if `NewTextContent` doesn't exist, the test becomes:

```go
{Content: []mcp.Content{mcp.TextContent{Text: "hello"}, mcp.TextContent{Text: "world"}}}
```

- [ ] **Step 10.2: Write `internal/mcp/flatten.go`**

```go
package mcp

import (
    "fmt"
    "strings"

    mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// flattenResult turns an MCP CallToolResult into a single string for
// the Ollama tool-role message. Text blocks are joined with newlines;
// image / resource blocks become short placeholders. Returns "" for nil.
func flattenResult(r *mcpgo.CallToolResult) string {
    if r == nil { return "" }
    var b strings.Builder
    for i, c := range r.Content {
        if i > 0 { b.WriteByte('\n') }
        switch v := c.(type) {
        case mcpgo.TextContent:
            b.WriteString(v.Text)
        case mcpgo.ImageContent:
            fmt.Fprintf(&b, "[image: %s, omitted]", v.MIMEType)
        case mcpgo.EmbeddedResource:
            b.WriteString("[embedded resource, omitted]")
        default:
            fmt.Fprintf(&b, "[unknown content type %T]", v)
        }
    }
    return b.String()
}
```

If the mcp-go API uses pointer types (e.g. `*mcpgo.TextContent`) instead of value types, change the type-switch cases accordingly. Run `go doc github.com/mark3labs/mcp-go/mcp.Content` to confirm.

- [ ] **Step 10.3: Run tests, verify pass**

```bash
go test ./internal/mcp/... -v
```

- [ ] **Step 10.4: Commit**

```bash
git add internal/mcp/flatten.go internal/mcp/flatten_test.go
git commit -m "feat(mcp): flatten CallToolResult content blocks to string"
```

---

## Task 11: MCP — fake server test fixture

A tiny in-repo MCP server we can spawn from tests. Lives under `testdata/` so it's not part of the build but Go-test-built on demand.

**Files:**
- Create: `internal/mcp/testdata/fakeserver/main.go`

- [ ] **Step 11.1: Write `internal/mcp/testdata/fakeserver/main.go`**

We use the mcp-go server package to expose one tool `echo` that returns its `text` argument.

```go
package main

import (
    "context"
    "fmt"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)

func main() {
    s := server.NewMCPServer("fakeserver", "0.0.1")

    s.AddTool(mcp.NewTool("echo",
        mcp.WithDescription("Echo back the text argument"),
        mcp.WithString("text", mcp.Required(), mcp.Description("text to echo")),
    ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        text, _ := req.Params.Arguments["text"].(string)
        return mcp.NewToolResultText(fmt.Sprintf("echo: %s", text)), nil
    })

    if err := server.ServeStdio(s); err != nil {
        panic(err)
    }
}
```

Constructor names in mcp-go evolve; if `mcp.NewTool` or `server.NewMCPServer` or `server.ServeStdio` differ in your installed version, check `go doc` and adjust. The intent is: a single-tool stdio server we can spawn.

- [ ] **Step 11.2: Verify it builds independently**

```bash
go build -o /tmp/fakeserver-check ./internal/mcp/testdata/fakeserver
rm /tmp/fakeserver-check
```

Expected: builds without error.

- [ ] **Step 11.3: Commit**

```bash
git add internal/mcp/testdata/
git commit -m "test(mcp): in-repo fake MCP server fixture"
```

---

## Task 12: MCP — Host

Hosts the connected MCP servers, exposes the merged tool registry and `Call`.

**Files:**
- Create: `internal/mcp/host.go`
- Create: `internal/mcp/host_test.go`

- [ ] **Step 12.1: Write failing test `internal/mcp/host_test.go`**

```go
package mcp

import (
    "context"
    "os/exec"
    "path/filepath"
    "testing"
    "time"
)

func buildFakeserver(t *testing.T) string {
    t.Helper()
    bin := filepath.Join(t.TempDir(), "fakeserver")
    cmd := exec.Command("go", "build", "-o", bin, "./testdata/fakeserver")
    cmd.Dir = "."
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("build fakeserver: %v\n%s", err, out)
    }
    return bin
}

func TestHost_ConnectsAndListsTools(t *testing.T) {
    bin := buildFakeserver(t)

    h, err := OpenHost(context.Background(), &Config{
        Servers: []ServerSpec{{Name: "fake", Command: bin}},
    })
    if err != nil { t.Fatal(err) }
    defer h.Close()

    tools := h.Tools()
    if len(tools) != 1 {
        t.Fatalf("tool count = %d, want 1", len(tools))
    }
    if tools[0].Name != "fake__echo" {
        t.Errorf("prefixed name = %q", tools[0].Name)
    }
}

func TestHost_CallRoutesToServer(t *testing.T) {
    bin := buildFakeserver(t)
    h, err := OpenHost(context.Background(), &Config{
        Servers: []ServerSpec{{Name: "fake", Command: bin}},
    })
    if err != nil { t.Fatal(err) }
    defer h.Close()

    out, err := h.Call(context.Background(), "fake__echo", map[string]any{"text": "hi"})
    if err != nil { t.Fatal(err) }
    if out != "echo: hi" {
        t.Errorf("got %q, want %q", out, "echo: hi")
    }
}

func TestHost_OneServerFailingDoesNotKillOthers(t *testing.T) {
    bin := buildFakeserver(t)
    h, err := OpenHost(context.Background(), &Config{
        Servers: []ServerSpec{
            {Name: "fake", Command: bin},
            {Name: "broken", Command: "/no/such/binary"},
        },
    })
    if err != nil { t.Fatal(err) }
    defer h.Close()

    tools := h.Tools()
    if len(tools) != 1 || tools[0].Name != "fake__echo" {
        t.Errorf("got tools %+v, want only fake__echo", tools)
    }
}

func TestHost_StartupTimeoutEnforced(t *testing.T) {
    // Use `sleep 60` as a server — it will never respond to initialize.
    // The host should give up after the budget (15s in prod; we override).
    cfg := &Config{Servers: []ServerSpec{
        {Name: "hang", Command: "sleep", Args: []string{"60"}},
    }}
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    h, err := openHostWithBudget(ctx, cfg, 1*time.Second)
    if err != nil { t.Fatal(err) }
    defer h.Close()

    if len(h.Tools()) != 0 {
        t.Errorf("expected no tools from hung server, got %+v", h.Tools())
    }
}
```

- [ ] **Step 12.2: Write `internal/mcp/host.go`**

```go
package mcp

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "sync"
    "time"

    mcpgo "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/client"
)

const defaultStartupBudget = 15 * time.Second

type Host struct {
    clients map[string]*client.Client      // server name → client
    tools   []HostTool                     // ordered for stable display
    lookup  map[string]hostToolLookup      // prefixed name → routing info
    mu      sync.Mutex
}

type HostTool struct {
    Name        string          // prefixed: server__rawname
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

    // env slice in KEY=VALUE form
    var env []string
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
    if err != nil { return "", fmt.Errorf("call %s: %w", prefixed, err) }

    out := flattenResult(res)
    if res.IsError {
        return out, fmt.Errorf("tool reported error: %s", out)
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
```

API caveats: if any of the mcp-go types (`InitializeRequest.Params.ProtocolVersion`, `CallToolRequest.Params.Name`, etc.) have different shapes in your installed version, run `go doc` against the package and adjust. The shape above matches mcp-go v0.x as of June 2026.

- [ ] **Step 12.3: Run tests, verify pass**

```bash
go test ./internal/mcp/... -v
```

The hung-server test may take ~1s; that's expected.

- [ ] **Step 12.4: Commit**

```bash
git add internal/mcp/host.go internal/mcp/host_test.go
git commit -m "feat(mcp): Host with lifecycle, prefixed registry, startup budget"
```

---

## Task 13: Agent — Display interface + ReAct loop

**Files:**
- Create: `internal/agent/display.go`
- Create: `internal/agent/agent.go`
- Create: `internal/agent/agent_test.go`

- [ ] **Step 13.1: Write `internal/agent/display.go`**

```go
package agent

// Display receives events as the agent runs. Implementations live in
// the repl package (terminal) and tests (no-op or capture).
type Display interface {
    ToolCallStart(name string, args map[string]any)
    ToolCallEnd(name, output string, err error)
    AssistantFinal(content string)
}

type NopDisplay struct{}

func (NopDisplay) ToolCallStart(string, map[string]any) {}
func (NopDisplay) ToolCallEnd(string, string, error)    {}
func (NopDisplay) AssistantFinal(string)                {}
```

- [ ] **Step 13.2: Write failing test `internal/agent/agent_test.go`**

```go
package agent

import (
    "context"
    "errors"
    "testing"

    "github.com/ollama/ollama/api"
)

type fakeLLM struct {
    responses []api.Message // queued in order
    calls     int
}

func (f *fakeLLM) Chat(ctx context.Context, model string, msgs []api.Message, tools api.Tools) (*api.Message, error) {
    if f.calls >= len(f.responses) {
        return nil, errors.New("fakeLLM exhausted")
    }
    r := f.responses[f.calls]
    f.calls++
    return &r, nil
}

type fakeMCP struct {
    out map[string]string
    err map[string]error
}

func (f *fakeMCP) Tools() []api.Tool { return nil }
func (f *fakeMCP) Call(ctx context.Context, name string, args map[string]any) (string, error) {
    if e, ok := f.err[name]; ok { return "", e }
    return f.out[name], nil
}

type captureDisplay struct {
    starts []string
    ends   []string
    finals []string
}

func (c *captureDisplay) ToolCallStart(n string, _ map[string]any) { c.starts = append(c.starts, n) }
func (c *captureDisplay) ToolCallEnd(n, _ string, _ error)         { c.ends = append(c.ends, n) }
func (c *captureDisplay) AssistantFinal(s string)                  { c.finals = append(c.finals, s) }

type fakeSession struct {
    msgs []api.Message
}

func (f *fakeSession) AppendUser(c string) error {
    f.msgs = append(f.msgs, api.Message{Role: "user", Content: c})
    return nil
}
func (f *fakeSession) AppendAssistant(c string, tcs []api.ToolCall) error {
    f.msgs = append(f.msgs, api.Message{Role: "assistant", Content: c, ToolCalls: tcs})
    return nil
}
func (f *fakeSession) AppendTool(name, c string) error {
    f.msgs = append(f.msgs, api.Message{Role: "tool", Content: c, ToolName: name})
    return nil
}
func (f *fakeSession) Messages() []api.Message { return f.msgs }

func newAgent(llm LLM, mcp MCP, sess Session, disp Display, maxIter int) *Agent {
    return &Agent{LLM: llm, MCP: mcp, Sess: sess, Display: disp, Model: "m", MaxIter: maxIter}
}

func TestRun_NoToolCallsReturnsFinal(t *testing.T) {
    llm := &fakeLLM{responses: []api.Message{
        {Role: "assistant", Content: "hi!"},
    }}
    disp := &captureDisplay{}
    a := newAgent(llm, &fakeMCP{}, &fakeSession{}, disp, 5)

    if err := a.Run(context.Background(), "hello"); err != nil { t.Fatal(err) }
    if len(disp.finals) != 1 || disp.finals[0] != "hi!" {
        t.Errorf("finals = %v", disp.finals)
    }
}

func TestRun_ToolCallThenFinal(t *testing.T) {
    llm := &fakeLLM{responses: []api.Message{
        {Role: "assistant", ToolCalls: []api.ToolCall{
            {Function: api.ToolCallFunction{Name: "echo", Arguments: api.ToolCallFunctionArguments{"text": "hi"}}},
        }},
        {Role: "assistant", Content: "done"},
    }}
    mcp := &fakeMCP{out: map[string]string{"echo": "echo: hi"}}
    disp := &captureDisplay{}
    a := newAgent(llm, mcp, &fakeSession{}, disp, 5)

    if err := a.Run(context.Background(), "say hi"); err != nil { t.Fatal(err) }
    if len(disp.starts) != 1 || disp.starts[0] != "echo" {
        t.Errorf("starts = %v", disp.starts)
    }
    if len(disp.finals) != 1 || disp.finals[0] != "done" {
        t.Errorf("finals = %v", disp.finals)
    }
}

func TestRun_ToolErrorIsSerializedNotFatal(t *testing.T) {
    llm := &fakeLLM{responses: []api.Message{
        {Role: "assistant", ToolCalls: []api.ToolCall{
            {Function: api.ToolCallFunction{Name: "broken", Arguments: api.ToolCallFunctionArguments{}}},
        }},
        {Role: "assistant", Content: "recovered"},
    }}
    mcp := &fakeMCP{err: map[string]error{"broken": errors.New("bang")}}
    a := newAgent(llm, mcp, &fakeSession{}, &captureDisplay{}, 5)

    if err := a.Run(context.Background(), "go"); err != nil { t.Fatalf("want nil, got %v", err) }
}

func TestRun_MaxIterReachedErrors(t *testing.T) {
    // Every assistant message asks for a tool call → loop forever.
    looping := api.Message{Role: "assistant", ToolCalls: []api.ToolCall{
        {Function: api.ToolCallFunction{Name: "echo", Arguments: api.ToolCallFunctionArguments{}}},
    }}
    llm := &fakeLLM{responses: []api.Message{looping, looping, looping}}
    mcp := &fakeMCP{out: map[string]string{"echo": ""}}
    a := newAgent(llm, mcp, &fakeSession{}, &captureDisplay{}, 2)

    err := a.Run(context.Background(), "x")
    if err == nil || !errors.Is(err, ErrMaxIterations) {
        t.Errorf("err = %v, want ErrMaxIterations", err)
    }
}
```

- [ ] **Step 13.3: Run tests, see compile failures**

```bash
go test ./internal/agent/...
```

- [ ] **Step 13.4: Write `internal/agent/agent.go`**

```go
package agent

import (
    "context"
    "errors"
    "fmt"

    "github.com/ollama/ollama/api"
)

var ErrMaxIterations = errors.New("max iterations reached")

type LLM interface {
    Chat(ctx context.Context, model string, msgs []api.Message, tools api.Tools) (*api.Message, error)
}

type MCP interface {
    Tools() []api.Tool
    Call(ctx context.Context, name string, args map[string]any) (string, error)
}

type Session interface {
    AppendUser(content string) error
    AppendAssistant(content string, toolCalls []api.ToolCall) error
    AppendTool(name, content string) error
    Messages() []api.Message
}

type Agent struct {
    LLM     LLM
    MCP     MCP
    Sess    Session
    Display Display
    Model   string
    MaxIter int
}

func (a *Agent) Run(ctx context.Context, userInput string) error {
    if err := a.Sess.AppendUser(userInput); err != nil {
        return fmt.Errorf("append user: %w", err)
    }

    for i := 0; i < a.MaxIter; i++ {
        resp, err := a.LLM.Chat(ctx, a.Model, a.Sess.Messages(), a.MCP.Tools())
        if err != nil { return fmt.Errorf("chat: %w", err) }

        if err := a.Sess.AppendAssistant(resp.Content, resp.ToolCalls); err != nil {
            return fmt.Errorf("append assistant: %w", err)
        }

        if len(resp.ToolCalls) == 0 {
            a.Display.AssistantFinal(resp.Content)
            return nil
        }

        for _, tc := range resp.ToolCalls {
            args := map[string]any(tc.Function.Arguments)
            a.Display.ToolCallStart(tc.Function.Name, args)

            out, callErr := a.MCP.Call(ctx, tc.Function.Name, args)
            content := out
            if callErr != nil {
                content = fmt.Sprintf("ERROR: %s", callErr)
            }
            a.Display.ToolCallEnd(tc.Function.Name, content, callErr)

            if err := a.Sess.AppendTool(tc.Function.Name, content); err != nil {
                return fmt.Errorf("append tool: %w", err)
            }
        }
    }
    return fmt.Errorf("%w (limit=%d)", ErrMaxIterations, a.MaxIter)
}
```

- [ ] **Step 13.5: Run tests, verify pass**

```bash
go test ./internal/agent/... -v
```

- [ ] **Step 13.6: Commit**

```bash
git add internal/agent/
git commit -m "feat(agent): ReAct loop with Display interface"
```

---

## Task 14: REPL — slash command dispatcher

Pure function for routing slash inputs. Pulled out of readline plumbing so it's directly testable.

**Files:**
- Create: `internal/repl/slash.go`
- Create: `internal/repl/slash_test.go`

- [ ] **Step 14.1: Write failing test `internal/repl/slash_test.go`**

```go
package repl

import "testing"

func TestParseSlash_KnownCommands(t *testing.T) {
    cases := map[string]SlashCommand{
        "/clear": SlashClear,
        "/exit":  SlashExit,
        "/bye":   SlashExit,
        "/tools": SlashTools,
        "/help":  SlashHelp,
    }
    for in, want := range cases {
        if got, ok := ParseSlash(in); !ok || got != want {
            t.Errorf("ParseSlash(%q) = (%v, %v), want (%v, true)", in, got, ok, want)
        }
    }
}

func TestParseSlash_NonSlashReturnsFalse(t *testing.T) {
    if _, ok := ParseSlash("hello"); ok {
        t.Error("non-slash input recognized as command")
    }
}

func TestParseSlash_UnknownSlashReturnsFalse(t *testing.T) {
    if _, ok := ParseSlash("/wat"); ok {
        t.Error("unknown slash input recognized")
    }
}
```

- [ ] **Step 14.2: Write `internal/repl/slash.go`**

```go
package repl

import "strings"

type SlashCommand int

const (
    SlashUnknown SlashCommand = iota
    SlashClear
    SlashExit
    SlashTools
    SlashHelp
)

func ParseSlash(input string) (SlashCommand, bool) {
    s := strings.TrimSpace(input)
    if !strings.HasPrefix(s, "/") { return SlashUnknown, false }
    switch s {
    case "/clear": return SlashClear, true
    case "/exit", "/bye": return SlashExit, true
    case "/tools": return SlashTools, true
    case "/help": return SlashHelp, true
    }
    return SlashUnknown, false
}

const HelpText = `Slash commands:
  /clear   drop all messages from this session (session stays, --continue still finds it)
  /tools   list connected MCP servers and their tools
  /exit    quit (also /bye)
  /help    show this message`
```

- [ ] **Step 14.3: Run tests, verify pass**

```bash
go test ./internal/repl/... -v
```

- [ ] **Step 14.4: Commit**

```bash
git add internal/repl/slash.go internal/repl/slash_test.go
git commit -m "feat(repl): slash command parser"
```

---

## Task 15: REPL — terminal Display + readline loop

The terminal `Display` impl and the readline loop together. Limited test coverage by nature; the slash dispatch logic is already covered in Task 14.

**Files:**
- Create: `internal/repl/display.go`
- Create: `internal/repl/repl.go`

- [ ] **Step 15.1: Write `internal/repl/display.go`**

```go
package repl

import (
    "encoding/json"
    "fmt"
    "io"
)

// Terminal implements agent.Display by writing to an io.Writer.
type Terminal struct {
    Out     io.Writer
    Verbose bool
}

func (t Terminal) ToolCallStart(name string, args map[string]any) {
    b, _ := json.Marshal(args)
    fmt.Fprintf(t.Out, "\x1b[2m→ %s(%s)\x1b[0m\n", name, string(b))
}

func (t Terminal) ToolCallEnd(name, output string, err error) {
    if err != nil {
        fmt.Fprintf(t.Out, "\x1b[31m← %s ERROR: %s\x1b[0m\n", name, err)
        return
    }
    if t.Verbose {
        fmt.Fprintf(t.Out, "\x1b[2m← %s:\n%s\x1b[0m\n", name, output)
    } else {
        fmt.Fprintf(t.Out, "\x1b[2m← %s (%d chars)\x1b[0m\n", name, len(output))
    }
}

func (t Terminal) AssistantFinal(content string) {
    fmt.Fprintln(t.Out, content)
}
```

- [ ] **Step 15.2: Write `internal/repl/repl.go`**

```go
package repl

import (
    "context"
    "fmt"
    "io"
    "os"
    "strings"

    "github.com/chzyer/readline"
)

type Runner struct {
    Prompt     string
    Out        io.Writer
    OnUser     func(ctx context.Context, line string) error
    OnClear    func() error
    OnTools    func() string
    OnExit     func() error
}

// Run blocks until the user exits. Handles slash commands inline and
// dispatches free-text lines to OnUser. Ctrl-C cancels the in-flight
// turn but does not exit the REPL.
func (r *Runner) Run(ctx context.Context) error {
    rl, err := readline.New(r.Prompt)
    if err != nil { return err }
    defer rl.Close()

    for {
        line, err := rl.Readline()
        if err == readline.ErrInterrupt {
            // empty Ctrl-C at the prompt: ignore.
            continue
        }
        if err == io.EOF {
            return r.OnExit()
        }
        if err != nil { return err }

        line = strings.TrimSpace(line)
        if line == "" { continue }

        if cmd, ok := ParseSlash(line); ok {
            switch cmd {
            case SlashClear:
                if err := r.OnClear(); err != nil {
                    fmt.Fprintf(r.Out, "clear: %s\n", err)
                }
            case SlashExit:
                return r.OnExit()
            case SlashTools:
                fmt.Fprintln(r.Out, r.OnTools())
            case SlashHelp:
                fmt.Fprintln(r.Out, HelpText)
            }
            continue
        }

        turnCtx, cancel := context.WithCancel(ctx)
        rl.Config.SetListener(func(line []rune, pos int, key rune) ([]rune, int, bool) { return line, pos, false })
        // Ctrl-C during a turn cancels the context, freeing the agent.
        sigCh := make(chan os.Signal, 1)
        go func() {
            <-sigCh
            cancel()
        }()
        if err := r.OnUser(turnCtx, line); err != nil {
            fmt.Fprintf(r.Out, "error: %s\n", err)
        }
        cancel()
    }
}
```

Note: the Ctrl-C handling above is incomplete by design — readline already grabs SIGINT. The simpler path is to let readline handle interruption at the prompt and rely on context cancellation propagating from main's signal handler for in-turn cancel. Wire signal.Notify in `cmd/aiwithtools/main.go` instead, and simplify Run by removing the sigCh code. The final REPL just needs:

```go
if err := r.OnUser(ctx, line); err != nil {
    fmt.Fprintf(r.Out, "error: %s\n", err)
}
```

Use that simpler form. Delete the sigCh/cancel block when writing.

- [ ] **Step 15.3: Verify build**

```bash
go build ./...
go vet ./...
```

- [ ] **Step 15.4: Commit**

```bash
git add internal/repl/display.go internal/repl/repl.go
git commit -m "feat(repl): terminal Display + readline runner"
```

---

## Task 16: cmd/aiwithtools — paths

Resolve XDG-style paths cross-platform (`$XDG_CONFIG_HOME` overrides, fallback to `$HOME/.config`).

**Files:**
- Create: `cmd/aiwithtools/paths.go`
- Create: `cmd/aiwithtools/paths_test.go`

- [ ] **Step 16.1: Write failing test `cmd/aiwithtools/paths_test.go`**

```go
package main

import "testing"

func TestConfigDir_DefaultsToHomeConfig(t *testing.T) {
    got := configDir("/home/amir", "")
    if got != "/home/amir/.config/aiwithtools" {
        t.Errorf("got %q", got)
    }
}

func TestConfigDir_XDGOverride(t *testing.T) {
    got := configDir("/home/amir", "/tmp/xdg")
    if got != "/tmp/xdg/aiwithtools" {
        t.Errorf("got %q", got)
    }
}

func TestDataDir_DefaultsToHomeShare(t *testing.T) {
    got := dataDir("/home/amir", "")
    if got != "/home/amir/.local/share/aiwithtools" {
        t.Errorf("got %q", got)
    }
}
```

- [ ] **Step 16.2: Write `cmd/aiwithtools/paths.go`**

```go
package main

import "path/filepath"

func configDir(home, xdgConfigHome string) string {
    if xdgConfigHome != "" {
        return filepath.Join(xdgConfigHome, "aiwithtools")
    }
    return filepath.Join(home, ".config", "aiwithtools")
}

func dataDir(home, xdgDataHome string) string {
    if xdgDataHome != "" {
        return filepath.Join(xdgDataHome, "aiwithtools")
    }
    return filepath.Join(home, ".local", "share", "aiwithtools")
}
```

- [ ] **Step 16.3: Run tests, verify pass**

```bash
go test ./cmd/aiwithtools/... -v
```

- [ ] **Step 16.4: Commit**

```bash
git add cmd/aiwithtools/paths.go cmd/aiwithtools/paths_test.go
git commit -m "feat(cmd): XDG-aware path resolution"
```

---

## Task 17: cmd/aiwithtools — `run` subcommand

This is the integration point. It wires everything together.

**Files:**
- Modify: `cmd/aiwithtools/main.go`
- Create: `cmd/aiwithtools/run.go`

- [ ] **Step 17.1: Replace `cmd/aiwithtools/main.go`**

```go
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

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

    if err := root.ExecuteContext(ctx); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

- [ ] **Step 17.2: Write `cmd/aiwithtools/run.go`**

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "net/url"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/ollama/ollama/api"
    "github.com/spf13/cobra"

    "github.com/amir/aiwithtools/internal/agent"
    "github.com/amir/aiwithtools/internal/llm"
    "github.com/amir/aiwithtools/internal/mcp"
    "github.com/amir/aiwithtools/internal/repl"
    "github.com/amir/aiwithtools/internal/session"
)

func newRunCmd() *cobra.Command {
    var (
        cont        bool
        resume      bool
        systemFile  string
        maxIter     int
        verbose     bool
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
    return cmd
}

func runRun(ctx context.Context, model string, cont, resume bool, systemFile string, maxIter int, verbose bool) error {
    home, _ := os.UserHomeDir()
    cfgDir := configDir(home, os.Getenv("XDG_CONFIG_HOME"))
    dataD := dataDir(home, os.Getenv("XDG_DATA_HOME"))

    if err := os.MkdirAll(cfgDir, 0700); err != nil { return err }
    if err := os.MkdirAll(dataD, 0700); err != nil { return err }

    // System prompt
    if systemFile == "" {
        systemFile = filepath.Join(cfgDir, "system.md")
    }
    systemPrompt := ""
    if b, err := os.ReadFile(systemFile); err == nil {
        systemPrompt = strings.TrimSpace(string(b))
    } else if !errors.Is(err, os.ErrNotExist) {
        return fmt.Errorf("read system prompt: %w", err)
    }

    // Session store
    store, err := session.Open(filepath.Join(dataD, "sessions.db"))
    if err != nil { return err }
    defer store.Close()

    // MCP host
    user := os.Getenv("USER")
    mcfg, err := mcp.LoadConfig(filepath.Join(cfgDir, "mcp.json"), home, user)
    if err != nil && !errors.Is(err, os.ErrNotExist) { return err }
    if mcfg == nil { mcfg = &mcp.Config{} }
    host, err := mcp.OpenHost(ctx, mcfg)
    if err != nil { return err }
    defer host.Close()

    // LLM client (talks to local Ollama daemon)
    ollamaClient, err := api.ClientFromEnvironment()
    if err != nil { return err }
    if ollamaClient == nil {
        u, _ := url.Parse("http://localhost:11434")
        ollamaClient = api.NewClient(u, nil)
    }
    llmClient := llm.New(ollamaClient)

    // Resolve / create the session
    sess, err := pickSession(store, model, cont, resume)
    if err != nil { return err }
    if sess == nil {
        sess, err = store.Create(model, systemPrompt)
        if err != nil { return err }
    }

    // MCP tools → llm.Tool → api.Tools
    var tools []llm.Tool
    for _, t := range host.Tools() {
        tools = append(tools, llm.Tool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
    }
    ollamaTools := llm.ToOllamaTools(tools)

    // Adapter: agent expects an LLM that already knows the model + tools shape.
    llmAdapter := &llmAdapter{c: llmClient, tools: ollamaTools, sessSystem: sess.System, sessStart: sess.CreatedAt, now: time.Now}
    sessAdapter := &sessionAdapter{s: sess}
    mcpAdapter := &mcpAdapter{h: host, tools: ollamaTools}

    a := &agent.Agent{
        LLM:     llmAdapter,
        MCP:     mcpAdapter,
        Sess:    sessAdapter,
        Display: repl.Terminal{Out: os.Stdout, Verbose: verbose},
        Model:   model,
        MaxIter: maxIter,
    }

    runner := &repl.Runner{
        Prompt: ">>> ",
        Out:    os.Stdout,
        OnUser: func(ctx context.Context, line string) error { return a.Run(ctx, line) },
        OnClear: func() error { return sess.Clear() },
        OnTools: func() string { return formatTools(host.Tools()) },
        OnExit:  func() error { return nil },
    }
    return runner.Run(ctx)
}

func pickSession(store *session.Store, model string, cont, resume bool) (*session.Session, error) {
    if cont {
        id, err := store.LastForModel(model)
        if err != nil { return nil, err }
        if id == "" {
            return nil, fmt.Errorf("no prior session for model %q", model)
        }
        return store.Load(id)
    }
    if resume {
        items, err := store.List(model)
        if err != nil { return nil, err }
        if len(items) == 0 {
            return nil, fmt.Errorf("no sessions for model %q", model)
        }
        for i, it := range items {
            fmt.Printf("%2d. %s  msgs=%d  last=%s  %q\n",
                i+1, it.ID[:8], it.MessageCount, time.Unix(it.UpdatedAt, 0).Format(time.RFC3339), truncate(it.FirstUserMessage, 60))
        }
        fmt.Print("pick a number: ")
        var n int
        if _, err := fmt.Scanln(&n); err != nil { return nil, err }
        if n < 1 || n > len(items) {
            return nil, fmt.Errorf("invalid selection")
        }
        return store.Load(items[n-1].ID)
    }
    return nil, nil
}

func truncate(s string, n int) string {
    s = strings.ReplaceAll(s, "\n", " ")
    if len(s) <= n { return s }
    return s[:n] + "…"
}

func formatTools(tools []mcp.HostTool) string {
    if len(tools) == 0 { return "(no MCP tools loaded)" }
    var b strings.Builder
    b.WriteString("Available MCP tools:\n")
    for _, t := range tools {
        fmt.Fprintf(&b, "  %s — %s\n", t.Name, t.Description)
    }
    return b.String()
}

// ===== adapters between internal packages and the agent interfaces =====

type llmAdapter struct {
    c          *llm.Client
    tools      api.Tools
    sessSystem string
    sessStart  time.Time
    now        func() time.Time
}

func (a *llmAdapter) Chat(ctx context.Context, model string, msgs []api.Message, tools api.Tools) (*api.Message, error) {
    // Replace the model's view of "system" message with our request-time
    // built one. The session-stored system message is just the user's
    // text; the date suffix is injected here.
    built := llm.BuildSystemMessage(a.sessSystem, a.sessStart, a.now())
    withSystem := make([]api.Message, 0, len(msgs)+1)
    withSystem = append(withSystem, api.Message{Role: "system", Content: built})
    for _, m := range msgs {
        if m.Role == "system" { continue } // never duplicate
        withSystem = append(withSystem, m)
    }
    return a.c.Chat(ctx, model, withSystem, tools)
}

type sessionAdapter struct{ s *session.Session }

func (a *sessionAdapter) AppendUser(c string) error { return a.s.AppendUser(c) }
func (a *sessionAdapter) AppendAssistant(c string, tcs []api.ToolCall) error {
    var out []session.ToolCall
    for _, tc := range tcs {
        out = append(out, session.ToolCall{Name: tc.Function.Name, Arguments: map[string]any(tc.Function.Arguments)})
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
            am.ToolCalls = append(am.ToolCalls, api.ToolCall{Function: api.ToolCallFunction{
                Name: tc.Name, Arguments: api.ToolCallFunctionArguments(tc.Arguments),
            }})
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
```

- [ ] **Step 17.3: Verify build**

```bash
go build ./...
go vet ./...
```

- [ ] **Step 17.4: Smoke check the binary starts**

```bash
./aiwithtools --help
./aiwithtools run --help
```

Expected: cobra prints usage. Do NOT actually run a session yet — no daemon assumed.

- [ ] **Step 17.5: Commit**

```bash
git add cmd/aiwithtools/main.go cmd/aiwithtools/run.go
git commit -m "feat(cmd): run subcommand wiring agent + repl + session + mcp + llm"
```

---

## Task 18: cmd/aiwithtools — `sessions` subcommand

**Files:**
- Create: `cmd/aiwithtools/sessions_cmd.go`

- [ ] **Step 18.1: Write `cmd/aiwithtools/sessions_cmd.go`**

```go
package main

import (
    "fmt"
    "os"
    "path/filepath"
    "text/tabwriter"
    "time"

    "github.com/spf13/cobra"

    "github.com/amir/aiwithtools/internal/session"
)

func newSessionsCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "sessions",
        Short: "List sessions",
        RunE: func(cmd *cobra.Command, args []string) error {
            model, _ := cmd.Flags().GetString("model")
            return sessionsList(model)
        },
    }
    cmd.Flags().String("model", "", "filter to a single model")

    rm := &cobra.Command{
        Use:   "rm <id>",
        Short: "Delete a session (or all with --all)",
        RunE: func(cmd *cobra.Command, args []string) error {
            all, _ := cmd.Flags().GetBool("all")
            if all {
                return sessionsRmAll()
            }
            if len(args) != 1 {
                return fmt.Errorf("expected <id> or --all")
            }
            return sessionsRm(args[0])
        },
    }
    rm.Flags().Bool("all", false, "delete every session")
    cmd.AddCommand(rm)
    return cmd
}

func openStore() (*session.Store, error) {
    home, _ := os.UserHomeDir()
    dataD := dataDir(home, os.Getenv("XDG_DATA_HOME"))
    return session.Open(filepath.Join(dataD, "sessions.db"))
}

func sessionsList(model string) error {
    s, err := openStore()
    if err != nil { return err }
    defer s.Close()

    items, err := s.List(model)
    if err != nil { return err }

    if len(items) == 0 {
        fmt.Println("(no sessions)")
        return nil
    }

    tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
    fmt.Fprintln(tw, "ID\tMODEL\tLAST\tMSGS\tFIRST")
    for _, it := range items {
        fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
            it.ID[:8], it.Model,
            time.Unix(it.UpdatedAt, 0).Format("2006-01-02 15:04"),
            it.MessageCount, truncate(it.FirstUserMessage, 50),
        )
    }
    return tw.Flush()
}

func sessionsRm(idPrefix string) error {
    s, err := openStore()
    if err != nil { return err }
    defer s.Close()

    items, err := s.List("")
    if err != nil { return err }
    var match string
    for _, it := range items {
        if len(it.ID) >= len(idPrefix) && it.ID[:len(idPrefix)] == idPrefix {
            if match != "" {
                return fmt.Errorf("prefix %q matches multiple sessions", idPrefix)
            }
            match = it.ID
        }
    }
    if match == "" {
        return fmt.Errorf("no session matching %q", idPrefix)
    }
    return s.Delete(match)
}

func sessionsRmAll() error {
    s, err := openStore()
    if err != nil { return err }
    defer s.Close()
    items, err := s.List("")
    if err != nil { return err }
    for _, it := range items {
        if err := s.Delete(it.ID); err != nil { return err }
    }
    return nil
}
```

- [ ] **Step 18.2: Verify build + smoke**

```bash
go build ./...
./aiwithtools sessions --help
./aiwithtools sessions
```

The bare `sessions` may produce `(no sessions)` on a fresh machine — that's expected.

- [ ] **Step 18.3: Commit**

```bash
git add cmd/aiwithtools/sessions_cmd.go
git commit -m "feat(cmd): sessions list/rm with --model and --all"
```

---

## Task 19: README + manual smoke test instructions

**Files:**
- Create: `README.md`

- [ ] **Step 19.1: Write `README.md`**

```markdown
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
>>> save the text 'hello' using the text-saver tool, then tell me what you did
```

Expected behavior: the model calls `text-saver__save_text` (you'll see the `→` line in the REPL), the result comes back (`←` line), then a final summary. This exercises the full ReAct loop end-to-end.

## Architecture

See `docs/superpowers/specs/2026-06-07-aiwithtools-cli-design.md`.
```

- [ ] **Step 19.2: Commit**

```bash
git add README.md
git commit -m "docs: README with install, config, REPL, and smoke test"
```

---

## Task 20: End-to-end smoke verification (manual)

Not automatable. The implementer runs this and reports back.

- [ ] **Step 20.1: Ensure prerequisites**

```bash
ollama serve &           # in another terminal, or check status
ollama signin            # cloud models require auth
ollama list              # confirm daemon reachable
```

- [ ] **Step 20.2: Place a working MCP config**

```bash
mkdir -p ~/.config/aiwithtools
cp etc/example/mcp.json ~/.config/aiwithtools/mcp.json
# edit out servers you don't have locally to avoid noise
```

- [ ] **Step 20.3: Build and run**

```bash
go build -o aiwithtools ./cmd/aiwithtools
./aiwithtools run nemotron-3-nano:30b-cloud
```

- [ ] **Step 20.4: Verify ReAct end-to-end**

At the `>>>` prompt:

```
>>> save the text 'hello' using the text-saver tool, then tell me what you did
```

Expected output structure:

```
→ text-saver__save_text({"text":"hello"})
← text-saver__save_text (NN chars)
I've saved the text "hello"…
```

If you see only a final answer with no `→` line, the model didn't call the tool. Check `/tools` shows it loaded.

- [ ] **Step 20.5: Verify resume works**

```
>>> /exit
$ ./aiwithtools run nemotron-3-nano:30b-cloud --continue
>>> what did I just ask you to do?
```

Expected: the model recalls the previous turn.

- [ ] **Step 20.6: Verify /clear works**

```
>>> /clear
>>> what did I just ask you to do?
```

Expected: model has no memory; treats this as the start of conversation.

- [ ] **Step 20.7: Verify sessions list**

```bash
./aiwithtools sessions
```

Expected: at least one row with the model name.

If all six checks pass, the v1 acceptance criterion is met.

---

## Self-Review

Checked against the spec:

- ✅ `aiwithtools run <model>` with `:cloud` suffix — Task 17
- ✅ `--continue` / `--resume` / `--system` / `--max-iterations` flags — Task 17
- ✅ `sessions` / `sessions --model` / `sessions rm` / `sessions rm --all` — Task 18
- ✅ `/clear`, `/exit`, `/bye`, `/tools`, `/help` slash commands — Tasks 14–15
- ✅ Ollama HTTP via `ollama/api` — Task 8
- ✅ MCP host with stdio transport, prefixed registry, 15s startup budget, initialize handshake — Tasks 9–12
- ✅ ReAct loop with max-iter, tool-error serialization, Ctrl-C cancel via context — Task 13
- ✅ Display interface; terminal impl with inline `→`/`←` lines — Tasks 13, 15
- ✅ SQLite schema, file mode 0600, dir 0700, ULID IDs — Task 2
- ✅ Sessions: Create/Append*/Messages/List/LastForModel/Clear/Delete with `updated_at` invariant — Tasks 3–4
- ✅ Dangling tool_calls recovery covering partial completion — Task 5
- ✅ System prompt frozen at session creation; date suffix request-time-only with multi-day note — Tasks 6, 17
- ✅ MCP content flattening (text join, image placeholder) — Task 10
- ✅ Path expansion (~/$HOME/$USER) — Task 9
- ✅ Tool name collision via `server__` prefix — Task 12
- ✅ In-repo Go fake server for integration tests — Task 11
- ✅ Manual smoke against `nemotron-3-nano:30b-cloud` — Task 20
- ✅ Cloud auth error path documented (README + smoke prerequisites) — Tasks 19–20

No gaps identified.
