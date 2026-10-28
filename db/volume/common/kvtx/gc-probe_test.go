package kvtx

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// TestProbePublishNamesSweptBlock reports whether a publication that names a
// swept block without carrying its bytes is accepted.
func TestProbePublishNamesSweptBlock(t *testing.T) {
	// Publish a block, release it and sweep it.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	released := publicationFor(t, "released", "", "one")
	if err := v.PublishAtomic(ctx, released); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, released)
	rg := v.refGraph.(*transactionRefGraph)
	stats, err := block_gc.NewCollector(rg, v, nil).Collect(ctx)
	if err != nil || stats.BlocksSwept != 1 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}

	// Publish a parent naming the swept block by ref only.
	parent := publicationFor(t, "parent", "", "two")
	parent.Entries[0].Refs, parent.Entries[0].RefsKnown = []*block.BlockRef{released.Entries[0].Ref}, true
	err = v.PublishAtomic(ctx, parent)
	_, found, _ := v.GetBlock(ctx, released.Entries[0].Ref)
	t.Logf("publish err=%v child found=%v", err, found)
}
