package provider_local_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
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

// TestCheckpointCadence writes edits on a Space its owner orders, checkpointing
// at the count only and with the deadline. It checks that a World is stored at
// most once per MinCheckpointOperations edits plus the deadline's last one,
// that a lone edit is checkpointed at the deadline, and logs the page bytes
// per edit by tree and the restart time with the edits above the checkpoint.
func TestCheckpointCadence(t *testing.T) {
	t.Run("count-only", func(t *testing.T) { checkpointCadence(t, true) })
	t.Run("deadline", func(t *testing.T) { checkpointCadence(t, false) })
}

// checkpointCadence runs TestCheckpointCadence in one checkpoint mode.
func checkpointCadence(t *testing.T, countOnly bool) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	// Start a daemon whose provider checkpoints in the mode.
	tb, _, account, _, release := setupProviderAndSession(ctx, t)
	t.Cleanup(release)
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	prov.(*provider_local.Provider).SetCheckpointCountOnly(countOnly)
	provRef.Release()
	tb.StaticResolver.AddFactory(space_world.NewEngineFactory(tb.Bus, space_world.OpTypes{}))

	// The owner creates the Space and orders its operations.
	ref, err := account.CreateSharedObject(ctx, "cadence-space", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mounted, releaseObject, err := account.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseObject)
	object := mounted.(*provider_local.SharedObject)
	owner := &offlineMember{t: t, name: "owner", tb: tb, account: account, object: object, ref: ref}
	owner.start(ctx)
	t.Cleanup(owner.stop)
	if _, err := object.SetSequencer(ctx, object.GetPeerID().String()); err != nil {
		t.Fatal(err)
	}
	owner.waitSequencer(ctx, object.GetPeerID().String())

	// Write the edits, and wait for the order to settle: every edit above the
	// checkpoint stable, none at the deadline, fewer than the count at the
	// count.
	const edits = 4 * sobject.MinCheckpointOperations
	above := 1
	if countOnly {
		above = sobject.MinCheckpointOperations
	}
	before := world_block.StoredPages.Snapshot()
	for i := range edits {
		owner.create(ctx, "edit-"+strconv.Itoa(i))
	}
	owner.waitPlaced(ctx)
	checkpoints, left := owner.waitSettled(ctx, above)

	// Count the stored Worlds and the page bytes by tree.
	byTree := make(map[world_block.PageTree]int64)
	var worlds, total int64
	for key, n := range world_block.StoredPages.Snapshot() {
		n.Pages -= before[key].Pages
		n.Bytes -= before[key].Bytes
		byTree[key.Tree] += n.Bytes
		total += n.Bytes
		if key.Tree == world_block.PageTreeRoot {
			worlds += int64(n.Pages)
		}
	}
	t.Logf("%d edits: %d checkpoints, %d edits above, %d Worlds stored, %d page bytes per edit (object %d, graph %d, changelog %d, root %d)",
		edits, checkpoints, left, worlds, total/edits,
		byTree[world_block.PageTreeObject]/edits, byTree[world_block.PageTreeGraph]/edits,
		byTree[world_block.PageTreeChangeLog]/edits, byTree[world_block.PageTreeRoot]/edits)
	limit := uint64(edits / sobject.MinCheckpointOperations)
	if !countOnly {
		limit++
	}
	if checkpoints > limit {
		t.Fatalf("%d checkpoints for %d edits, want at most %d", checkpoints, edits, limit)
	}

	// A lone edit is checkpointed at the deadline.
	if !countOnly {
		start := time.Now()
		owner.create(ctx, "lone")
		owner.waitPlaced(ctx)
		owner.waitSettled(ctx, 1)
		t.Logf("lone edit checkpointed after %v", time.Since(start).Round(time.Millisecond))
	}

	// Restart with the edits above the checkpoint, and time the World's
	// return.
	root := owner.root(ctx)
	owner.stop()
	start := time.Now()
	owner.start(ctx)
	owner.waitRoot(ctx, root)
	t.Logf("restart with %d edits above the checkpoint showed the World after %v", left, time.Since(start).Round(time.Millisecond))
}

// waitSettled waits until every operation above the state's checkpoint is
// stable and fewer than above of them remain, so no checkpoint is owed. It
// returns the checkpoint's height, which counts the checkpoints, and the
// operations above it.
func (m *offlineMember) waitSettled(ctx context.Context, above int) (uint64, int) {
	// Watch the readable state.
	t := m.t
	t.Helper()
	states, release, err := m.object.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	var ev *sobject.OrderEvent
	if _, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		var err error
		ev, err = sobject.ReadOrder(ctx, snapshot)
		if err != nil {
			return false, err
		}
		return ev.Stable == len(ev.Order) && ev.Stable < above, nil
	}, nil); err != nil {
		t.Fatalf("%s: order did not settle below %d: %v", m.name, above, err)
	}
	return ev.Checkpoint.GetHeight(), ev.Stable
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
