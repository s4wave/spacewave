package sobject_world_engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/testbed"
)

// untrackedRetainer is a store that owns roots but holds block bytes without
// their refs.
type untrackedRetainer struct {
	block.StoreOps
	block.RootRetainer
}

// SupportsRootRetention reports that the store owns roots.
func (untrackedRetainer) SupportsRootRetention() bool { return true }

// TestRootRejectsUnknownRefs checks that holding an untracked root fails
// naming the block, instead of storing it as a leaf.
func TestRootRejectsUnknownRefs(t *testing.T) {
	// Hold a root that the store has without its refs.
	ctx := t.Context()
	inner := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
	root, _, err := inner.PutBlock(ctx, []byte("untracked root"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// A hold and a retained checkpoint each fail naming the root.
	hold := newRootHold(&untrackedRetainer{StoreOps: inner})
	holdErr := hold.world(ctx, "world", root)
	if len(hold.entries) != 0 || len(hold.roots) != 0 {
		t.Fatalf("hold gathered %d entries and %d roots", len(hold.entries), len(hold.roots))
	}
	_, storeErr := storeRootBlock(ctx, &testSharedObject{blockStore: newTestBlockStore("untracked", inner)}, root)
	for name, err := range map[string]error{"hold": holdErr, "store": storeErr} {
		if !errors.Is(err, block.ErrRefsUnknown) || !strings.Contains(err.Error(), root.MarshalString()) {
			t.Fatalf("%s = %v, want ErrRefsUnknown naming %s", name, err, root.MarshalString())
		}
	}
}

// readProbe records the blocks read through the durable store, exposing the
// real volume's root retention that testBlockStore hides.
type readProbe struct {
	*testBlockStore
	block.RootRetainer
	// reads are the refs read through GetStoredBlock.
	reads []*block.BlockRef
}

// GetStoredBlock records the read and returns the real store's result.
func (p *readProbe) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	p.reads = append(p.reads, ref)
	return p.testBlockStore.GetStoredBlock(ctx, ref)
}

// TestPendingSaveCandidatePayload checks that preparing a publication's replay
// save never reads a payload still in the transaction buffer, which no store
// holds before the publication writes it, while it still carries a stored
// payload so the span keeps holding it.
func TestPendingSaveCandidatePayload(t *testing.T) {
	// Build a real volume that keeps hold counts.
	ctx := t.Context()
	space := newReplayTestSpace(t)
	tb, err := testbed.NewTestbed(ctx, space.c.le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)
	inner := cursor.GetBucket()
	probe := &readProbe{testBlockStore: newTestBlockStore("candidate-payload", inner), RootRetainer: inner.(block.RootRetainer)}
	space.so.blockStore = probe
	if !block.KeepsHoldCounts(probe) {
		t.Fatal("the volume must keep hold counts")
	}

	// Store one payload, and buffer another as a write's candidate.
	storedData := []byte("stored payload")
	stored, err := block.BuildBlockRef(storedData, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := probe.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: stored, Data: storedData, RefsKnown: true}}); err != nil {
		t.Fatal(err)
	}
	data := []byte("candidate payload")
	candidate, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	buffer := block.NewBufferedStore(ctx, probe)
	if _, err := buffer.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: candidate, Data: data, RefsKnown: true}}); err != nil {
		t.Fatal(err)
	}
	batch, err := buffer.TakePending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer batch.Complete(nil)

	// Prepare the save as publish does, with the borrowed batch.
	r := &replayer{c: space.c, so: space.so, base: space.genesis}
	pending, err := r.pendingSave(ctx, replayTestPublisher{}, []*block.BlockRef{candidate, stored}, batch.Entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range probe.reads {
		if ref.EqualsRef(candidate) {
			t.Fatalf("pendingSave read the unpublished candidate %s", candidate.MarshalString())
		}
	}
	var carried bool
	for _, e := range pending.entries {
		if e.Ref.EqualsRef(candidate) {
			t.Fatal("pendingSave put the candidate the batch already carries")
		}
		carried = carried || e.Ref.EqualsRef(stored)
	}
	if !carried {
		t.Fatal("pendingSave did not carry the stored payload")
	}
}
