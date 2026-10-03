package provider_local_test

import (
	"context"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
)

// TestMainDeviceOffline has the owner's device order a local Space as its main
// device, then go offline. A writer's edit still applies to its World at once
// and shows as waiting to be put in order. When the main device returns it
// places the edit, the count clears and both members reach the same World.
func TestMainDeviceOffline(t *testing.T) {
	// Full peer sync runs natively under one deadline.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	// Start one daemon with both members' accounts and a World engine.
	tb, _, ownerAccount, ownerSession, release := setupProviderAndSession(ctx, t)
	t.Cleanup(release)
	memberAccount, memberSession := addLocalSession(ctx, t, tb)
	tb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tb.Bus))

	// The owner creates the Space and a writer joins it.
	ref, err := ownerAccount.CreateSharedObject(ctx, "main-device-space", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mounted, releaseObject, err := ownerAccount.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseObject)
	ownerObject := mounted.(*provider_local.SharedObject)
	memberObject, memberRef := joinWriter(ctx, t, ownerAccount, ownerSession, ownerObject, memberAccount, memberSession)

	// Both members replay the Space, and the owner's device becomes the main
	// device and places every edit so far.
	owner := &offlineMember{t: t, name: "owner", tb: tb, account: ownerAccount, object: ownerObject, ref: ref}
	member := &offlineMember{t: t, name: "member", tb: tb, account: memberAccount, object: memberObject, ref: memberRef}
	for _, m := range []*offlineMember{owner, member} {
		m.start(ctx)
		t.Cleanup(m.stop)
	}
	if _, err := ownerObject.SetSequencer(ctx, ownerObject.GetPeerID().String()); err != nil {
		t.Fatal(err)
	}
	member.waitSequencer(ctx, ownerObject.GetPeerID().String())
	member.waitUnordered(ctx, 0)

	// The main device goes offline. The writer's edit applies at once and
	// waits to be put in order.
	ownerAccount.StopP2PSync()
	member.create(ctx, "while-main-offline")
	member.requireObjects(ctx, "while-main-offline")
	member.waitUnordered(ctx, 1)

	// The main device returns and places the edit.
	if err := ownerAccount.StartPersistentP2PSync(ctx, ownerAccount.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	member.waitUnordered(ctx, 0)
	owner.waitOperations(ctx, memberObject.GetPeerID().String(), 1)
	ownerRoot := owner.root(ctx)
	if memberRoot := member.root(ctx); !memberRoot.EqualVT(ownerRoot) {
		t.Fatalf("member World root %v; want the owner's %v", memberRoot, ownerRoot)
	}
	owner.requireObjects(ctx, "while-main-offline")
}

// waitSequencer waits until the Space's config appoints peerID.
func (m *offlineMember) waitSequencer(ctx context.Context, peerID string) {
	// Watch the readable state.
	t := m.t
	t.Helper()
	states, release, err := m.object.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Read the sequencer of each state's config.
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		cfg, err := snapshot.GetConfig(ctx)
		if err != nil {
			return false, err
		}
		return cfg.GetSequencer().GetPeerId() == peerID, nil
	}, nil); err != nil {
		t.Fatalf("%s did not see sequencer %s: %v", m.name, peerID, err)
	}
}

// waitUnordered waits until the health counts n operations waiting for the
// sequencer.
func (m *offlineMember) waitUnordered(ctx context.Context, n uint32) {
	// Replay the latest state so the World engine reports its count.
	t := m.t
	t.Helper()
	m.sync(ctx)

	// Watch the health.
	healths, release, err := m.object.AccessSharedObjectHealth(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := healths.WaitValueWithValidator(ctx, func(health *sobject.SharedObjectHealth) (bool, error) {
		return health.GetUnorderedCount() == n, nil
	}, nil); err != nil {
		t.Fatalf("%s health did not reach %d unordered operations: %v", m.name, n, err)
	}
}
