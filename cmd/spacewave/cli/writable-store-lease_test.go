//go:build !js && !wasip1

package spacewave_cli

import (
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestAcquireStatePathLeaseIgnoresStaleWriterPID reproduces a sidecar whose
// recorded writer PID now belongs to an unrelated live process.
func TestAcquireStatePathLeaseIgnoresStaleWriterPID(t *testing.T) {
	// Keep an unrelated process alive while reproducing its recycled PID.
	holder, _ := startStatePathLeaseHolder(t, t.TempDir())
	statePath := t.TempDir()
	storePath := filepath.Join(statePath, "stale.s4wave")
	if err := os.WriteFile(storePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// Seed the bbolt v1 sidecar left by a crashed writer with the live PID.
	data := make([]byte, 192)
	binary.LittleEndian.PutUint32(data[0:4], 0xBB01D100)
	binary.LittleEndian.PutUint32(data[4:8], 1)
	binary.LittleEndian.PutUint32(data[8:12], 1)
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[128:132], uint32(holder.Process.Pid))
	if err := os.WriteFile(storePath+"-lock", data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Acquire the state path through its kernel lease despite the stale sidecar.
	lease, err := acquireStatePathLease(t.Context(), statePath, false)
	if err != nil {
		t.Fatalf("acquire state path with recycled writer PID: %v", err)
	}
	t.Cleanup(func() {
		if err := lease.release(); err != nil {
			t.Errorf("release state path lease: %v", err)
		}
	})
}

// TestAcquireStatePathLeaseAfterProcessExit verifies that the OS relinquishes
// the runtime lease when a holder dies without running its cleanup.
func TestAcquireStatePathLeaseAfterProcessExit(t *testing.T) {
	// Wait for another process to hold the runtime lease before killing it.
	statePath := t.TempDir()
	holder, storePath := startStatePathLeaseHolder(t, statePath)
	if err := holder.Process.Kill(); err != nil {
		t.Fatalf("kill lease holder: %v", err)
	}
	var exitErr *exec.ExitError
	if err := holder.Wait(); !errors.As(err, &exitErr) {
		t.Fatalf("join killed lease holder: %v", err)
	}

	// Confirm the crash left sidecar metadata instead of deleting the lock file.
	if _, err := os.Stat(storePath + "-lock"); err != nil {
		t.Fatalf("crashed holder's sidecar: %v", err)
	}

	// Reacquire immediately after the process exit event without repairing files.
	lease, err := acquireStatePathLease(t.Context(), statePath, false)
	if err != nil {
		t.Fatalf("acquire state path after holder exit: %v", err)
	}
	t.Cleanup(func() {
		if err := lease.release(); err != nil {
			t.Errorf("release replacement lease: %v", err)
		}
	})
}
