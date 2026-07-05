package world_block

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
)

// typeObjectReadCounter embeds the concrete world state so it forwards the
// type-object memo methods by promotion while counting GetObject reads. It
// proves EnsureTypeExists reuses the transaction-local type memo.
type typeObjectReadCounter struct {
	*WorldState

	getObjectCalls map[string]int
}

func (c *typeObjectReadCounter) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	c.getObjectCalls[key]++
	return c.WorldState.GetObject(ctx, key)
}

// forwardingTypeObjectReadCounter wraps a world.WorldState and counts GetObject
// reads while explicitly forwarding the type-object memo methods to the wrapped
// state. It mirrors how the production transaction wrappers
// (world_block_tx.WorldState, block.Tx) forward memo knowledge to the state they
// wrap, proving memo reuse survives an explicit wrapper's forwarding rather than
// depending only on the base interface being promoted.
type forwardingTypeObjectReadCounter struct {
	inner world.WorldState

	getObjectCalls map[string]int
}

func (c *forwardingTypeObjectReadCounter) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	c.getObjectCalls[key]++
	return c.inner.GetObject(ctx, key)
}

func (c *forwardingTypeObjectReadCounter) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	return c.inner.CreateObject(ctx, key, rootRef)
}

func (c *forwardingTypeObjectReadCounter) IterateObjects(ctx context.Context, prefix string, reversed bool) world.ObjectIterator {
	return c.inner.IterateObjects(ctx, prefix, reversed)
}

func (c *forwardingTypeObjectReadCounter) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	return c.inner.RenameObject(ctx, oldKey, newKey, descendants)
}

func (c *forwardingTypeObjectReadCounter) DeleteObject(ctx context.Context, key string) (bool, error) {
	return c.inner.DeleteObject(ctx, key)
}

func (c *forwardingTypeObjectReadCounter) TypeObjectEnsured(typeObjectKey string) bool {
	return c.inner.TypeObjectEnsured(typeObjectKey)
}

func (c *forwardingTypeObjectReadCounter) MarkTypeObjectEnsured(typeObjectKey string) {
	c.inner.MarkTypeObjectEnsured(typeObjectKey)
}

func (c *forwardingTypeObjectReadCounter) GetReadOnly() bool { return c.inner.GetReadOnly() }

func (c *forwardingTypeObjectReadCounter) Sync(ctx context.Context) (bool, error) {
	return c.inner.Sync(ctx)
}

func (c *forwardingTypeObjectReadCounter) GetSeqno(ctx context.Context) (uint64, error) {
	return c.inner.GetSeqno(ctx)
}

func (c *forwardingTypeObjectReadCounter) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return c.inner.WaitSeqno(ctx, value)
}

func (c *forwardingTypeObjectReadCounter) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return c.inner.BuildStorageCursor(ctx)
}

func (c *forwardingTypeObjectReadCounter) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return c.inner.AccessWorldState(ctx, ref, cb)
}

func (c *forwardingTypeObjectReadCounter) ApplyWorldOp(ctx context.Context, op world.Operation, opSender peer.ID) (uint64, bool, error) {
	return c.inner.ApplyWorldOp(ctx, op, opSender)
}

func (c *forwardingTypeObjectReadCounter) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	return c.inner.AccessCayleyGraph(ctx, write, cb)
}

func (c *forwardingTypeObjectReadCounter) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	return c.inner.LookupGraphQuads(ctx, filter, limit)
}

func (c *forwardingTypeObjectReadCounter) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	return c.inner.LookupGraphQuadsBatch(ctx, filters, limitPerFilter)
}

func (c *forwardingTypeObjectReadCounter) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	return c.inner.QueryGraphPath(ctx, query)
}

func (c *forwardingTypeObjectReadCounter) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	return c.inner.SetGraphQuad(ctx, q)
}

func (c *forwardingTypeObjectReadCounter) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	return c.inner.DeleteGraphQuad(ctx, q)
}

func (c *forwardingTypeObjectReadCounter) DeleteGraphObject(ctx context.Context, value string) error {
	return c.inner.DeleteGraphObject(ctx, value)
}

