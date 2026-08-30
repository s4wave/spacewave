//go:build !js

package spacewave_cli

import (
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	session "github.com/s4wave/spacewave/core/session"
	"github.com/sirupsen/logrus"
)

type testLocalSessionMount struct {
	released bool
}

func (m *testLocalSessionMount) Release() {
	m.released = true
}

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
