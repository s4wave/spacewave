package device_policy

import (
	"context"
	"os"
	"testing"
	"time"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

func TestPolicyStoreMissingFileLoadsEmptySnapshot(t *testing.T) {
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
	stateRoot := t.TempDir()
	want := &DevicePolicy{
		Revision: 7,
		RemoteShell: &RemoteShellPolicy{
			Enabled: true,
			Detail:  "terminal enabled by local policy",
		},
		CheckoutRoot: []*CheckoutRootPolicy{{
			Name:      "skiffos",
			LocalPath: "/work/skiffos",
			Access:    s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE,
		}},
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
	stateRoot := t.TempDir()
	initial := &DevicePolicy{Revision: 1}
	if err := WriteFile(stateRoot, initial); err != nil {
		t.Fatalf("write initial policy: %v", err)
	}
	store, err := NewPolicyStore(stateRoot)
	if err != nil {
		t.Fatalf("NewPolicyStore() error = %v", err)
	}

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

	next := &DevicePolicy{
		Revision:    2,
		RemoteShell: &RemoteShellPolicy{Enabled: true},
	}
	if err := WriteFile(stateRoot, next); err != nil {
		t.Fatalf("write next policy: %v", err)
	}
	if err := store.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

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
	stateRoot := t.TempDir()
	want := &DevicePolicy{
		Revision:    11,
		RemoteShell: &RemoteShellPolicy{Enabled: true},
	}
	if err := WriteFile(stateRoot, want); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	store, err := NewPolicyStore(stateRoot)
	if err != nil {
		t.Fatalf("NewPolicyStore() error = %v", err)
	}
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
