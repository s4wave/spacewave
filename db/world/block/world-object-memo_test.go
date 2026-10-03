package world_block

import (
	"context"
	"slices"
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

// Exists counts the call and forwards it to the wrapped tree.
func (c *countingObjTree) Exists(ctx context.Context, key []byte) (bool, error) {
	c.existsCalls[string(key)]++
	return c.BlockTx.Exists(ctx, key)
}

func TestSetObjectTypeReusesObjectExistenceWithinTransaction(t *testing.T) {
	// Build a World whose object tree counts existence checks.
	ctx := context.Background()
	ws, _, cleanup := newTestWorld(t, ctx)
	defer cleanup()
	counter := &countingObjTree{BlockTx: ws.objTree, existsCalls: map[string]int{}}
	ws.objTree = counter

	// Create the type object up front so the loop never re-creates it.
	const typeID = "memo-type"
	typeKey := world_types.BuildTypeObjectKey(typeID)
	existsKey := objectKeyPrefix + typeKey
	if err := world_types.EnsureTypeExists(ctx, ws, typeID); err != nil {
		t.Fatal(err.Error())
	}

	// Drop transaction-local knowledge so the first HasObject re-checks storage,
	// then measure only the object-tree existence checks from that point on.
	ws.objectExistsMemo = nil
	counter.existsCalls = map[string]int{}

	// Type several objects with the same type.
	const objCount = 3
	for i := range objCount {
		key := "memo/obj-" + strconv.Itoa(i)
		createTestObject(t, ctx, ws, key)
		if err := world_types.SetObjectType(ctx, ws, key, typeID); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Check the type object was read from storage once.
	if got := counter.existsCalls[existsKey]; got != 1 {
		t.Fatalf("type-object existence checks = %d, want 1 across %d same-type writes", got, objCount)
	}
}

func TestSetObjectTypeIdempotentSameTypeIsNoOp(t *testing.T) {
	// Build a World with one object.
	ctx := context.Background()
	ws, _, cleanup := newTestWorld(t, ctx)
	defer cleanup()
	key := "idempotent/obj"
	createTestObject(t, ctx, ws, key)

	// Set the same type twice and check one type edge remains.
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
	// Build a World with one object.
	ctx := context.Background()
	ws, _, cleanup := newTestWorld(t, ctx)
	defer cleanup()
	key := "retype/obj"
	createTestObject(t, ctx, ws, key)

	// Change the type and check only the new type edge remains.
	if err := world_types.SetObjectType(ctx, ws, key, "type-one"); err != nil {
		t.Fatal(err.Error())
	}
	if err := world_types.SetObjectType(ctx, ws, key, "type-two"); err != nil {
		t.Fatal(err.Error())
	}
	assertSingleTypeEdge(t, ctx, ws, key, "type-two")

	// Check the object reports the new type.
	gotType, err := world_types.GetObjectType(ctx, ws, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if gotType != "type-two" {
		t.Fatalf("GetObjectType = %q, want %q", gotType, "type-two")
	}
}

func TestHasObjectForgetsDeletedObject(t *testing.T) {
	// Build a World with one object.
	ctx := context.Background()
	ws, _, cleanup := newTestWorld(t, ctx)
	defer cleanup()
	key := "forget/obj"
	createTestObject(t, ctx, ws, key)

	// CreateObject records existence, so the memo answers positively.
	if !ws.objectExistsKnown(key) {
		t.Fatal("expected created object memoized")
	}
	assertHasObject(t, ctx, ws, key, true)

	// Delete the object and check the memo forgot it.
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
	assertHasObject(t, ctx, ws, key, false)
}

func TestHasObjectForgetsRenamedObject(t *testing.T) {
	// Build a World with one object.
	ctx := context.Background()
	ws, _, cleanup := newTestWorld(t, ctx)
	defer cleanup()
	oldKey := "rename/src"
	newKey := "rename/dst"
	createTestObject(t, ctx, ws, oldKey)

	// Rename the object.
	objectState, err := ws.RenameObject(ctx, oldKey, newKey, false)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check the memo forgot the old key and the new key exists.
	if ws.objectExistsKnown(oldKey) {
		t.Fatal("expected old-key memo invalidated after RenameObject")
	}
	assertHasObject(t, ctx, ws, oldKey, false)
	assertHasObject(t, ctx, ws, newKey, true)
}

func TestHasObjectMemoResetsWithBlockTransaction(t *testing.T) {
	// Build a World with one memoized object.
	ctx := context.Background()
	ws, _, cleanup := newTestWorld(t, ctx)
	defer cleanup()
	key := "reset/obj"
	createTestObject(t, ctx, ws, key)
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

// assertHasObject checks that HasObject reports want for key.
func assertHasObject(t *testing.T, ctx context.Context, ws *WorldState, key string, want bool) {
	t.Helper()
	exists, err := ws.HasObject(ctx, key)
	if err != nil {
		t.Fatal(err.Error())
	}
	if exists != want {
		t.Fatalf("HasObject(%q) = %v, want %v", key, exists, want)
	}
}

// assertSingleTypeEdge checks that key has exactly one type edge, to typeID.
func assertSingleTypeEdge(t *testing.T, ctx context.Context, ws *WorldState, key, typeID string) {
	// Look up the object's type edges.
	t.Helper()
	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(key, world_types.TypePred.String(), "", ""),
		0,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Decode the edge targets.
	typeKeys := make([]string, 0, len(quads))
	for _, q := range quads {
		objKey, err := world.GraphValueToKey(q.GetObj())
		if err != nil {
			t.Fatal(err.Error())
		}
		typeKeys = append(typeKeys, objKey)
	}

	// Compare them with the one expected type object.
	if want := []string{world_types.BuildTypeObjectKey(typeID)}; !slices.Equal(typeKeys, want) {
		t.Fatalf("type edges for %s = %#v, want %#v", key, typeKeys, want)
	}
}
