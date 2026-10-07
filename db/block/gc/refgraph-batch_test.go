package block_gc

import (
	"encoding/binary"
	"slices"
	"strconv"
	"testing"

	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	cayley_proto "github.com/aperturerobotics/cayley/graph/proto"
	cayley_hkv "github.com/aperturerobotics/cayley/kv"
	cayley_flat "github.com/aperturerobotics/cayley/kv/flat"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestHasRefsBatch validates deleted history, exact endpoints, and aligned results.
func TestHasRefsBatch(t *testing.T) {
	// Prepare one storage transaction holding the exact graph posting format.
	ctx := t.Context()
	tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	ids := map[string]uint64{"owner": 1, "a": 2, "b": 3, "c": 4, "d": 5}
	const predID = 9

	// Seed postings with a deleted newest entry and an older missing entry
	// that a newer live primitive makes unnecessary to read.
	for object, quads := range map[string][]uint64{"a": {1, 2}, "b": {99, 3}, "c": {4}, "d": {5}} {
		key := cayley_flat.KeyEscape(cayley_kv.DefaultQuadIndexes[1].Key([]uint64{ids[object], predID, ids["owner"]}))
		var posting []byte
		for _, id := range quads {
			posting = binary.AppendUvarint(posting, id)
		}
		if err := tx.Set(ctx, key, posting); err != nil {
			t.Fatal(err)
		}
	}

	// Seed primitives whose labels and endpoints must still be validated.
	primitives := []cayley_proto.Primitive{
		{Subject: 1, Predicate: predID, Object: 2},
		{Subject: 1, Predicate: predID, Object: 2, Deleted: true},
		{Subject: 1, Predicate: predID, Object: 3},
		{Subject: 1, Predicate: predID, Object: 4, Label: 7},
		{Subject: 1, Predicate: predID, Object: 2},
	}
	for i := range primitives {
		data, err := primitives[i].MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		key := cayley_flat.KeyEscape(cayley_hkv.Key{[]byte("l"), []byte(strconv.Itoa(i + 1))})
		if err := tx.Set(ctx, key, data); err != nil {
			t.Fatal(err)
		}
	}

	// Read unsorted, duplicate, absent, and mismatched edges in caller order.
	edges := []RefEdge{
		{Subject: "owner", Object: "b"}, {Subject: "owner", Object: "a"},
		{Subject: "owner", Object: "missing"}, {Subject: "owner", Object: "c"},
		{Subject: "owner", Object: "d"}, {Subject: "owner", Object: "a"},
	}
	found, err := hasRefsInTransaction(ctx, tx, predID, ids, edges)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(found, []bool{true, true, false, false, false, true}) {
		t.Fatalf("exact membership = %v", found)
	}

	// Require a missing newest primitive to remain a storage error.
	if err := tx.Delete(ctx, cayley_flat.KeyEscape(cayley_hkv.Key{[]byte("l"), []byte("3")})); err != nil {
		t.Fatal(err)
	}
	if _, err := hasRefsInTransaction(ctx, tx, predID, ids, edges[:1]); err == nil {
		t.Fatal("missing indexed primitive was accepted")
	}
}
