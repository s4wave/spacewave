package world_block

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// countingObjTree wraps the world state's object tree and counts Exists calls
// per key. HasObject issues one Exists per object-tree miss, so counting Exists
// proves EnsureTypeExists reuses the transaction-local object memo instead of
// re-reading the type object on every same-type write.
type countingObjTree struct {
	kvtx.BlockTx

	existsCalls map[string]int
}

func (c *countingObjTree) Exists(ctx context.Context, key []byte) (bool, error) {
	c.existsCalls[string(key)]++
	return c.BlockTx.Exists(ctx, key)
}

func TestSetObjectTypeReusesObjectExistenceWithinTransaction(t *testing.T) {
	ctx := context.Background()
	ws, _, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()

	counter := &countingObjTree{BlockTx: ws.objTree, existsCalls: map[string]int{}}
	ws.objTree = counter

	const typeID = "memo-type"
	typeKey := world_types.BuildTypeObjectKey(typeID)
	existsKey := objectKeyPrefix + typeKey

	// Create the type object up front so the loop never re-creates it.
	if _, err := world_types.EnsureTypeExists(ctx, ws, typeID); err != nil {
		t.Fatal(err.Error())
	}

	// Drop transaction-local knowledge so the first HasObject re-checks storage,
	// then measure only the object-tree existence checks from that point on.
	ws.objectExistsMemo = nil
	counter.existsCalls = map[string]int{}

	const objCount = 3
	for i := range objCount {
		key := "memo/obj-" + strconv.Itoa(i)
		if _, err := ws.CreateObject(ctx, key, nil); err != nil {
			t.Fatal(err.Error())
		}
		if err := world_types.SetObjectType(ctx, ws, key, typeID); err != nil {
			t.Fatal(err.Error())
		}
	}
	if got := counter.existsCalls[existsKey]; got != 1 {
		t.Fatalf("type-object existence checks = %d, want 1 across %d same-type writes", got, objCount)
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

func TestHasObjectForgetsDeletedObject(t *testing.T) {
	ctx := context.Background()
	ws, _, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()

	key := "forget/obj"
	if _, err := ws.CreateObject(ctx, key, nil); err != nil {
		t.Fatal(err.Error())
	}
	// CreateObject records existence, so the memo answers positively.
	if !ws.objectExistsKnown(key) {
		t.Fatal("expected created object memoized")
	}
	exists, err := ws.HasObject(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !exists {
		t.Fatal("HasObject = false, want true for created object")
	}

	deleted, err := ws.DeleteObject(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatal("expected object to be deleted")
	}
	if ws.objectExistsKnown(key) {
		t.Fatal("expected object memo invalidated after DeleteObject")
	}
	exists, err = ws.HasObject(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("HasObject = true, want false for deleted object")
	}
}

func TestHasObjectForgetsRenamedObject(t *testing.T) {
	ctx := context.Background()
	ws, _, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()

	oldKey := "rename/src"
	newKey := "rename/dst"
	if _, err := ws.CreateObject(ctx, oldKey, nil); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := ws.RenameObject(ctx, oldKey, newKey, false); err != nil {
		t.Fatal(err.Error())
	}
	if ws.objectExistsKnown(oldKey) {
		t.Fatal("expected old-key memo invalidated after RenameObject")
	}
	exists, err := ws.HasObject(ctx, oldKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists {
		t.Fatal("HasObject(oldKey) = true, want false after rename")
	}
	exists, err = ws.HasObject(ctx, newKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !exists {
		t.Fatal("HasObject(newKey) = false, want true after rename")
	}
}

func TestHasObjectMemoResetsWithBlockTransaction(t *testing.T) {
	ctx := context.Background()
	ws, _, cleanup := newRefBatchTestWorld(t, ctx)
	defer cleanup()

	key := "reset/obj"
	if _, err := ws.CreateObject(ctx, key, nil); err != nil {
		t.Fatal(err.Error())
	}
	if !ws.objectExistsKnown(key) {
		t.Fatal("expected created object memoized")
	}

	// Discard resets the transaction-local knowledge; a re-created memo starts
	// empty so a later HasObject re-checks storage rather than trusting stale
	// cross-transaction knowledge.
	ws.Discard()
	if ws.objectExistsKnown(key) {
		t.Fatal("expected object memo reset after Discard")
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
