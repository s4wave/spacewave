package provider_local_test

import (
	"context"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
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
	tb.StaticResolver.AddFactory(space_world.NewFactory(tb.Bus, space_world.OpTypes{}))

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

	// The owner's device becomes the main device, places every edit so far
	// and checkpoints the World holding them, which the writer installs.
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
	member.waitCheckpoint(ctx, owner.root(ctx))

	// The main device goes offline. The writer's edit applies at once and
	// waits to be put in order.
	ownerAccount.StopP2PSync()
	member.create(ctx, "while-main-offline")
	member.requireObjects(ctx, "while-main-offline")
	member.waitUnordered(ctx, 1)

	// The main device returns and places the edit, then checkpoints the World
	// holding it, which the writer installs.
	if err := ownerAccount.StartPersistentP2PSync(ctx, ownerAccount.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	member.waitUnordered(ctx, 0)
	owner.requireObjects(ctx, "while-main-offline")
	ownerRoot := owner.root(ctx)
	member.waitCheckpoint(ctx, ownerRoot)
	member.waitRoot(ctx, ownerRoot)
}

// TestFollowerInstallsLeaderCheckpoint has the owner's device lead a Space: it
// orders and checkpoints every edit. A writer with no edits of its own installs
// each checkpoint as its World, so after every owner edit the writer holds the
// World the checkpoint publishes, with no operation left above it to replay.
func TestFollowerInstallsLeaderCheckpoint(t *testing.T) {
	// Full peer sync runs natively under one deadline.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	// Start one daemon with both members' accounts and a World engine.
	tb, _, ownerAccount, ownerSession, release := setupProviderAndSession(ctx, t)
	t.Cleanup(release)
	memberAccount, memberSession := addLocalSession(ctx, t, tb)
	tb.StaticResolver.AddFactory(space_world.NewFactory(tb.Bus, space_world.OpTypes{}))

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

	// Each owner edit reaches the writer as a checkpoint it installs.
	var keys []string
	for _, key := range []string{"first", "second", "third"} {
		owner.create(ctx, key)
		keys = append(keys, key)
		root := owner.root(ctx)
		member.waitCheckpoint(ctx, root)
		member.waitRoot(ctx, root)
		member.requireObjects(ctx, keys...)
	}
}

// waitCheckpoint waits until the state the World engine replays holds a
// checkpoint of the World root with no edit above it. The durable state can
// lead the admitted state, so a durable checkpoint does not yet reach the
// engine.
func (m *offlineMember) waitCheckpoint(ctx context.Context, root *block.BlockRef) {
	// Watch the admitted state.
	t := m.t
	t.Helper()
	states, release, err := m.object.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Decode each checkpoint's World and look for edits above it.
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		// Read the checkpoint's World root.
		if snapshot == nil {
			return false, nil
		}
		checkpoint, err := snapshot.GetCheckpoint(ctx)
		if err != nil {
			return false, err
		}
		head := &space_world.InnerState{}
		if err := head.UnmarshalVT(checkpoint.GetStateData()); err != nil {
			return false, err
		}
		if !head.GetHeadRef().GetRootRef().EqualVT(root) {
			return false, nil
		}

		// Require that only acknowledgments remain above it.
		set, err := snapshot.GetOperationSet(ctx)
		if err != nil {
			return false, err
		}
		for _, h := range set.Order() {
			if !set.Get(h).IsAcknowledgment() {
				return false, nil
			}
		}
		return true, nil
	}, nil); err != nil {
		t.Fatalf("%s did not see a checkpoint of World %v: %v", m.name, root, err)
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
	if _, err := healths.WaitValueWithValidator(ctx, func(health *sobject.SharedObjectHealth) (bool, error) {
		return health.GetUnorderedCount() == n, nil
	}, nil); err != nil {
		t.Fatalf("%s health did not reach %d unordered operations: %v", m.name, n, err)
	}
}
