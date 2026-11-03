package sobject_world_engine

import (
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/net/hash"
)

// TestWorldDeathsComposesAcceptedChain tries to make the transition table
// report a page from a path the named World did not take, or report anything
// across a gap or a forfeited commit. Each World is one page, and each commit
// replaces it.
func TestWorldDeathsComposesAcceptedChain(t *testing.T) {
	// world stores the one-page World named name and returns its root.
	ctx := t.Context()
	store := block_mock.NewMockStore(hash.RecommendedHashType)
	world := func(name string) *block.BlockRef {
		// Store the page under a new root.
		tx, root := block.NewTransaction(store, nil, nil, nil)
		root.SetBlock(block_mock.NewExample(name), true)
		ref, _, err := tx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}

	// commit replaces the World at from with the one named to and returns
	// the commit's step.
	commit := func(from *block.BlockRef, to string) (*block.BlockRef, *block.ReplacementStep) {
		// Load the World through a recording transaction.
		log := block.NewReplacementLog(from)
		tx, root := block.NewTransaction(store, nil, from, nil)
		tx.RecordReplacements(log)
		if _, err := block_mock.UnmarshalExample(ctx, root); err != nil {
			t.Fatal(err)
		}

		// Replace its page and take the step.
		root.SetBlock(block_mock.NewExample(to), true)
		ref, _, err := tx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		return ref, log.Take(ref)
	}

	// dead returns what died across net. The commits store their pages
	// directly rather than through tracked puts, so every page counts as
	// stored afresh, and a dead one the chain created is dropped.
	dead := func(net *pendingNet) []*block.BlockRef {
		record := net.steps.Join().Net(nil)
		return append(record.Replaced, record.Dropped...)
	}

	// A local fork from a to x, but replay accepted a to b to c: a and b die,
	// x was never on the chain.
	a := world("a")
	d := newWorldDeaths(a)
	x, step := commit(a, "x")
	d.add(a, x, step)
	b, step := commit(a, "b")
	d.add(a, b, step)

	// Compose the accepted chain.
	c, step := commit(b, "c")
	d.add(b, c, step)
	_, net := d.take(c)
	if net == nil {
		t.Fatal("forfeited the accepted chain")
	}
	if got := dead(net); len(got) != 2 || !slices.ContainsFunc(got, a.EqualsRef) || !slices.ContainsFunc(got, b.EqualsRef) || slices.ContainsFunc(got, x.EqualsRef) {
		t.Fatalf("composed %v, want a and b", got)
	}

	// A put stored the fork's x afresh, deferring its edges to the record,
	// so the record drops x though no step on the chain created it.
	fork := []*block.PutBatchEntry{{Ref: x, RefsKnown: true}}
	receipt := block.NewPublicationReceipt()
	receipt.ResolveExisted(nil, []bool{false}, true, 0)
	net.puts = append(net.puts, newWorldPut(receipt, fork, fork, 0))
	if got := net.net(c).Dropped; !slices.EqualFunc(got, []*block.BlockRef{x}, (*block.BlockRef).EqualsRef) {
		t.Fatalf("dropped %v, want the fork's x", got)
	}

	// A current interval's G1 receipts cannot certify a From root whose
	// first fresh lineage receipt used G0, even if a later root put used G1.
	lineageEntry := []*block.PutBatchEntry{{Ref: a, RefsKnown: true}}
	oldRoot, laterRoot, currentPut := block.NewPublicationReceipt(), block.NewPublicationReceipt(), block.NewPublicationReceipt()
	oldRoot.ResolveExisted(nil, []bool{false}, true, 0)
	laterRoot.ResolveExisted(nil, []bool{false}, true, 1)
	currentPut.ResolveExisted(nil, []bool{false}, true, 1)
	currentEntry := []*block.PutBatchEntry{{Ref: c, RefsKnown: true}}
	epochNet := &pendingNet{
		steps: net.steps, sealed: true,
		lineage: &pendingNet{sealed: true, puts: []worldPut{
			newWorldPut(oldRoot, lineageEntry, lineageEntry, 0),
			newWorldPut(laterRoot, lineageEntry, lineageEntry, 0),
		}},
		puts: []worldPut{newWorldPut(currentPut, currentEntry, currentEntry, 0)},
	}
	epochNet.poll(t.Context(), 40, a, c, 0, false, nil)
	<-epochNet.encoded
	if epochNet.err != nil || epochNet.steps != nil || !epochNet.dead.CountsComplete || epochNet.dead.CountingGeneration != 1 {
		t.Fatalf("old lineage counted across generations: steps=%v dead=%+v error=%v", epochNet.steps, epochNet.dead, epochNet.err)
	}
	for _, entry := range epochNet.blocks {
		if entry.Ref.EqualsRef(epochNet.record) {
			var header block.Deadlist
			if err := header.UnmarshalVT(entry.Data); err != nil || !header.GetFrom().GetEmpty() {
				t.Fatalf("old lineage record From=%v error=%v", header.GetFrom(), err)
			}
		}
	}

	// Duplicate entries use their first receipt position, even after ordinary,
	// Counted and tombstone entries are omitted from World-page accounting.
	firstA, laterA := &block.PutBatchEntry{Ref: a}, &block.PutBatchEntry{Ref: a}
	firstB, laterB := &block.PutBatchEntry{Ref: b}, &block.PutBatchEntry{Ref: b}
	counted, tombstone := &block.PutBatchEntry{Ref: c, Counted: true}, &block.PutBatchEntry{Ref: x, Tombstone: true}
	batch := []*block.PutBatchEntry{{Ref: x}, firstA, laterA, counted, tombstone, firstB, laterB}
	duplicateReceipt := block.NewPublicationReceipt()
	duplicateReceipt.ResolveExisted(nil, []bool{true, false, true, true, false, true, true}, true, 1)
	put := newWorldPut(duplicateReceipt, batch, batch[1:], 0)
	if got := put.found(duplicateReceipt.Existed()); len(got) != 1 || !got[0].Ref.EqualsRef(b) {
		t.Fatalf("duplicate puts found %v, want only b's first entry", got)
	}

	// A later duplicate that found a cannot erase its first fresh root birth.
	sameBatch := &pendingNet{lineage: &pendingNet{puts: []worldPut{put}}, puts: epochNet.puts}
	if !sameBatch.counted(a, 0, false) {
		t.Fatal("later duplicate erased the first fresh lineage receipt")
	}

	// A transition from a World never named leaves a gap: the interval nets
	// from its puts alone.
	y, step := commit(x, "y")
	d.add(x, y, step)
	if _, net := d.take(y); net == nil || net.steps != nil {
		t.Fatal("composed across a gap")
	}

	// A commit that recorded no step leaves the interval to its puts too, and
	// the next one starts clean.
	e, _ := commit(y, "e")
	d.add(y, e, nil)
	f, step := commit(e, "f")
	d.add(e, f, step)
	if _, net := d.take(f); net == nil || net.steps != nil {
		t.Fatal("composed over a forfeited commit")
	}
	g, step := commit(f, "g")
	d.add(f, g, step)
	if _, net := d.take(g); net == nil || !slices.EqualFunc(dead(net), []*block.BlockRef{f}, (*block.BlockRef).EqualsRef) {
		t.Fatalf("next interval: %v", net)
	}

	// A World that leaves its root and returns keeps the loop, as a presence
	// toggling online and offline does: the page the loop created dies.
	h, step := commit(g, "h")
	d.add(g, h, step)
	back, step := commit(h, "g")
	d.add(h, back, step)
	if _, net := d.take(g); net == nil || !slices.EqualFunc(dead(net), []*block.BlockRef{h}, (*block.BlockRef).EqualsRef) {
		t.Fatalf("returned to its root: %v", net)
	}

	// A loop inside the chain is kept too, not cut short by the direct step.
	k, step := commit(g, "k")
	d.add(g, k, step)
	back, step = commit(k, "g")
	d.add(k, back, step)
	m, step := commit(g, "m")
	d.add(g, m, step)
	_, net = d.take(m)
	if got := dead(net); len(got) != 2 || !slices.ContainsFunc(got, g.EqualsRef) || !slices.ContainsFunc(got, k.EqualsRef) {
		t.Fatalf("loop inside the chain: %v", got)
	}
}
