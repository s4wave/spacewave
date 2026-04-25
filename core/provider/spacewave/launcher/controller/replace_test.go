//go:build !js

package spacewave_launcher_controller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStageRawUpdateRelay(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "spacewave")
	stagedPath := filepath.Join(dir, "staged-spacewave")
	if err := os.WriteFile(execPath, []byte("OLD"), 0o755); err != nil {
		t.Fatalf("seed exec: %v", err)
	}
	if err := os.WriteFile(stagedPath, []byte("NEW"), 0o755); err != nil {
		t.Fatalf("seed staged: %v", err)
	}

	tmpPath, err := stageRawUpdateRelay(execPath, stagedPath)
	if err != nil {
		t.Fatalf("stageRawUpdateRelay: %v", err)
	}
	if tmpPath != execPath+".tmp" {
		t.Fatalf("tmp path = %q, want %q", tmpPath, execPath+".tmp")
	}

	got, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("read tmp: %v", err)
	}
	if string(got) != "NEW" {
		t.Fatalf("tmp content = %q, want NEW", string(got))
	}
	if _, err := os.Stat(stagedPath); !os.IsNotExist(err) {
		t.Fatalf("staged file should be removed, stat err = %v", err)
	}
}

func TestRawUpdateEnv(t *testing.T) {
	t.Setenv(rawUpdateRelayTargetEnv, "old-target")
	t.Setenv(rawUpdateRelayCleanupEnv, "old-cleanup")
	t.Setenv("KEEP_ME", "1")

	env := rawUpdateEnv(
		map[string]string{rawUpdateRelayTargetEnv: "new-target"},
		rawUpdateRelayCleanupEnv,
	)
	var sawTarget bool
	var sawCleanup bool
	var sawKeep bool
	for _, kv := range env {
		if kv == rawUpdateRelayTargetEnv+"=new-target" {
			sawTarget = true
		}
		if strings.HasPrefix(kv, rawUpdateRelayCleanupEnv+"=") {
			sawCleanup = true
		}
		if kv == "KEEP_ME=1" {
			sawKeep = true
		}
	}
	if !sawTarget {
		t.Fatal("expected new target env")
	}
	if sawCleanup {
		t.Fatal("cleanup env should be removed")
	}
	if !sawKeep {
		t.Fatal("unrelated env should be preserved")
	}
}
