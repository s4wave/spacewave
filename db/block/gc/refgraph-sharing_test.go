package block_gc

import (
	"context"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// testHolders is a HolderSource over fixed answers: the blocks the store
// holds, and the block each owner subject implies.
type testHolders struct {
	// stored is the set of blocks the store holds.
	stored map[string]bool
	// implied maps an owner subject to the block it implies.
	implied map[string]string
}

// Stored reports whether block is in the stored set.
func (h *testHolders) Stored(_ context.Context, block string) (bool, error) {
	return h.stored[block], nil
}

// Implied returns the block owner implies, whatever its object.
func (h *testHolders) Implied(_ context.Context, owner, _ string) (string, error) {
	return h.implied[owner], nil
}

// sharingModel is the oracle of the sharing property test: the holder edges
// the graph holds, from which it derives each block's holders.
type sharingModel struct {
	// holders answers the graph's Implied questions.
	holders *testHolders
	// lists is each parent block's set of children.
	lists map[string]map[string]bool
	// owners is the set of owner edges from a subject that is not a block.
	owners map[RefEdge]bool
}

// count returns the holders of block.
func (m *sharingModel) count(block string) int {
	// Count the lists that name it.
	var n int
	for _, children := range m.lists {
		if children[block] {
			n++
		}
	}

	// Count the owner edges to it and the edges that imply it.
	for edge := range m.owners {
		if edge.Object == block {
			n++
		}
		if m.holders.implied[edge.Subject] == block {
			n++
		}
	}
	return n
}

// TestSharingWalkProperty builds a shared graph without counts, then walks it
// one record per slice while random writes add and remove holders, twice over.
// After each walk every count must be at least its block's holders minus one,
// and exact when no holder was removed: an under-count could free a live
// block. Writes during counting add holds as the incremental writers do.
func TestSharingWalkProperty(t *testing.T) {
	for _, removals := range []bool{false, true} {
		for seed := range uint64(24) {
			t.Run("removals="+strconv.FormatBool(removals)+"/seed="+strconv.FormatUint(seed, 10), func(t *testing.T) {
				testSharingWalkProperty(t, seed, removals)
			})
		}
	}
}

func testSharingWalkProperty(t *testing.T, seed uint64, removals bool) {
	// Open the graph.
	ctx := t.Context()
	store := store_kvtx_inmem.NewStore()
	rg := NewRefGraph(store)
	t.Cleanup(func() { rg.Close() })
	rng := rand.New(rand.NewPCG(seed, 7))

	// Name twelve blocks and four pairs of deadlist subjects, each pair
	// implying the same old root, so a rename keeps its holder.
	blocks := make([]string, 12)
	for i := range blocks {
		blocks[i] = BlockIRI(testBlockRef(t, "sharing "+strconv.Itoa(i)))
	}
	holders := &testHolders{stored: make(map[string]bool), implied: make(map[string]string)}
	deadlists := make([]string, 8)
	for i := range deadlists {
		deadlists[i] = ObjectIRI("deadlist/" + strconv.Itoa(i))
		holders.implied[deadlists[i]] = blocks[i/2]
	}
	rg.SetHolderSource(holders)
	m := &sharingModel{holders: holders, lists: make(map[string]map[string]bool), owners: make(map[RefEdge]bool)}

	// apply commits one batch, then adds a hold for each new holder of a
	// block that already had one, as the writers do while counting.
	apply := func(adds, removes []RefEdge, gained []string) {
		// Count the gaining blocks' holders before the batch.
		t.Helper()
		before := make(map[string]int)
		for _, b := range gained {
			if _, ok := before[b]; !ok {
				before[b] = m.count(b)
			}
		}

		// Commit the batch, then hold each block that already had a holder.
		if err := rg.ApplyRefBatch(ctx, adds, removes); err != nil {
			t.Fatal(err)
		}
		var holds []string
		for _, b := range gained {
			if before[b] != 0 {
				holds = append(holds, b)
			}
			before[b]++
		}
		if err := rg.AddHolds(ctx, holds); err != nil {
			t.Fatal(err)
		}
	}

	// write makes one random change, keeping the model in step.
	pick := func(s []string) string { return s[rng.IntN(len(s))] }
	write := func() {
		t.Helper()
		ops := 4
		if removals {
			ops = 7
		}
		switch rng.IntN(ops) {
		case 0:
			// Add a child to a list.
			parent, child := pick(blocks), pick(blocks)
			if parent == child {
				return
			}
			var gained []string
			if m.lists[parent] == nil {
				m.lists[parent] = make(map[string]bool)
			}
			if !m.lists[parent][child] {
				gained = []string{child}
			}
			apply([]RefEdge{{Subject: parent, Object: child}}, nil, gained)
			m.lists[parent][child] = true
		case 1, 2:
			// Add an owner edge, from an object or a deadlist record.
			subject := ObjectIRI("object/" + strconv.Itoa(rng.IntN(4)))
			if rng.IntN(2) == 0 {
				subject = pick(deadlists)
			}
			edge := RefEdge{Subject: subject, Object: pick(blocks)}
			var gained []string
			if !m.owners[edge] {
				gained = []string{edge.Object}
				if implied := holders.implied[subject]; implied != "" {
					gained = append(gained, implied)
				}
			}
			apply([]RefEdge{edge}, nil, gained)
			m.owners[edge] = true
		case 3:
			// Stage a block, which holds nothing.
			apply([]RefEdge{{Subject: NodeUnreferenced, Object: pick(blocks)}}, nil, nil)
		case 4:
			// Remove a child from a list.
			parent := pick(blocks)
			for child := range m.lists[parent] {
				apply(nil, []RefEdge{{Subject: parent, Object: child}}, nil)
				delete(m.lists[parent], child)
				return
			}
		case 5:
			// Remove an owner edge.
			for edge := range m.owners {
				apply(nil, []RefEdge{edge}, nil)
				delete(m.owners, edge)
				return
			}
		case 6:
			// Rename a deadlist record to its pair, which keeps its holders.
			for edge := range m.owners {
				i := indexOf(deadlists, edge.Subject)
				if i < 0 {
					continue
				}
				renamed := RefEdge{Subject: deadlists[i^1], Object: edge.Object}
				if m.owners[renamed] {
					continue
				}
				apply([]RefEdge{renamed}, []RefEdge{edge}, nil)
				delete(m.owners, edge)
				m.owners[renamed] = true
				return
			}
		}
	}

	// Walk the counts twice: from stale counts, then again after a stop.
	for round := range 2 {
		// Write without counts, then begin the walk.
		if err := rg.StopSharing(ctx); err != nil {
			t.Fatal(err)
		}
		for range 40 {
			write()
		}
		s, err := rg.BeginSharing(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Interleave one-record slices with writes until the walk ends.
		for steps := 0; s.Phase.Running(); steps++ {
			if steps > 100000 {
				t.Fatalf("round %d: walk did not end, phase %c", round, s.Phase)
			}
			if rng.IntN(2) == 0 {
				write()
				continue
			}
			if s, err = rg.WalkSharing(ctx, func() bool { return true }); err != nil {
				t.Fatal(err)
			}
		}
		if s.Phase != SharingComplete || s.Doubt {
			t.Fatalf("round %d: walk ended in phase %c doubt %v", round, s.Phase, s.Doubt)
		}

		// Write while counting, then check every count against the holders.
		for range 40 {
			write()
		}
		for _, b := range blocks {
			got, err := rg.GetHolds(ctx, b)
			if err != nil {
				t.Fatal(err)
			}
			want := uint64(max(m.count(b)-1, 0))
			if got < want || (!removals && got != want) {
				t.Fatalf("round %d: %s holds %d, holders %d", round, FormatIRI(b), got, m.count(b))
			}
		}

		// The walk leaves no seen record behind.
		for key := range snapshotRefGraphRecords(t, store) {
			if key[0] == kindSeen {
				t.Fatalf("round %d: seen record %q remains", round, key)
			}
		}
	}
}

// indexOf returns the index of s in list, or -1.
func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// TestSharingWalkGates checks the counts' writers and the walk's doubt: an
// empty graph completes at once; while walking, AddHolds adds nothing and
// ChangeHolds refuses; a stored block with no child list puts the counts in
// doubt, which outlives the walk, while a hole or a tracked leaf does not.
func TestSharingWalkGates(t *testing.T) {
	// An empty graph completes its counts at once.
	ctx := t.Context()
	rg := newTestRefGraph(t)
	s, err := rg.BeginSharing(ctx)
	if err != nil || s.Phase != SharingComplete {
		t.Fatalf("empty graph phase %c err %v", s.Phase, err)
	}

	// Track a leaf, a hole and a stored block of unknown references, each
	// held twice.
	leaf, hole, unknown := BlockIRI(testBlockRef(t, "leaf")), BlockIRI(testBlockRef(t, "hole")), BlockIRI(testBlockRef(t, "unknown"))
	holders := &testHolders{stored: map[string]bool{leaf: true, unknown: true}}
	rg.SetHolderSource(holders)
	adds := []RefEdge{{Subject: NodeUnreferenced, Object: leaf}}
	for _, b := range []string{leaf, hole} {
		adds = append(adds, RefEdge{Subject: ObjectIRI("a"), Object: b}, RefEdge{Subject: ObjectIRI("b"), Object: b})
	}
	if err := rg.ApplyRefBatch(ctx, adds, nil); err != nil {
		t.Fatal(err)
	}
	addTestRoot(t, rg, NodeGCRoot)

	// While walking, AddHolds adds nothing and ChangeHolds refuses.
	if _, err := rg.BeginSharing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rg.AddHolds(ctx, []string{leaf}); err != nil {
		t.Fatal(err)
	}
	if n, err := rg.GetHolds(ctx, leaf); err != nil || n != 0 {
		t.Fatalf("hold added while walking: %d %v", n, err)
	}
	if _, err := rg.ChangeHolds(ctx, map[string]int64{leaf: -1}); !errors.Is(err, ErrSharingWalk) {
		t.Fatalf("ChangeHolds while walking: %v", err)
	}

	// walk runs the walk to its end.
	walk := func() SharingState {
		t.Helper()
		for {
			s, err := rg.WalkSharing(ctx, func() bool { return false })
			if err != nil {
				t.Fatal(err)
			}
			if !s.Phase.Running() {
				return s
			}
		}
	}

	// A hole, a tracked leaf and a root that is not a block leave the counts
	// certain.
	if s := walk(); s.Phase != SharingComplete || s.Doubt {
		t.Fatalf("walk ended in phase %c doubt %v", s.Phase, s.Doubt)
	}
	for _, b := range []string{leaf, hole} {
		if n, err := rg.GetHolds(ctx, b); err != nil || n != 1 {
			t.Fatalf("%s holds %d %v, want 1", FormatIRI(b), n, err)
		}
	}

	// A stored block of unknown references puts the walk in doubt, which
	// stays after the walk ends.
	if err := rg.AddRef(ctx, ObjectIRI("a"), unknown); err != nil {
		t.Fatal(err)
	}
	if err := rg.StopSharing(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := rg.ChangeHolds(ctx, map[string]int64{leaf: -1}); !errors.Is(err, ErrSharingWalk) {
		t.Fatalf("ChangeHolds while stopped: %v", err)
	}
	if _, err := rg.BeginSharing(ctx); err != nil {
		t.Fatal(err)
	}
	if s := walk(); s.Phase != SharingComplete || !s.Doubt {
		t.Fatalf("walk ended in phase %c doubt %v, want doubt", s.Phase, s.Doubt)
	}
	if s, err := rg.GetSharing(ctx); err != nil || !s.Doubt {
		t.Fatalf("doubt not kept: %v %v", s.Doubt, err)
	}

	// A registered root naming a block tracks it, and the walk, which cannot
	// count it, ends in doubt. Beginning again while it forgets keeps its
	// place.
	rg = newTestRefGraph(t)
	addTestRoot(t, rg, leaf)
	if s, err = rg.BeginSharing(ctx); err != nil || s.Phase != SharingClearingHolds {
		t.Fatalf("rooted graph phase %c err %v, want a walk", s.Phase, err)
	}
	for s.Phase != SharingForgetting {
		if s, err = rg.WalkSharing(ctx, func() bool { return true }); err != nil {
			t.Fatal(err)
		}
	}
	if s, err := rg.BeginSharing(ctx); err != nil || s.Phase != SharingForgetting {
		t.Fatalf("begin while forgetting gave phase %c err %v", s.Phase, err)
	}
	if s := walk(); s.Phase != SharingComplete || !s.Doubt {
		t.Fatalf("rooted walk ended in phase %c doubt %v, want doubt", s.Phase, s.Doubt)
	}
}
