package provider_local_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
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

// stalledEdits is the number of objects the owner creates while no checkpoint
// can land, enough for a replica to save its replay cursor, which it does
// after MinCheckpointOperations + 32 positions.
const stalledEdits = 2*sobject.MinCheckpointOperations + 2

// TestCheckpointOverSavedRefusal has the owner refuse an operation the member
// applies and save the refusal in the form an earlier build saved for a
// missing handler, then checkpoint over it. It measures what the member
// installing the checkpoint holds and reports.
func TestCheckpointOverSavedRefusal(t *testing.T) {
	// Start one daemon for each member, each running a World engine.
	skipFullP2PSyncUnderGoScript(t)
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Second)
	defer cancel()
	ownerTb, _, ownerAccount, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseOwner)
	memberTb, _, memberAccount, memberSession, releaseMember := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseMember)
	ownerTb.StaticResolver.AddFactory(space_world.NewFactory(ownerTb.Bus, space_world.OpTypes{}))
	memberTb.StaticResolver.AddFactory(space_world.NewFactory(memberTb.Bus, space_world.OpTypes{}))

	// The owner creates the Space and a writer joins it.
	ref, err := ownerAccount.CreateSharedObject(ctx, "refusal-space", &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
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
	owner := &offlineMember{t: t, name: "owner", tb: ownerTb, account: ownerAccount, object: ownerObject, ref: ref}
	member := &offlineMember{t: t, name: "member", tb: memberTb, account: memberAccount, object: memberObject, ref: memberRef}
	for _, m := range []*offlineMember{owner, member} {
		m.start(ctx)
		t.Cleanup(m.stop)
	}

	// The owner's handler fails the operation and the member's applies it.
	// The member creates "z" and runs the operation on it.
	releaseFailing := startCheckpointOp(ctx, t, owner.tb, func() world.Operation { return &failingCheckpointOp{} })
	defer startCheckpointOp(ctx, t, member.tb, func() world.Operation { return &checkpointOp{} })()
	member.create(ctx, "z")
	member.write(ctx, func(tx world.Tx) error {
		_, _, err := tx.ApplyWorldOp(ctx, &checkpointOp{MockWorldOp: world_mock.MockWorldOp{ObjectKey: "z", NextMsg: "op"}}, member.object.GetPeerID())
		return err
	})

	// The owner writes while the member writes nothing, so no operation
	// past the member's last one is stable and no checkpoint lands. The
	// positions pile up until the owner saves its replay cursor.
	keys := []string{"z"}
	for i := range stalledEdits {
		keys = append(keys, fmt.Sprintf("filler-%02d", i))
		owner.create(ctx, keys[len(keys)-1])
	}

	// The owner saves its refusal. With its engine stopped and its
	// handler gone, the refusal is rewritten as a missing handler's.
	var cursor *space_world.ReplayCursor
	owner.wait(ctx, func() error {
		cursor = readReplayCursor(ctx, t, owner.object)
		if refusedCount(cursor) == 0 {
			return errors.Errorf("%s saved no refusal; cursor outcomes %d, replay %v", owner.name, len(cursor.GetOutcomes()), owner.health(ctx).GetReplay())
		}
		return nil
	})
	owner.stop()
	releaseFailing()
	owner.wait(ctx, func() error {
		if owner.health(ctx).GetReplay() != nil {
			return errors.Errorf("%s body still runs", owner.name)
		}
		return nil
	})
	cursor = readReplayCursor(ctx, t, owner.object)
	rewritten := 0
	for _, outcome := range cursor.GetOutcomes() {
		if reason, ok := strings.CutSuffix(outcome.GetReason(), errCheckpointOpRefused.Error()); ok {
			outcome.Reason = reason + world.ErrUnhandledOp.Error()
			rewritten++
		}
	}
	if rewritten != 1 {
		t.Fatalf("%s saved %d refusals of the operation, want 1", owner.name, rewritten)
	}
	writeReplayCursor(ctx, t, owner.object, cursor)
	owner.start(ctx)

	// The member writes on top of the owner's edits, which makes them
	// stable, and the owner checkpoints over the refused operation.
	keys = append(keys, "member-after")
	member.create(ctx, "member-after")
	height, _ := waitCheckpoint(ctx, t, ownerObject, 0)
	waitCheckpoint(ctx, t, memberObject, height-1)
	ownerRoot := owner.root(ctx)
	reached := within(ctx, 10*time.Second, func() bool { return member.root(ctx).EqualVT(ownerRoot) })
	member.requireObjects(ctx, keys...)

	// Report each side.
	t.Logf("checkpoint height %d; member reached the owner's World: %v", height, reached)
	for _, m := range []*offlineMember{owner, member} {
		c := readReplayCursor(ctx, t, m.object)
		h := m.health(ctx)
		t.Logf("%s: op write present %v; checkpoint mismatch %v; replay %v; rejected edits %d; cursor height %d, outcomes %d, refused %d, seqnos %v, missing-handler reasons %d",
			m.name, hasCheckpointOpLink(ctx, t, m, "z"), h.GetCheckpointMismatch(), h.GetReplay(), len(h.GetRejectedEdits()),
			c.GetHeight(), len(c.GetOutcomes()), refusedCount(c), cursorSeqnos(c), unhandledCount(c))
	}
}

