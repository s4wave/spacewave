package provider_local_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
)

// joinCheckpointEdits is the number of objects the owner creates before the
// writer joins, enough for the owner to checkpoint and trim them.
const joinCheckpointEdits = sobject.MinCheckpointOperations + 2

// TestJoinFromCheckpoint has an owner edit a Space until it checkpoints and
// trims the history, then has a writer join. The writer starts from the
// checkpoint, receives only the operations above it, reaches the owner's World
// and reports no wrong checkpoint.
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
	ownerTb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(ownerTb.Bus))
	memberTb.StaticResolver.AddFactory(sobject_world_engine.NewFactory(memberTb.Bus))

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
	held := waitCheckpoint(ctx, t, ownerObject)
	if held >= joinCheckpointEdits {
		t.Fatalf("owner holds %d operations after its checkpoint; want fewer than %d", held, joinCheckpointEdits)
	}

	// A writer joins from the checkpoint and holds only the operations above
	// it.
	memberObject, memberRef := joinWriter(ctx, t, ownerAccount, ownerSession, ownerObject, memberAccount, memberSession)
	member := &offlineMember{t: t, name: "member", tb: memberTb, account: memberAccount, object: memberObject, ref: memberRef}
	member.start(ctx)
	t.Cleanup(member.stop)
	if got := waitCheckpoint(ctx, t, memberObject); got >= joinCheckpointEdits {
		t.Fatalf("member holds %d operations; want fewer than %d", got, joinCheckpointEdits)
	}

	// It reaches the owner's World with every object, and reports no wrong
	// checkpoint.
	member.requireObjects(ctx, keys...)
	if ownerRoot, memberRoot := owner.root(ctx), member.root(ctx); !memberRoot.EqualVT(ownerRoot) {
		t.Fatalf("member World root %v; want the owner's %v", memberRoot, ownerRoot)
	}
	if mismatch := member.health(ctx).GetCheckpointMismatch(); mismatch != nil {
		t.Fatalf("member reported a wrong checkpoint %v", mismatch)
	}
}

// waitCheckpoint waits until the Space holds a checkpoint above genesis and
// returns the number of operations held above it.
func waitCheckpoint(ctx context.Context, t *testing.T, object *provider_local.SharedObject) int {
	// Watch the readable state.
	t.Helper()
	states, release, err := object.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Wait for a checkpoint above genesis.
	snapshot, err := states.WaitValueWithValidator(ctx, func(snapshot sobject.SharedObjectStateSnapshot) (bool, error) {
		if snapshot == nil {
			return false, nil
		}
		checkpoint, err := snapshot.GetCheckpoint(ctx)
		return checkpoint.GetHeight() != 0, err
	}, nil)
	if err != nil {
		t.Fatalf("no checkpoint above genesis: %v", err)
	}
	set, err := snapshot.GetOperationSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return set.Len()
}
