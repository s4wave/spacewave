package kvtx

import (
	"testing"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestYoungReadySurvivesReopen checks that minor passes run on a store born
// empty and on one a block pass promoted before a restart, never on a store
// upgraded from a format whose builds may not log young records, and that a
// minor pass sweeps only released blocks.
func TestYoungReadySurvivesReopen(t *testing.T) {
	ctx := t.Context()
	store := store_kvtx_inmem.NewStore()
	keys := store_kvkey.NewDefaultKVKey()
	open := func() *Volume {
		t.Helper()
		v, err := NewVolume(ctx, "test/young", keys, store, &store_kvtx.Config{}, false, false, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	closeVolume := func(v *Volume) {
		t.Helper()
		if err := v.Close(); err != nil {
			t.Fatal(err)
		}
	}
	sweepYoung := func(v *Volume) {
		t.Helper()
		var stats block_gc.Stats
		if err := v.SweepYoungBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
			t.Fatal(err)
		}
	}

	// A store born empty is minor-ready: a minor pass sweeps a released
	// block and keeps a published one.
	v := open()
	live := publicationFor(t, "live", "", "one")
	if err := v.PublishAtomic(ctx, live); err != nil {
		t.Fatal(err)
	}
	first := publicationFor(t, "first", "", "one")
	if err := v.PublishAtomic(ctx, first); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, first)
	sweepYoung(v)
	assertPublishedBlock(t, v, first, false)
	assertPublishedBlock(t, v, live, true)
	closeVolume(v)

	// Stand in for a store of format version 4 with blocks: drop the ready
	// record, 'm' in the reference graph, and rewind the version.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Delete(ctx, append(append([]byte(nil), refGraphPrefix...), 'm')); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(4)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The upgraded store is not minor-ready: a minor pass sweeps nothing.
	v = open()
	second := publicationFor(t, "second", "", "one")
	if err := v.PublishAtomic(ctx, second); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, second)
	sweepYoung(v)
	if _, found, err := v.GetBlock(ctx, second.Entries[0].Ref); err != nil || !found {
		t.Fatalf("minor pass on an upgraded store: found=%v err=%v", found, err)
	}

	// A major pass sweeps it and promotes.
	var stats block_gc.Stats
	if err := v.SweepUnreachedBlocks(ctx, v.GetRefGraph(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.BlocksSwept != 1 {
		t.Fatalf("major pass stats=%+v", stats)
	}
	assertPublishedBlock(t, v, live, true)
	closeVolume(v)

	// After a restart minor passes run, and keep the live block.
	v = open()
	t.Cleanup(func() { closeVolume(v) })
	third := publicationFor(t, "third", "", "one")
	if err := v.PublishAtomic(ctx, third); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, third)
	sweepYoung(v)
	assertPublishedBlock(t, v, third, false)
	assertPublishedBlock(t, v, live, true)
}
