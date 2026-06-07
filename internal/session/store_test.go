package session

import (
	"os"
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
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	s2.Close()
}

func TestOpen_FilePermIs0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("perm = %o, want 0600", perm)
	}
}
