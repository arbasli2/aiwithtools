package session

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
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
		id, model, system, now.UnixNano(), now.UnixNano(),
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
	if err != nil {
		return 0, err
	}
	if !seq.Valid {
		return 0, nil
	}
	return int(seq.Int64) + 1, nil
}

func (s *Session) appendRaw(role, content, toolName string, toolCalls []ToolCall) error {
	seq, err := s.nextSeq()
	if err != nil {
		return fmt.Errorf("next seq: %w", err)
	}

	var tcJSON sql.NullString
	if len(toolCalls) > 0 {
		b, err := json.Marshal(toolCalls)
		if err != nil {
			return fmt.Errorf("marshal tool_calls: %w", err)
		}
		tcJSON = sql.NullString{String: string(b), Valid: true}
	}
	var tn sql.NullString
	if toolName != "" {
		tn = sql.NullString{String: toolName, Valid: true}
	}

	now := time.Now()
	tx, err := s.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO messages(session_id, seq, role, content, tool_calls, tool_name, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.ID, seq, role, content, tcJSON, tn, now.UnixNano(),
	); err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	if _, err := tx.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`, now.UnixNano(), s.ID); err != nil {
		return fmt.Errorf("bump updated_at: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}

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

func (s *Store) Load(id string) (*Session, error) {
	var sess Session
	var createdAt, updatedAt int64
	err := s.db.QueryRow(
		`SELECT id, model, system, created_at, updated_at FROM sessions WHERE id = ?`, id,
	).Scan(&sess.ID, &sess.Model, &sess.System, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session %q not found", id)
	}
	if err != nil {
		return nil, err
	}
	sess.CreatedAt = time.Unix(0, createdAt)
	sess.UpdatedAt = time.Unix(0, updatedAt)
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
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}

	lastAssistant := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			lastAssistant = i
			break
		}
	}
	if lastAssistant == -1 {
		return nil
	}
	asst := msgs[lastAssistant]
	if len(asst.ToolCalls) == 0 {
		return nil
	}

	following := 0
	for _, m := range msgs[lastAssistant+1:] {
		if m.Role == "tool" {
			following++
		}
	}
	if following >= len(asst.ToolCalls) {
		return nil
	}

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

func (s *Session) Messages() ([]Message, error) {
	rows, err := s.store.db.Query(
		`SELECT role, content, tool_calls, tool_name FROM messages WHERE session_id = ? ORDER BY seq ASC`,
		s.ID,
	)
	if err != nil {
		return nil, err
	}
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
		if tn.Valid {
			m.ToolName = tn.String
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
