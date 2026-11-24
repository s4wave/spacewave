package provider_local_test

import (
	"context"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
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
	tb.StaticResolver.AddFactory(space_world.NewEngineFactory(tb.Bus, space_world.OpTypes{}))

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

	// The owner's device becomes the main device and places every edit so
	// far. The writer holds every position and replays them to the owner's
	// World.
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
	member.waitPlaced(ctx)
	member.waitRoot(ctx, owner.root(ctx))

	// The main device goes offline. The writer's edit applies at once and
	// waits to be put in order.
	ownerAccount.StopP2PSync()
	member.create(ctx, "while-main-offline")
	member.requireObjects(ctx, "while-main-offline")
	member.waitUnordered(ctx, 1)

	// The main device returns and places the edit, and the writer replays the
	// order to the owner's World.
	if err := ownerAccount.StartPersistentP2PSync(ctx, ownerAccount.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	member.waitUnordered(ctx, 0)
	owner.requireObjects(ctx, "while-main-offline")
	member.waitRoot(ctx, owner.root(ctx))
}

// TestFollowerReplaysAboveCheckpoint has the owner's device lead a Space: it
// orders every edit and checkpoints only once enough are stable. A writer with
// no edits of its own replays each edit above the checkpoint to the owner's
// World. With the leader stopped before any checkpoint, a restarted writer
// still shows every edit.
func TestFollowerReplaysAboveCheckpoint(t *testing.T) {
	// Full peer sync runs natively under one deadline.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	// Start one daemon with both members' accounts and a World engine.
	tb, _, ownerAccount, ownerSession, release := setupProviderAndSession(ctx, t)
	t.Cleanup(release)
	memberAccount, memberSession := addLocalSession(ctx, t, tb)
	tb.StaticResolver.AddFactory(space_world.NewEngineFactory(tb.Bus, space_world.OpTypes{}))

	// The owner creates the Space and a writer joins it.
	ref, err := ownerAccount.CreateSharedObject(ctx, "follower-space", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
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

	// The owner's device becomes the leader.
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

	// Each owner edit reaches the writer as an operation it replays.
	var keys []string
	for _, key := range []string{"first", "second", "third"} {
		owner.create(ctx, key)
		keys = append(keys, key)
		member.waitRoot(ctx, owner.root(ctx))
		member.requireObjects(ctx, keys...)
	}
	root := owner.root(ctx)

	// The leader stops before any checkpoint covers the edits, and the
	// writer restarts: it replays them again from its state.
	ownerAccount.StopP2PSync()
	owner.stop()
	if h := member.checkpointHeight(ctx); h != 0 {
		t.Fatalf("the leader checkpointed %d edits at height %d", len(keys), h)
	}
	member.stop()
	member.start(ctx)
	member.waitRoot(ctx, root)
	member.requireObjects(ctx, keys...)
}

// checkpointHeight returns the height of the checkpoint the state holds.
func (m *offlineMember) checkpointHeight(ctx context.Context) uint64 {
	// Read the readable state.
	t := m.t
	t.Helper()
	states, release, err := m.object.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	snapshot, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		return snapshot != nil, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Read its checkpoint.
	checkpoint, err := snapshot.GetCheckpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint.GetHeight()
}

// waitPlaced waits until the appointed sequencer has placed every edit the
// member holds. It reads the state, since the health may still show a count
// from before the sequencer was appointed.
func (m *offlineMember) waitPlaced(ctx context.Context) {
	// Watch the readable state.
	t := m.t
	t.Helper()
	states, release, err := m.object.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		ev, err := sobject.ReadOrder(ctx, snapshot)
		if err != nil {
			return false, err
		}
		return ev.Config.GetSequencer() != nil && ev.Set.Unordered() == 0, nil
	}, nil); err != nil {
		t.Fatalf("%s did not receive every position: %v", m.name, err)
	}
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
	// Watch the health.
	t := m.t
	t.Helper()
	healths, release, err := m.object.AccessSharedObjectHealth(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	var last uint32
	if _, err := healths.WaitValueWithValidator(ctx, func(health *sobject.SharedObjectHealth) (bool, error) {
		last = health.GetUnorderedCount()
		return last == n, nil
	}, nil); err != nil {
		t.Fatalf("%s health did not reach %d unordered operations from %d: %v", m.name, n, last, err)
	}
}
