package session

import (
	"database/sql"
	"fmt"
	"time"
)

type Summary struct {
	ID               string
	Model            string
	UpdatedAt        time.Time
	MessageCount     int
	FirstUserMessage string
}

func (s *Store) LastForModel(model string) (string, error) {
	var id string
	err := s.db.QueryRow(
		`SELECT id FROM sessions WHERE model = ? ORDER BY updated_at DESC LIMIT 1`,
		model,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("last for model: %w", err)
	}
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
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Summary
	for rows.Next() {
		var sum Summary
		var updated int64
		if err := rows.Scan(&sum.ID, &sum.Model, &updated, &sum.MessageCount, &sum.FirstUserMessage); err != nil {
			return nil, err
		}
		sum.UpdatedAt = time.Unix(0, updated)
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
