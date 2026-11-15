package layer

import (
	"context"
	"io/fs"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// TestMergeLayers checks that laying layers onto the base, one at a time or
// all at once, yields the root a flat build of the same entries yields, and
// reports the file a later layer replaces.
func TestMergeLayers(t *testing.T) {
	// Open a cursor over an in-memory store.
	ctx := context.Background()
	store := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0)
	le := logrus.NewEntry(logrus.New())
	bls := bucket_lookup.NewCursor(ctx, nil, le, nil, store, nil, &bucket.ObjectRef{}, nil, nil)
	defer bls.Release()

	// Build the base and two layers that share usr/bin and replace one file.
	layers := [][]*Entry{
		{
			{Path: "usr/bin", Type: fs.ModeDir, Perm: 0o755},
			{Path: "usr/bin/sh", Perm: 0o755, Data: []byte("shell")},
			{Path: "usr/share/sh", Type: fs.ModeDir, Perm: 0o755},
		},
		{
			{Path: "usr/bin", Type: fs.ModeDir, Perm: 0o755},
			{Path: "usr/bin/sh", Perm: 0o755, Data: []byte("other shell")},
			{Path: "usr/bin/ls", Type: fs.ModeSymlink, Target: "sh"},
		},
	}
	base, err := BuildTree(ctx, bls, BaseEntries)
	if err != nil {
		t.Fatal(err)
	}
	flat := append([]*Entry(nil), BaseEntries...)
	refs := make([]*block.BlockRef, len(layers))
	for i, entries := range layers {
		if refs[i], err = BuildTree(ctx, bls, entries); err != nil {
			t.Fatal(err)
		}
		flat = append(flat, entries...)
	}

	// Lay the layers one at a time, recording each replaced file.
	var collisions []string
	collide := func(path string) { collisions = append(collisions, path) }
	inc := base
	for _, ref := range refs {
		if inc, err = MergeLayers(ctx, bls, inc, []*block.BlockRef{ref}, collide); err != nil {
			t.Fatal(err)
		}
	}
	if len(collisions) != 1 || collisions[0] != "/usr/bin/sh" {
		t.Fatalf("collisions %v, want [/usr/bin/sh]", collisions)
	}

	// Compare with laying them all at once and with the flat build.
	all, err := MergeLayers(ctx, bls, base, refs, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	flatRef, err := BuildTree(ctx, bls, flat)
	if err != nil {
		t.Fatal(err)
	}
	if !inc.EqualsRef(all) || !all.EqualsRef(flatRef) {
		t.Fatalf("roots differ: incremental %s, all %s, flat %s",
			inc.MarshalString(), all.MarshalString(), flatRef.MarshalString())
	}
}
