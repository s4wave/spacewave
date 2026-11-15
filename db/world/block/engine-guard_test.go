package world_block_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// refusedOpType is the op type refuseGuard refuses.
const refusedOpType = "test/refused"

// refuseGuard refuses the op type refusedOpType and writes to object
// "protected".
type refuseGuard struct{}

// CheckWrite refuses refusedOpType and writes to "protected".
func (*refuseGuard) CheckWrite(ctx context.Context, ws world.WorldState, w *world.GuardedWrite) error {
	if w.OpTypeID == refusedOpType || slices.Contains(w.ObjectKeys, "protected") {
		return &world.OperationRejection{Message: "refused"}
	}
	return nil
}

// swallowOp makes an allowed write, then a refused one whose error it
// swallows, and succeeds. An op of refusedOpType does nothing.
type swallowOp struct {
	world.Operation
	typeID string
}

// TypeID returns the op's type.
func (o *swallowOp) TypeID() string { return o.typeID }

// ApplyWorldOp creates "created", then swallows the refusal of creating
// "protected".
func (o *swallowOp) ApplyWorldOp(ctx context.Context, le *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
	if o.typeID == refusedOpType {
		return false, nil
	}
	obj, err := ws.CreateObject(ctx, "created", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		return false, err
	}
	obj, _ = ws.CreateObject(ctx, "protected", nil)
	world.ReleaseObjectState(obj)
	return false, nil
}

// ApplyWorldObjectOp increments the object's revision, then swallows the
// refusal of a nested op of refusedOpType.
func (o *swallowOp) ApplyWorldObjectOp(ctx context.Context, le *logrus.Entry, obj world.ObjectState, sender peer.ID) (bool, error) {
	if o.typeID == refusedOpType {
		return false, nil
	}
	if _, err := obj.IncrementRev(ctx); err != nil {
		return false, err
	}
	_, _, _ = obj.ApplyObjectOp(ctx, &swallowOp{typeID: refusedOpType}, sender)
	return false, nil
}

// TestEngineGuardedOpRefusalNeverCommits verifies that a World op and an object
// op applied through a guarded engine World state, each of which makes an
// allowed write and swallows a refused one, return the refusal and commit
// nothing: the engine fails the op before its own transaction commits.
func TestEngineGuardedOpRefusalNeverCommits(t *testing.T) {
	// Open an Engine on an empty World.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ocs.Release)
	btx, bcs := ocs.BuildTransaction(nil)
	root := world_block.NewWorld(world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV, false)
	root.ObjectKeyValue = kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL)
	bcs.SetBlock(root, true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	ocs.SetRootRef(rootRef)
	engine, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	plain := world.NewEngineWorldState(engine, true)
	own, err := plain.CreateObject(ctx, "own", nil)
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(own)
	ws := world.GuardWorldState(plain, &refuseGuard{})
	op := &swallowOp{typeID: "test/swallow"}

	// A World op that swallowed a refusal fails and creates nothing.
	var rejection *world.OperationRejection
	if _, _, err := ws.ApplyWorldOp(ctx, op, ""); !errors.As(err, &rejection) {
		t.Fatalf("World op returned %v, want the refusal", err)
	}
	created, found, err := plain.GetObject(ctx, "created")
	world.ReleaseObjectState(created)
	if err != nil || found {
		t.Fatalf("World op committed its allowed write: found %v, %v", found, err)
	}

	// An object op that swallowed a refusal fails and leaves its object.
	before := ownRev(ctx, t, plain)
	obj, err := world.MustGetObject(ctx, ws, "own")
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)
	if _, _, err := obj.ApplyObjectOp(ctx, op, ""); !errors.As(err, &rejection) {
		t.Fatalf("object op returned %v, want the refusal", err)
	}
	if after := ownRev(ctx, t, plain); after != before {
		t.Fatalf("object op committed its allowed write: rev %d, was %d", after, before)
	}
}

// ownRev returns the committed revision of object "own".
func ownRev(ctx context.Context, t *testing.T, ws world.WorldState) uint64 {
	obj, err := world.MustGetObject(ctx, ws, "own")
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)
	_, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return rev
}
