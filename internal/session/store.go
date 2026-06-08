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
	// We didn't write anything; ignore close error explicitly. The blank
	// assignment also satisfies CodeQL's go/unhandled-writable-file-close.
	_ = f.Close()
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
	if _, err := db.Exec(`INSERT OR IGNORE INTO schema_version(version) VALUES (?)`, schemaVersion); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed schema_version: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }
