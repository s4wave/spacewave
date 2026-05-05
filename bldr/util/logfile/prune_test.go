package logfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeFile writes name under dir with mtime set to mtime. It fails the
// test on any I/O error.
func writeFile(t *testing.T, dir, name string, mtime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("entry\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
	return path
}

func TestPruneOldLogs_DeletesAged(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	maxAge := 7 * 24 * time.Hour

	old := writeFile(t, dir, "20260101-000000.log", now.Add(-30*24*time.Hour))
	young := writeFile(t, dir, "20260503-000000.log", now.Add(-24*time.Hour))
	nonLog := writeFile(t, dir, "old.txt", now.Add(-30*24*time.Hour))

	removed, err := PruneOldLogs(dir, maxAge, now)
	if err != nil {
		t.Fatalf("PruneOldLogs: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old log should be removed: stat err = %v", err)
	}
	if _, err := os.Stat(young); err != nil {
		t.Errorf("young log should remain: %v", err)
	}
	if _, err := os.Stat(nonLog); err != nil {
		t.Errorf("non-.log file should remain: %v", err)
	}
}

func TestPruneOldLogs_KeepsAtCutoff(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	maxAge := 7 * 24 * time.Hour

	// File whose mtime is exactly at the cutoff is kept (Before is strict).
	atCutoff := writeFile(t, dir, "edge.log", now.Add(-maxAge))

	removed, err := PruneOldLogs(dir, maxAge, now)
	if err != nil {
		t.Fatalf("PruneOldLogs: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0 at cutoff", removed)
	}
	if _, err := os.Stat(atCutoff); err != nil {
		t.Errorf("at-cutoff log should remain: %v", err)
	}
}

func TestPruneOldLogs_NonExistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	removed, err := PruneOldLogs(dir, time.Hour, time.Now())
	if err != nil {
		t.Errorf("non-existent dir should be no-op, got err = %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
}

func TestPruneOldLogs_SkipsSubdirs(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	maxAge := 7 * 24 * time.Hour

	subdir := filepath.Join(dir, "old.log")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chtimes(subdir, now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour)); err != nil {
		t.Fatalf("chtimes subdir: %v", err)
	}

	removed, err := PruneOldLogs(dir, maxAge, now)
	if err != nil {
		t.Fatalf("PruneOldLogs: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
	if _, err := os.Stat(subdir); err != nil {
		t.Errorf("subdir named *.log should remain: %v", err)
	}
}

func TestPruneOldLogs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	removed, err := PruneOldLogs(dir, time.Hour, time.Now())
	if err != nil {
		t.Errorf("empty dir: err = %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
}
