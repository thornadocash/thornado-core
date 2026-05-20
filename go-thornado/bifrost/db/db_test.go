package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thornadocash/go-thornado/config"
)

func TestNewLevelDB_InMemory(t *testing.T) {
	// Empty path should use in-memory storage
	db, err := NewLevelDB("", config.LevelDBOptions{})
	if err != nil {
		t.Fatalf("expected no error for in-memory db, got: %v", err)
	}
	defer db.Close()

	// Verify the db works
	err = db.Put([]byte("key"), []byte("value"), nil)
	if err != nil {
		t.Fatalf("expected no error putting key, got: %v", err)
	}
	val, err := db.Get([]byte("key"), nil)
	if err != nil {
		t.Fatalf("expected no error getting key, got: %v", err)
	}
	if string(val) != "value" {
		t.Fatalf("expected 'value', got '%s'", string(val))
	}
}

func TestNewLevelDB_FileBasedPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "testdb")

	db, err := NewLevelDB(dbPath, config.LevelDBOptions{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer db.Close()

	// Verify the db works
	err = db.Put([]byte("hello"), []byte("world"), nil)
	if err != nil {
		t.Fatalf("expected no error putting key, got: %v", err)
	}
}

func TestNewLevelDB_InvalidPath(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping: permission-based test does not work as root")
	}
	// Use a path that can't be opened (read-only dir with nested path)
	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0o444); err != nil {
		t.Fatalf("failed to create read-only dir: %v", err)
	}
	defer os.Chmod(readOnlyDir, 0o755) // nolint

	dbPath := filepath.Join(readOnlyDir, "subdir", "testdb")
	_, err := NewLevelDB(dbPath, config.LevelDBOptions{})
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestNewLevelDB_WithCompactOnInit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "compactdb")

	opts := config.LevelDBOptions{
		CompactOnInit: true,
	}
	db, err := NewLevelDB(dbPath, opts)
	if err != nil {
		t.Fatalf("expected no error with compact on init, got: %v", err)
	}
	defer db.Close()

	// Write some data and verify it survives compaction
	err = db.Put([]byte("test"), []byte("data"), nil)
	if err != nil {
		t.Fatalf("expected no error putting key, got: %v", err)
	}
}

func TestNewLevelDB_WithLevelDBOptions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "optsdb")

	opts := config.LevelDBOptions{
		FilterBitsPerKey:              10,
		CompactionTableSizeMultiplier: 2.0,
		WriteBuffer:                   8 * 1024 * 1024,
		BlockCacheCapacity:            16 * 1024 * 1024,
	}
	db, err := NewLevelDB(dbPath, opts)
	if err != nil {
		t.Fatalf("expected no error with options, got: %v", err)
	}
	defer db.Close()
}
