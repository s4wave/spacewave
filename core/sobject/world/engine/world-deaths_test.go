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
	// stored afresh.
	dead := func(net *pendingNet) []*block.BlockRef {
		return net.steps.Join().Net(nil).Replaced
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
