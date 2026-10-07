package device_policy

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPolicyStoreMissingFileLoadsEmptySnapshot(t *testing.T) {
	// Verify a missing policy file yields an empty snapshot without creating a file.
	stateRoot := t.TempDir()
	store, err := NewPolicyStore(stateRoot)
	if err != nil {
		t.Fatalf("NewPolicyStore() error = %v", err)
	}
	if !store.Snapshot().EqualVT(&DevicePolicy{}) {
		t.Fatalf("snapshot = %v, want empty policy for missing file", store.Snapshot())
	}
	if _, err := os.Stat(FilePath(stateRoot)); !os.IsNotExist(err) {
		t.Fatalf("policy file stat error = %v, want missing file to remain absent", err)
	}
}

func TestPolicyFileWriteReadRoundTripGeneratedJSON(t *testing.T) {
	// Write and read a device policy containing shell and node type settings.
	stateRoot := t.TempDir()
	want := &DevicePolicy{
		Revision:   7,
		NodeTypeId: []string{"tcp-port", "remote-shell"},
	}
	if err := WriteFile(stateRoot, want); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	got, err := ReadFile(stateRoot)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !got.EqualVT(want) {
		t.Fatalf("ReadFile() = %v, want %v", got, want)
	}
}

func TestPolicyStoreReloadBroadcastsNewSnapshot(t *testing.T) {
	// Initialize the policy store with its first saved revision.
	stateRoot := t.TempDir()
	initial := &DevicePolicy{Revision: 1}
	if err := WriteFile(stateRoot, initial); err != nil {
		t.Fatalf("write initial policy: %v", err)
	}
	store, err := NewPolicyStore(stateRoot)
	if err != nil {
		t.Fatalf("NewPolicyStore() error = %v", err)
	}

	// Start a watcher awaiting a policy snapshot different from the initial revision.
	ctx := t.Context()
	changed := make(chan *DevicePolicy, 1)
	errs := make(chan error, 1)
	go func() {
		policy, err := store.WaitChange(ctx, initial)
		if err != nil {
			errs <- err
			return
		}
		changed <- policy
	}()

	// Save and reload the next device-policy revision.
	next := &DevicePolicy{
		Revision:   2,
		NodeTypeId: []string{"remote-shell"},
	}
	if err := WriteFile(stateRoot, next); err != nil {
		t.Fatalf("write next policy: %v", err)
	}
	if err := store.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	// Verify reload broadcasts the next snapshot and updates the current state.
	select {
	case err := <-errs:
		t.Fatalf("WaitChange() error = %v", err)
	case got := <-changed:
		if !got.EqualVT(next) {
			t.Fatalf("WaitChange() = %v, want %v", got, next)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitChange() did not receive reload broadcast")
	}
	if !store.Snapshot().EqualVT(next) {
		t.Fatalf("Snapshot() after Reload = %v, want %v", store.Snapshot(), next)
	}
}

func TestPolicyStoreWaitChangeReturnsCurrentWithoutPolling(t *testing.T) {
	// Prepare a saved current policy for a canceled-context read.
	stateRoot := t.TempDir()
	want := &DevicePolicy{
		Revision:   11,
		NodeTypeId: []string{"remote-shell"},
	}
	if err := WriteFile(stateRoot, want); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	store, err := NewPolicyStore(stateRoot)
	if err != nil {
		t.Fatalf("NewPolicyStore() error = %v", err)
	}

	// Verify WaitChange returns the current policy even when the caller is canceled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := store.WaitChange(ctx, nil)
	if err != nil {
		t.Fatalf("WaitChange() error = %v", err)
	}
	if !got.EqualVT(want) {
		t.Fatalf("WaitChange() = %v, want current policy %v", got, want)
	}
}
