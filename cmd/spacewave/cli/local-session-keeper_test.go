//go:build !js

package spacewave_cli

import (
	"context"
	"errors"
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	session "github.com/s4wave/spacewave/core/session"
	"github.com/sirupsen/logrus"
)

// testLocalSessionMount records release of a retained session resource.
type testLocalSessionMount struct {
	released bool
}

// Release records the keeper's session cleanup.
func (m *testLocalSessionMount) Release() {
	m.released = true
}

// TestReconcileLocalSessionMountsRetainsConfiguredSession checks mount reuse
// across snapshots and release after removal.
func TestReconcileLocalSessionMountsRetainsConfiguredSession(t *testing.T) {
	entry := &session.SessionListEntry{
		SessionIndex: 1,
		SessionRef: &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
			ProviderId: "local",
		}},
	}
	mount := &testLocalSessionMount{}
	mounted := make(map[uint32]localSessionMount)
	mountCalls := 0
	mountFunc := func(index uint32) (localSessionMount, error) {
		mountCalls++
		if index != 1 {
			t.Fatalf("mount index = %d, want 1", index)
		}
		return mount, nil
	}
	le := logrus.NewEntry(logrus.New())

	reconcileLocalSessionMounts(le, []*session.SessionListEntry{entry}, mounted, mountFunc)
	reconcileLocalSessionMounts(le, []*session.SessionListEntry{entry}, mounted, mountFunc)
	if mountCalls != 1 || mount.released {
		t.Fatalf("configured session calls=%d released=%v", mountCalls, mount.released)
	}

	reconcileLocalSessionMounts(le, nil, mounted, mountFunc)
	if !mount.released || len(mounted) != 0 {
		t.Fatalf("removed session released=%v mounted=%d", mount.released, len(mounted))
	}
}

// TestReconcileDeviceEnrollmentRestoresAndReleasesLocalSession covers the
// persisted local completion across session snapshots and keeper shutdown.
func TestReconcileDeviceEnrollmentRestoresAndReleasesLocalSession(t *testing.T) {
	statePath := t.TempDir()
	record := &deviceSetupRecord{
		SetupState: deviceSetupStateSessionReady, Completion: deviceLocalCompletionPrefix + "stub",
		SessionIndex: 3, DeviceObjectKey: "devices/key",
	}
	if err := writeDeviceSetupRecord(statePath, record); err != nil {
		t.Fatal(err)
	}
	oldMount := deviceMountLocalSession
	deviceMountLocalSession = func(_ context.Context, _ *sdkClient, _ string, current *deviceSetupRecord) (*deviceSetupRecord, error) {
		return current, nil
	}
	t.Cleanup(func() { deviceMountLocalSession = oldMount })

	mount := &testLocalSessionMount{}
	mountCalls := 0
	mountFunc := func(index uint32) (localSessionMount, error) {
		mountCalls++
		if index != 3 {
			t.Fatalf("mount index = %d, want 3", index)
		}
		return mount, nil
	}
	entries := []*session.SessionListEntry{{SessionIndex: 3}}
	le := logrus.NewEntry(logrus.New())
	var cleanup func()
	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, entries, mountFunc, &cleanup)
	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, entries, mountFunc, &cleanup)
	if cleanup == nil || mountCalls != 1 || mount.released {
		t.Fatalf("retained enrollment: cleanup=%v mounts=%d released=%v", cleanup != nil, mountCalls, mount.released)
	}
	cleanup()
	if !mount.released {
		t.Fatal("daemon shutdown did not release Device session")
	}
}

// TestReconcileDeviceEnrollmentRetriesPendingProjection checks that a failed
// World write remains pending until a later eligible session-list revision.
func TestReconcileDeviceEnrollmentRetriesPendingProjection(t *testing.T) {
	statePath := t.TempDir()
	if err := writeDeviceSetupRecord(statePath, &deviceSetupRecord{
		SetupState: deviceSetupStateImported, SessionIndex: 3,
	}); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	withDeviceObjectUpsertStub(t, func(context.Context, *sdkClient, string, *deviceSetupRecord) (string, error) {
		attempts++
		if attempts == 1 {
			return "", errors.New("base World root is stale")
		}
		return "devices/key", nil
	})
	mount := func(uint32) (localSessionMount, error) {
		t.Fatal("linked Device should not restore local enrollment")
		return nil, nil
	}
	le := logrus.NewEntry(logrus.New())
	var cleanup func()
	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, []*session.SessionListEntry{{SessionIndex: 4}}, mount, &cleanup)
	if attempts != 0 {
		t.Fatalf("projection before eligible revision: %d attempts", attempts)
	}

	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, []*session.SessionListEntry{{SessionIndex: 3}}, mount, &cleanup)
	pending, err := readDeviceSetupRecord(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || pending.SetupState != deviceSetupStateImported || pending.DeviceObjectKey != "" || pending.FailureReason == "" {
		t.Fatalf("pending projection: attempts=%d record=%+v", attempts, pending)
	}

	reconcileDeviceEnrollment(t.Context(), le, statePath, nil, []*session.SessionListEntry{{SessionIndex: 3}, {SessionIndex: 4}}, mount, &cleanup)
	ready, err := readDeviceSetupRecord(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || ready.SetupState != deviceSetupStateSessionReady || ready.DeviceObjectKey != "devices/key" || ready.FailureReason != "" {
		t.Fatalf("projected Device: attempts=%d record=%+v", attempts, ready)
	}
}
