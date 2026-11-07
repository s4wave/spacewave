package provider_local_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
)

// joinCheckpointEdits is the number of objects the owner creates before the
// writer joins, enough for the owner to checkpoint and trim them.
const joinCheckpointEdits = sobject.MinCheckpointOperations + 2

// TestJoinFromCheckpoint has an owner edit a Space until it checkpoints and
// trims the history, then has a writer join. The writer starts from the
// checkpoint, receives only the operations above it, reaches the owner's World
// and reports no wrong checkpoint. Both members then edit and the owner
// checkpoints again: each Session joined beside its storage identity, which
// writes for it, so the trimming roster holds only the storage identities.
func TestJoinFromCheckpoint(t *testing.T) {
	// Start one daemon for each member.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	ownerTb, _, ownerAccount, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseOwner)
	memberTb, _, memberAccount, memberSession, releaseMember := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseMember)

	// Run a World engine on each.
	ownerTb.StaticResolver.AddFactory(space_world.NewFactory(ownerTb.Bus, space_world.OpTypes{}))
	memberTb.StaticResolver.AddFactory(space_world.NewFactory(memberTb.Bus, space_world.OpTypes{}))

	// The owner creates and mounts the Space.
	ref, err := ownerAccount.CreateSharedObject(ctx, "join-space", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mounted, releaseObject, err := ownerAccount.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseObject)
	ownerObject := mounted.(*provider_local.SharedObject)

	// The owner edits it alone.
	owner := &offlineMember{t: t, name: "owner", tb: ownerTb, account: ownerAccount, object: ownerObject, ref: ref}
	owner.start(ctx)
	t.Cleanup(owner.stop)
	keys := make([]string, joinCheckpointEdits)
	for i := range keys {
		keys[i] = fmt.Sprintf("object-%02d", i)
		owner.create(ctx, keys[i])
	}

	// The owner checkpoints the history it alone has built on.
	height, held := waitCheckpoint(ctx, t, ownerObject, 0)
	if held >= joinCheckpointEdits {
		t.Fatalf("owner holds %d operations after its checkpoint; want fewer than %d", held, joinCheckpointEdits)
	}

	// A writer joins from the checkpoint and holds only the operations above
	// it.
	memberObject, memberRef := joinWriter(ctx, t, ownerAccount, ownerSession, ownerObject, memberAccount, memberSession)
	member := &offlineMember{t: t, name: "member", tb: memberTb, account: memberAccount, object: memberObject, ref: memberRef}
	member.start(ctx)
	t.Cleanup(member.stop)
	if _, got := waitCheckpoint(ctx, t, memberObject, 0); got >= joinCheckpointEdits {
		t.Fatalf("member holds %d operations; want fewer than %d", got, joinCheckpointEdits)
	}

	// It reaches the owner's World with every object, and reports no wrong
	// checkpoint.
	member.requireObjects(ctx, keys...)
	member.waitRoot(ctx, owner.root(ctx))
	if mismatch := member.health(ctx).GetCheckpointMismatch(); mismatch != nil {
		t.Fatalf("member reported a wrong checkpoint %v", mismatch)
	}

	// Both members build on each other's edits, and the owner checkpoints
	// them under a roster of the two storage identities.
	for i := range joinCheckpointEdits {
		key := fmt.Sprintf("after-join-%02d", i)
		owner.create(ctx, key)
		keys = append(keys, key)
	}
	member.requireObjects(ctx, keys...)
	member.create(ctx, "member-edit")
	owner.requireObjects(ctx, "member-edit")
	owner.create(ctx, "owner-edit")
	waitCheckpoint(ctx, t, ownerObject, height)
	state, err := ownerObject.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ownerObject.GetPeerID().String(), memberObject.GetPeerID().String()}
	if roster := state.GetConfig().TrimRoster(); !slices.Equal(roster, want) {
		t.Fatalf("trimming roster %v; want the storage identities %v", roster, want)
	}
}

// waitCheckpoint waits until the Space holds a checkpoint above height and
// returns its height and the number of operations held above it. On timeout
// it reports how much of the order the trimming roster leaves stable.
func waitCheckpoint(ctx context.Context, t *testing.T, object *provider_local.SharedObject, height uint64) (uint64, int) {
	// Watch the readable state.
	t.Helper()
	states, release, err := object.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Wait for a checkpoint above height.
	var roster []string
	var stable, held int
	var next uint64
	_, err = states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		checkpoint, err := snapshot.GetCheckpoint(ctx)
		if err != nil {
			return false, err
		}
		cfg, err := snapshot.GetConfig(ctx)
		if err != nil {
			return false, err
		}
		set, err := snapshot.GetOperationSet(ctx)
		if err != nil {
			return false, err
		}
		roster = cfg.TrimRoster()
		stable, held, next = len(set.StablePoint(roster)), set.Len(), checkpoint.GetHeight()
		return next > height, nil
	}, nil)
	if err != nil {
		t.Fatalf("no checkpoint above height %d: %d of %d operations stable under roster %v: %v", height, stable, held, roster, err)
	}
	return next, held
}