func TestSetObjectTypeReusesEnsuredTypeObjectWithinTransaction(t *testing.T) {
	ctx := context.Background()
	ws, _, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()

	const objCount = 3

	// Memoized path: the counting wrapper embeds *WorldState, so the
	// TypeObjectMemo optional interface is promoted and EnsureTypeExists reuses
	// the transaction-local memo across same-type writes.
	memoWS := &typeObjectReadCounter{WorldState: ws, getObjectCalls: map[string]int{}}
	const memoType = "memo-type"
	memoTypeKey := world_types.BuildTypeObjectKey(memoType)
	for i := range objCount {
		key := "memo/obj-" + strconv.Itoa(i)
		if _, err := ws.CreateObject(ctx, key, nil); err != nil {
			t.Fatal(err.Error())
		}
		if err := world_types.SetObjectType(ctx, memoWS, key, memoType); err != nil {
			t.Fatal(err.Error())
		}
	}
	if got := memoWS.getObjectCalls[memoTypeKey]; got != 1 {
		t.Fatalf("memoized type-object reads = %d, want 1 across %d same-type writes", got, objCount)
	}

	// Forwarding path: the wrapper holds the state in a field and explicitly
	// forwards the memo methods, as the production transaction wrappers do, so
	// EnsureTypeExists reuses the memo through the wrapper and reads the type
	// object exactly once across the same-type writes.
	fwdWS := &forwardingTypeObjectReadCounter{inner: ws, getObjectCalls: map[string]int{}}
	const fwdType = "forward-type"
	fwdTypeKey := world_types.BuildTypeObjectKey(fwdType)
	for i := range objCount {
		key := "forward/obj-" + strconv.Itoa(i)
		if _, err := ws.CreateObject(ctx, key, nil); err != nil {
			t.Fatal(err.Error())
		}
		if err := world_types.SetObjectType(ctx, fwdWS, key, fwdType); err != nil {
			t.Fatal(err.Error())
		}
	}
	if got := fwdWS.getObjectCalls[fwdTypeKey]; got != 1 {
		t.Fatalf("forwarded type-object reads = %d, want 1 across %d same-type writes", got, objCount)
	}
}

func TestSetObjectTypeIdempotentSameTypeIsNoOp(t *testing.T) {
	ctx := context.Background()
	ws, _, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()

	key := "idempotent/obj"
	if _, err := ws.CreateObject(ctx, key, nil); err != nil {
		t.Fatal(err.Error())
	}
	const typeID = "idempotent-type"
	if err := world_types.SetObjectType(ctx, ws, key, typeID); err != nil {
		t.Fatal(err.Error())
	}
	if err := world_types.SetObjectType(ctx, ws, key, typeID); err != nil {
		t.Fatal(err.Error())
	}
	assertSingleTypeEdge(t, ctx, ws, key, typeID)
}

func TestSetObjectTypeDeletesStaleTypeEdgeOnTypeChange(t *testing.T) {
	ctx := context.Background()
	ws, _, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()

	key := "retype/obj"
	if _, err := ws.CreateObject(ctx, key, nil); err != nil {
		t.Fatal(err.Error())
	}
	if err := world_types.SetObjectType(ctx, ws, key, "type-one"); err != nil {
		t.Fatal(err.Error())
	}
	if err := world_types.SetObjectType(ctx, ws, key, "type-two"); err != nil {
		t.Fatal(err.Error())
	}
	assertSingleTypeEdge(t, ctx, ws, key, "type-two")

	gotType, err := world_types.GetObjectType(ctx, ws, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if gotType != "type-two" {
		t.Fatalf("GetObjectType = %q, want %q", gotType, "type-two")
	}
}

func TestEnsureTypeExistsRecreatesTypeObjectAfterDeletion(t *testing.T) {
	ctx := context.Background()
	ws, _, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()

	const typeID = "invalidate-type"
	typeKey := world_types.BuildTypeObjectKey(typeID)

	if _, err := world_types.EnsureTypeExists(ctx, ws, typeID); err != nil {
		t.Fatal(err.Error())
	}
	if !ws.TypeObjectEnsured(typeKey) {
		t.Fatal("expected type object memoized after EnsureTypeExists")
	}

	deleted, err := ws.DeleteObject(ctx, typeKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatal("expected type object to be deleted")
	}
	if ws.TypeObjectEnsured(typeKey) {
		t.Fatal("expected type memo invalidated after DeleteObject")
	}

	if _, err := world_types.EnsureTypeExists(ctx, ws, typeID); err != nil {
		t.Fatal(err.Error())
	}
	_, exists, err := ws.GetObject(ctx, typeKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !exists {
		t.Fatal("expected type object recreated after deletion")
	}
}

func assertSingleTypeEdge(t *testing.T, ctx context.Context, ws *WorldState, key, typeID string) {
	t.Helper()

	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(key, world_types.TypePred.String(), "", ""),
		0,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	typeKeys := make([]string, 0, len(quads))
	for _, q := range quads {
		objKey, err := world.GraphValueToKey(q.GetObj())
		if err != nil {
			t.Fatal(err.Error())
		}
		typeKeys = append(typeKeys, objKey)
	}
	assertStringSet(t, "type edges for "+key, typeKeys, []string{world_types.BuildTypeObjectKey(typeID)})
}

// _ is a type assertion
var (
	_ world.WorldState = ((*typeObjectReadCounter)(nil))
	_ world.WorldState = ((*forwardingTypeObjectReadCounter)(nil))
)
