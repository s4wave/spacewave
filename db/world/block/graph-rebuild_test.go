package world_block

import (
	"bytes"
	"maps"
	"testing"

	"sort"
	"strings"

	cayley_proto "github.com/aperturerobotics/cayley/graph/proto"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/quad/pquads"
	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestRebuildGraphTree checks a rebuild of an undamaged graph changes nothing,
// and a rebuild of a graph that lost log records, a reference count and a
// posting restores every entry.
func TestRebuildGraphTree(t *testing.T) {
	// Open a World with four objects and three edges, one of them labeled.
	ctx := t.Context()
	store := block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	btx, bcs := block.NewTransaction(store, nil, nil, nil)
	ws, err := NewWorldState(ctx, logrus.NewEntry(logrus.New()), true, btx, bcs, store, nil, world_mock.LookupMockOp, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"a", "b", "c", "lost"} {
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []world.GraphQuad{
		world.NewGraphQuadWithKeys("a", "<next>", "b", ""),
		world.NewGraphQuadWithKeys("b", "<next>", "lost", ""),
		world.NewGraphQuadWithKeys("a", "<owner>", "c", "tag"),
	} {
		if err := ws.SetGraphQuad(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := ws.flushGraphWrites(ctx); err != nil {
		t.Fatal(err)
	}
	tree := ws.graphTree.(*kvtx_block_okra.Tx)
	before := graphEntries(t, tree)

	// An undamaged graph rebuilds to the same entries.
	r, err := ws.RebuildGraphTree(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	compared := make(map[string]int)
	for _, c := range r.Checks {
		compared[c.Bucket] = c.Same
		if c.Changed != 0 || c.Added != 0 || c.Removed != 0 {
			t.Errorf("undamaged bucket %s: %+v", c.Bucket, c)
		}
	}
	for _, bucket := range []string{"m", "n", "ops", "sp", "v"} {
		if compared[bucket] == 0 {
			t.Errorf("bucket %s compared no keys: %v", bucket, compared)
		}
	}
	if len(r.Dropped) != 0 {
		t.Errorf("undamaged graph dropped %+v", r.Dropped)
	}

	// Drop the records of "lost" and the quad naming it, one reference count
	// and one subject posting.
	var lostNode uint64
	records := readGraphLog(t, tree)
	for _, p := range records {
		if !p.IsNode() {
			continue
		}
		if v, err := pquads.UnmarshalValue(ctx, p.Value); err == nil && v == quad.IRI("lost") {
			lostNode = p.ID
		}
	}
	drop := [][]byte{graphLogKey(lostNode), firstKey(before, "n/"), firstKey(before, "sp/")}
	for _, p := range records {
		if !p.IsNode() && p.Object == lostNode {
			drop = append(drop, graphLogKey(p.ID))
		}
	}
	for _, key := range drop {
		if err := tree.Delete(ctx, key); err != nil {
			t.Fatal(err)
		}
	}

	// The rebuild restores every entry.
	r, err = ws.RebuildGraphTree(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.RecoveredNodes) != 1 || len(r.RecoveredQuads) != 1 || len(r.Dropped) != 0 {
		t.Fatalf("recovered nodes=%v quads=%v dropped=%+v", r.RecoveredNodes, r.RecoveredQuads, r.Dropped)
	}
	after := graphEntries(t, tree)
	if !maps.EqualFunc(before, after, bytes.Equal) {
		for k, v := range before {
			if !bytes.Equal(after[k], v) {
				t.Errorf("key %q: before %x after %x", k, v, after[k])
			}
		}
		for k := range after {
			if _, ok := before[k]; !ok {
				t.Errorf("key %q added", k)
			}
		}
	}
}

// graphEntries reads every key and value of tree.
func graphEntries(t *testing.T, tree *kvtx_block_okra.Tx) map[string][]byte {
	out := make(map[string][]byte)
	err := tree.Salvage(t.Context(), nil, nil, func(entry kvtx_block_okra.BuildEntry) bool {
		data, _, err := tree.Get(t.Context(), entry.Key)
		if err != nil {
			t.Fatal(err)
		}
		out[string(entry.Key)] = bytes.Clone(data)
		return true
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// readGraphLog reads every log record of tree.
func readGraphLog(t *testing.T, tree *kvtx_block_okra.Tx) []*cayley_proto.Primitive {
	var out []*cayley_proto.Primitive
	err := tree.Salvage(t.Context(), []byte("l/"), []byte("l0"), func(entry kvtx_block_okra.BuildEntry) bool {
		data, _, err := tree.Get(t.Context(), entry.Key)
		if err != nil {
			t.Fatal(err)
		}
		p := new(cayley_proto.Primitive)
		if err := p.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
		return true
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// firstKey returns the first key of entries with prefix.
func firstKey(entries map[string][]byte, prefix string) []byte {
	var keys []string
	for k := range entries {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return []byte(keys[0])
}