// within reports whether check succeeds before d passes.
func within(ctx context.Context, d time.Duration, check func() bool) bool {
	deadline := time.After(d)
	for !check() {
		select {
		case <-ctx.Done():
			return false
		case <-deadline:
			return false
		case <-time.After(20 * time.Millisecond):
		}
	}
	return true
}

// checkpointOpTypeID is the type of checkpointOp, which only a handler a test
// registers resolves.
const checkpointOpTypeID = "test/checkpoint-refusal-op"

// checkpointOp links its object to itself.
type checkpointOp struct {
	world_mock.MockWorldOp
}

// TypeID returns the operation type identifier.
func (*checkpointOp) TypeID() string {
	return checkpointOpTypeID
}

// ApplyWorldOp links the operation's object to itself.
func (o *checkpointOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, _ peer.ID) (bool, error) {
	key := o.GetObjectKey()
	return false, ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, checkpointOpTypeID, key, ""))
}

// errCheckpointOpRefused is the error failingCheckpointOp's apply fails with,
// of no kind replay waits out.
var errCheckpointOpRefused = errors.New("the handler refuses the operation")

// failingCheckpointOp is a checkpointOp whose apply fails.
type failingCheckpointOp struct {
	checkpointOp
}

// ApplyWorldOp fails.
func (*failingCheckpointOp) ApplyWorldOp(context.Context, *logrus.Entry, world.WorldState, peer.ID) (bool, error) {
	return false, errCheckpointOpRefused
}

// startCheckpointOp registers newOp as checkpointOp's handler on tb's bus for
// every engine, and returns the release that stops it.
func startCheckpointOp(ctx context.Context, t *testing.T, tb *testbed.Testbed, newOp func() world.Operation) func() {
	t.Helper()
	lookup := func(_ context.Context, opTypeID string) (world.Operation, error) {
		if opTypeID != checkpointOpTypeID {
			return nil, nil
		}
		return newOp(), nil
	}
	release, err := tb.Bus.AddController(ctx, world.NewLookupOpController("test-op-"+checkpointOpTypeID, "", lookup), nil)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

// hasCheckpointOpLink reports whether m's World holds checkpointOp's link on
// key.
func hasCheckpointOpLink(ctx context.Context, t *testing.T, m *offlineMember, key string) bool {
	t.Helper()
	tx, err := m.engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	quads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, checkpointOpTypeID, key, ""), 1)
	if err != nil {
		t.Fatal(err)
	}
	return len(quads) != 0
}

// replayCursorStore and replayCursorKey locate the replay cursor the Space
// world Adapter saves in a SharedObject's local state.
const replayCursorStore, replayCursorKey = "space-world-replay", "cursor"

// readReplayCursor returns the replay cursor saved for so, or nil.
func readReplayCursor(ctx context.Context, t *testing.T, so sobject.SharedObject) *space_world.ReplayCursor {
	t.Helper()
	store, release, err := so.AccessLocalStateStore(ctx, replayCursorStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, []byte(replayCursorKey))
	if err != nil || !found {
		if err != nil {
			t.Fatal(err)
		}
		return nil
	}
	cursor := &space_world.ReplayCursor{}
	if err := cursor.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	return cursor
}

// writeReplayCursor replaces the replay cursor saved for so.
func writeReplayCursor(ctx context.Context, t *testing.T, so sobject.SharedObject, cursor *space_world.ReplayCursor) {
	t.Helper()
	data, err := cursor.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	store, release, err := so.AccessLocalStateStore(ctx, replayCursorStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(replayCursorKey), data); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// refusedCount returns the number of refused outcomes of cursor.
func refusedCount(cursor *space_world.ReplayCursor) int {
	n := 0
	for _, outcome := range cursor.GetOutcomes() {
		if outcome.GetReason() != "" {
			n++
		}
	}
	return n
}

// unhandledCount returns the number of outcomes of cursor refused for a
// missing handler.
func unhandledCount(cursor *space_world.ReplayCursor) int {
	n := 0
	for _, outcome := range cursor.GetOutcomes() {
		if strings.HasSuffix(outcome.GetReason(), world.ErrUnhandledOp.Error()) {
			n++
		}
	}
	return n
}

// cursorSeqnos returns the World seqno after each outcome of cursor.
func cursorSeqnos(cursor *space_world.ReplayCursor) []uint64 {
	seqnos := make([]uint64, len(cursor.GetOutcomes()))
	for i, outcome := range cursor.GetOutcomes() {
		seqnos[i] = outcome.GetSeqno()
	}
	return seqnos
}
