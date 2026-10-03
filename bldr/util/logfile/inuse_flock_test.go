//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package logfile

import (
	"os"
	"testing"
	"time"
)

// TestPruneOldLogs_SkipsInUse keeps a log held open by a writer even when it
// falls outside the keep count and retention window.
func TestPruneOldLogs_SkipsInUse(t *testing.T) {
	// Create the log directory and retention reference time.
	dir := t.TempDir()
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

	// Write active, idle, and recent log fixtures.
	active := writeFile(t, dir, "20260101-000000.log", now.Add(-30*24*time.Hour))
	idle := writeFile(t, dir, "20260102-000000.log", now.Add(-29*24*time.Hour))
	writeFile(t, dir, "20260503-000000.log", now.Add(-time.Hour))

	// Open and mark the active log so retention preserves it.
	f, err := os.OpenFile(active, os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	markLogInUse(f)

	// Prune expired logs while the active log stays open.
	removed, err := PruneOldLogs(dir, 7*24*time.Hour, 1, now)
	if err != nil {
		t.Fatalf("PruneOldLogs: %v", err)
	}

	// Verify pruning removes the idle log and preserves the active log.
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(active); err != nil {
		t.Errorf("in-use log should remain: %v", err)
	}
	if _, err := os.Stat(idle); !os.IsNotExist(err) {
		t.Errorf("idle old log should be removed: stat err = %v", err)
	}
}
