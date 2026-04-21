//go:build !js && !bldr_sqlite

package storage_native

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBoltDBDeleteVolumeKeepsLockFile(t *testing.T) {
	rootDir := t.TempDir()
	store := &BoltDB{rootDir: rootDir}

	id := "test/volume"
	filename := "test_volume" + BoltDBExt
	dbPath := filepath.Join(rootDir, filename)
	lockPath := dbPath + "-lock"

	if err := os.WriteFile(dbPath, []byte("db"), 0o600); err != nil {
		t.Fatalf("write db file: %v", err)
	}
	if err := os.WriteFile(lockPath, []byte("lock"), 0o600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	if err := store.DeleteVolume(id); err != nil {
		t.Fatalf("DeleteVolume failed: %v", err)
	}

	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("expected db file to be removed, got err=%v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected lock file to remain, got err=%v", err)
	}
}
