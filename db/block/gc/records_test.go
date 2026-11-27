package block_gc

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestNormalizeChildrenAllocs checks that normalizing a page's child list
// allocates its output once, at 32 and 128 children, rather than a block key
// per comparison inside the writer.
func TestNormalizeChildrenAllocs(t *testing.T) {
	for _, n := range []int{32, 128} {
		// Build n children and repeat the first.
		children := make([]*block.BlockRef, 0, n+1)
		for i := range n {
			ref, err := block.BuildBlockRef([]byte("child "+strconv.Itoa(i)), nil)
			if err != nil {
				t.Fatal(err)
			}
			children = append(children, ref)
		}
		children = append(children, children[0])

		// Normalize them and count the allocations.
		got := normalizeChildren(children)
		if len(got) != n {
			t.Fatalf("%d children normalized to %d", n, len(got))
		}
		if allocs := testing.AllocsPerRun(20, func() { normalizeChildren(children) }); allocs > 1 {
			t.Errorf("normalizing %d children made %v allocations, want 1", n, allocs)
		}
	}
}

// TestReleasePendingCountsUnmovedChildren checks that releasePending counts
// a key once exactly when it is a child the paged release has not moved yet,
// as scanReleasePending lists them, for a release of three pages resumed at
// its start, mid-page, at a page boundary and at its last child. A key it
// misses would leave the freed block's holder uncounted, so the free could
// take a block the release still holds. The keys mix moved, pending and
// absent blocks across every page.
func TestReleasePendingCountsUnmovedChildren(t *testing.T) {
	ctx := context.Background()
	const n = 2*releasePageChildren + 500
	refs := make([]*block.BlockRef, 0, n+n/10)
	for i := range n + n/10 {
		ref, err := block.BuildBlockRef([]byte("release child "+strconv.Itoa(i)), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	// The first n are the release's children; the rest are absent from it.
	children := normalizeChildren(refs[:n])
	all := normalizeChildren(refs)
	nexts := []uint64{0, 700, releasePageChildren, releasePageChildren + 1, n - 1}
	edges := make(map[string]bool)
	for _, next := range nexts {
		edges[string(BlockKey(children[next]))] = true
		if next != 0 {
			edges[string(BlockKey(children[next-1]))] = true
		}
	}
	var keys [][]byte
	for i, ref := range all {
		if key := BlockKey(ref); i%3 == 0 || edges[string(key)] {
			keys = append(keys, key)
		}
	}

	for _, next := range nexts {
		t.Run(strconv.FormatUint(next, 10), func(t *testing.T) {
			tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			rec := NewRecords(tx, []byte("l/"))
			if err := rec.putReleasePages(ctx, children); err != nil {
				t.Fatal(err)
			}
			rel := &Release{Block: []byte("freed"), Next: next, Children: n}
			want := make(map[string]int64)
			if err := rec.scanReleasePending(ctx, rel, func(child *block.BlockRef) error {
				want[string(BlockKey(child))] = 1
				return nil
			}); err != nil {
				t.Fatal(err)
			}

			got, err := releasePending(ctx, rec, &LivenessState{Release: rel}, keys)
			if err != nil {
				t.Fatal(err)
			}
			var counted int
			for _, key := range keys {
				if got[string(key)] != want[string(key)] {
					t.Errorf("key %x: pending %d, want %d", key, got[string(key)], want[string(key)])
				}
				counted += int(want[string(key)])
			}
			if counted == 0 || len(got) != counted {
				t.Fatalf("%d keys pending, want %d of them", len(got), counted)
			}
		})
	}
}

// TestCheckStoredCountsLiveQueued checks that the audit counts a block the
// walk reached, at zero holders and ripe, as live and queued only when its
// queue entry sits at the count's ripe sequence, the entry the free would
// pop. An entry at another sequence frees nothing, so it counts nothing.
func TestCheckStoredCountsLiveQueued(t *testing.T) {
	ctx := context.Background()
	ref, err := block.BuildBlockRef([]byte("live queued block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	key := BlockKey(ref)
	const ripe = 5
	for _, tc := range []struct {
		name    string
		queueAt uint64
		want    uint64
	}{
		{"at its ripe", ripe, 1},
		{"at another ripe", ripe + 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			rec := NewRecords(tx, []byte("l/"))
			if err := rec.SetHolders(ctx, ref, 0, ripe); err != nil {
				t.Fatal(err)
			}
			if err := rec.enqueue(ctx, tc.queueAt, key); err != nil {
				t.Fatal(err)
			}

			var res AuditResult
			mark := &Mark{Live: map[string]struct{}{string(key): {}}}
			state := &LivenessState{UpCursor: ripe + 5}
			if err := res.checkStored(ctx, rec, state, ref, mark, nil, newReleaseCursor(rec, nil), true, nil); err != nil {
				t.Fatal(err)
			}
			if res.LiveQueued != tc.want {
				t.Fatalf("LiveQueued %d, want %d", res.LiveQueued, tc.want)
			}
		})
	}
}

// TestReadsAnswerEachKey checks that the reads the collector decides on
// answer for the record they name: a holder row reports the queue, released
// and parked keys at its count's ripe sequence each in its own field, and
// each batched read reports every key at its index as its point read does,
// the last one included.
func TestReadsAnswerEachKey(t *testing.T) {
	ctx := t.Context()
	keys, err := store_kvkey.NewKVKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	rec := NewRecords(tx, []byte("l/"))
	blocks := block_store_kvtx.NewKVTxBlock(keys, kvtx.NewTxStore(tx), 0)
	if err := rec.SetState(ctx, &LivenessState{Generation: Generation_GENERATION_OPEN, UpCursor: 10}); err != nil {
		t.Fatal(err)
	}

	// Give each of three blocks at zero, ripe at 5, one proof record.
	const ripe = 5
	proofs := []struct {
		name string
		key  func(uint64, []byte) []byte
		want func(HolderRow) bool
	}{
		{"queued", rec.queueKey, func(r HolderRow) bool { return r.Queued && !r.Released && !r.Parked }},
		{"released", rec.releasedKey, func(r HolderRow) bool { return !r.Queued && r.Released && !r.Parked }},
		{"parked", rec.parkedKey, func(r HolderRow) bool { return !r.Queued && !r.Released && r.Parked }},
	}
	refs := make([]*block.BlockRef, len(proofs))
	for i, proof := range proofs {
		ref, _, err := blocks.PutBlock(ctx, []byte(proof.name+" block"), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = ref
		for _, err := range []error{
			rec.PutChildren(ctx, ref, nil),
			rec.SetHolders(ctx, ref, 0, ripe),
			tx.Set(ctx, proof.key(ripe, BlockKey(ref)), nil),
		} {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, proof := range proofs {
		row, err := readHolderRow(ctx, rec, blocks, 10, refs[i])
		if err != nil {
			t.Fatal(err)
		}
		if !proof.want(row) {
			t.Errorf("%s row: queued %v released %v parked %v", proof.name, row.Queued, row.Released, row.Parked)
		}
	}

	// Read the lists and counts of the three, an unlisted block between
	// them and none after, in batches and one at a time.
	unlisted, err := block.BuildBlockRef([]byte("unlisted block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	bkeys := [][]byte{BlockKey(refs[0]), BlockKey(unlisted), BlockKey(refs[1]), BlockKey(refs[2])}
	listed, err := rec.listedBatch(ctx, bkeys)
	if err != nil {
		t.Fatal(err)
	}
	lists, found, err := rec.childListsBatch(ctx, bkeys)
	if err != nil {
		t.Fatal(err)
	}
	counts, counted, err := rec.countsBatch(ctx, bkeys)
	if err != nil {
		t.Fatal(err)
	}
	holders, held, err := rec.holdersBatch(ctx, bkeys)
	if err != nil {
		t.Fatal(err)
	}
	for i, key := range bkeys {
		list, ok, err := rec.childList(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		count, countOK, err := tx.Get(ctx, rec.key(kindCount, key))
		if err != nil {
			t.Fatal(err)
		}
		if want := i != 1; ok != want || listed[i] != want || found[i] != want || countOK != want || counted[i] != want || held[i] != want {
			t.Errorf("key %d: listed %v, batch listed %v, found %v; counted %v, batch %v, holders %v; want %v", i, ok, listed[i], found[i], countOK, counted[i], held[i], want)
		}
		if !bytes.Equal(lists[i], list) || !bytes.Equal(counts[i], count) || !bytes.Equal(holders[i], count) {
			t.Errorf("key %d: batch read %x %x %x, point read %x %x", i, lists[i], counts[i], holders[i], list, count)
		}
	}
}

// TestRequeueMovesStillParked checks that once an adopted Volume's
// generation left Open, the requeue moves a block its transaction still reads
// parked back onto the free queue at its ripe sequence, and skips one whose
// parked key a Land deleted since the page was read.
func TestRequeueMovesStillParked(t *testing.T) {
	ctx := t.Context()
	tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	rec := NewRecords(tx, []byte("l/"))
	state := &LivenessState{
		Adoption:   Adoption_ADOPTION_ADOPTED,
		Generation: Generation_GENERATION_SWEEPING,
		UpCursor:   10,
	}
	if err := rec.SetState(ctx, state); err != nil {
		t.Fatal(err)
	}
	parked := []struct {
		key  []byte
		ripe uint64
	}{{[]byte("parked block"), 5}, {[]byte("stored again"), 7}}
	for _, p := range parked {
		if err := tx.Set(ctx, rec.parkedKey(p.ripe, p.key), nil); err != nil {
			t.Fatal(err)
		}
	}
	page := pageIn(ctx, t, rec, kindParked)
	if err := tx.Delete(ctx, rec.parkedKey(parked[1].ripe, parked[1].key)); err != nil {
		t.Fatal(err)
	}

	var res CollectResult
	if _, _, _, err := NewCollector(nil, CollectorOptions{Frees: true}).requeueSlice(ctx, rec, &res, nil, page); err != nil {
		t.Fatal(err)
	}
	if res.Requeued != 1 {
		t.Fatalf("requeued %d, want the one still parked", res.Requeued)
	}
	for i, p := range parked {
		queued, err := rec.hasQueueKey(ctx, p.ripe, p.key)
		if err != nil {
			t.Fatal(err)
		}
		still, err := rec.hasParkedKey(ctx, p.ripe, p.key)
		if err != nil {
			t.Fatal(err)
		}
		if queued != (i == 0) || still {
			t.Errorf("%s: queued %v, parked %v; want queued %v and not parked", p.key, queued, still, i == 0)
		}
	}
}

// TestSettleWaitsForNetting checks that neither the front pop, the free nor
// the flip runs while the free could not: a paged release in progress (the model's
// FlipSettled, mutant rps-noflipsettled), an adoption younger than the grace
// period, or a rewritten list awaiting its recount. Each blocker leaves a
// listed block at zero, queued at a ripe sequence the up cursor passed, so
// the free proof would pop it and the flip would follow, unless the check
// holds; otherwise the free parks it, which the flip requires. A recount
// owed holds the park too, since the zero may miss a holder. Collect nets
// before the settle runs, so no writer reaches these states with work owed;
// the test writes them as a netting step cut short would.
func TestSettleWaitsForNetting(t *testing.T) {
	ctx := context.Background()
	ref, err := block.BuildBlockRef([]byte("front block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	key := BlockKey(ref)
	blockers := []struct {
		name  string
		block func(rec *Records, state *LivenessState) error
	}{
		{"a paged release", func(_ *Records, state *LivenessState) error {
			state.Release = &Release{Block: key}
			return nil
		}},
		{"an adoption younger than the grace period", func(_ *Records, state *LivenessState) error {
			state.AdoptAtUnixMilli = time.Now().UnixMilli()
			return nil
		}},
		{"a list rewrite awaiting a recount", func(rec *Records, _ *LivenessState) error {
			return rec.tx.Set(ctx, rec.key(kindRewritten), nil)
		}},
		{"nothing", nil},
	}
	for _, bc := range blockers {
		t.Run(bc.name, func(t *testing.T) {
			// An adopted Open Volume with the run complete and one listed,
			// unheld, ripe block on the front.
			tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			rec := NewRecords(tx, []byte("l/"))
			state := &LivenessState{
				Adoption:   Adoption_ADOPTION_ADOPTED,
				Generation: Generation_GENERATION_OPEN,
				UpCursor:   10,
			}
			for _, err := range []error{
				rec.PutChildren(ctx, ref, nil),
				rec.SetHolders(ctx, ref, 0, 5),
				tx.Set(ctx, rec.queueKey(5, key), nil),
				tx.Set(ctx, rec.key(kindFront, key), nil),
				tx.Set(ctx, rec.key(kindUnmarked, runComplete), nil),
				tx.Set(ctx, rec.key(kindHolesClear), nil),
			} {
				if err != nil {
					t.Fatal(err)
				}
			}
			if bc.block != nil {
				if err := bc.block(rec, state); err != nil {
					t.Fatal(err)
				}
			}
			if err := rec.SetState(ctx, state); err != nil {
				t.Fatal(err)
			}

			// Pop the front, park what the free finds, then try the flip.
			c := NewCollector(nil, CollectorOptions{Frees: true})
			var res CollectResult
			if _, _, err := c.frontSlice(ctx, rec, nil, &res, frontPos{}, pageIn(ctx, t, rec, kindFront)); err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := c.freeSlice(ctx, rec, nil, &res, queuedIn(ctx, t, rec)); err != nil {
				t.Fatal(err)
			}
			if err := c.flip(ctx, rec, &res, proofIn(ctx, t, c, rec)); err != nil {
				t.Fatal(err)
			}
			held := bc.block != nil
			if held && (res.Popped != 0 || res.Parked != 0 || res.Flipped) {
				t.Fatalf("with %s, popped %d, parked %d and flipped %v; want none", bc.name, res.Popped, res.Parked, res.Flipped)
			}
			if !held && (res.Popped != 1 || !res.Flipped) {
				t.Fatalf("with netting done, popped %d and flipped %v; want the pop and the flip", res.Popped, res.Flipped)
			}
		})
	}
}

// TestFlipRefusesWriteAfterProof checks that the flip refuses a proof that a
// writer outdated between the snapshot and the flip: a put pushing a block
// onto the front, an audit recording a hole, and an adoption queueing a block
// at zero, each run as the FlipProof spec's writers do. The flip's
// transaction seeks none of those kinds, so only the add revision, the
// holes-clear mark and the adoption mark can show the write; the spec's
// mutants drop each. Each writer runs through its own Records, as its own
// transaction would.
func TestFlipRefusesWriteAfterProof(t *testing.T) {
	ctx := context.Background()
	ref, err := block.BuildBlockRef([]byte("written block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	key := BlockKey(ref)
	writers := []struct {
		name  string
		write func(rec *Records) error
	}{
		{"a put pushing onto the front", func(rec *Records) error {
			return rec.addFront(ctx, key)
		}},
		{"a recorded hole", func(rec *Records) error {
			return rec.addHole(ctx, key)
		}},
		{"an adoption queueing a block", func(rec *Records) error {
			state, err := rec.GetState(ctx)
			if err != nil {
				return err
			}
			state.AdoptAtUnixMilli++
			if err := rec.tx.Set(ctx, rec.queueKey(1, key), nil); err != nil {
				return err
			}
			return rec.SetState(ctx, state)
		}},
		{"nothing", nil},
	}
	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			// An adopted Open Volume with the run complete, the holes-clear
			// mark, an empty front and an empty free queue: ready to flip.
			tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			rec := NewRecords(tx, []byte("l/"))
			state := &LivenessState{
				Adoption:         Adoption_ADOPTION_ADOPTED,
				Generation:       Generation_GENERATION_OPEN,
				AdoptAtUnixMilli: 1,
				UpCursor:         10,
			}
			for _, err := range []error{
				tx.Set(ctx, rec.key(kindUnmarked, runComplete), nil),
				tx.Set(ctx, rec.key(kindHolesClear), nil),
				rec.SetState(ctx, state),
			} {
				if err != nil {
					t.Fatal(err)
				}
			}

			// Read the proof, let the writer land, then flip.
			c := NewCollector(nil, CollectorOptions{Frees: true})
			proof := proofIn(ctx, t, c, rec)
			if !proof.read {
				t.Fatal("the snapshot found the flip's conditions unmet")
			}
			if w.write != nil {
				if err := w.write(NewRecords(tx, []byte("l/"))); err != nil {
					t.Fatal(err)
				}
			}
			var res CollectResult
			if err := c.flip(ctx, rec, &res, proof); err != nil {
				t.Fatal(err)
			}
			if want := w.write == nil; res.Flipped != want {
				t.Fatalf("after %s, flipped %v; want %v", w.name, res.Flipped, want)
			}
		})
	}
}

// TestFlipPassesOnlyWrittenOffHoles checks the proof's read of the hole
// records: with the front empty, an open hole holds the flip, alone or in
// either key order beside a written-off one, and written-off holes alone
// let the flip pass. An open hole that a verified list names is on the front
// as well, so only this read holds the flip for one that is not.
func TestFlipPassesOnlyWrittenOffHoles(t *testing.T) {
	ctx := context.Background()
	var keys [2][]byte
	for i, data := range []string{"first hole", "second hole"} {
		ref, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = BlockKey(ref)
	}
	cases := []struct {
		name string
		// off selects which of the two recorded holes are written off.
		off  [2]bool
		want bool
	}{
		{"both open", [2]bool{false, false}, false},
		{"the first written off", [2]bool{true, false}, false},
		{"the second written off", [2]bool{false, true}, false},
		{"both written off", [2]bool{true, true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// An adopted Open Volume with the run complete, an empty free
			// queue, and two recorded holes, each pushed onto the front as
			// the walk of its parent pushes it.
			tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			rec := NewRecords(tx, []byte("l/"))
			state := &LivenessState{
				Adoption:         Adoption_ADOPTION_ADOPTED,
				Generation:       Generation_GENERATION_OPEN,
				AdoptAtUnixMilli: 1,
				UpCursor:         10,
			}
			for _, err := range []error{
				tx.Set(ctx, rec.key(kindUnmarked, runComplete), nil),
				rec.SetState(ctx, state),
				rec.addHole(ctx, keys[0]),
				rec.addHole(ctx, keys[1]),
				rec.addFront(ctx, keys[0]),
				rec.addFront(ctx, keys[1]),
			} {
				if err != nil {
					t.Fatal(err)
				}
			}

			// Write off the selected holes, and take the others off the
			// front, as a hole no verified list names is.
			for i, key := range keys {
				if !tc.off[i] {
					if err := tx.Delete(ctx, rec.key(kindFront, key)); err != nil {
						t.Fatal(err)
					}
					continue
				}
				if hole, err := rec.writeOffHole(ctx, key); err != nil || !hole {
					t.Fatalf("write-off found a hole %v, err %v", hole, err)
				}
				if pushed, err := rec.pushOwed(ctx, key); err != nil || pushed {
					t.Fatalf("a push of the written-off hole pushed %v, err %v; want none", pushed, err)
				}
			}

			// Read the proof and flip.
			c := NewCollector(nil, CollectorOptions{Frees: true})
			var res CollectResult
			if err := c.flip(ctx, rec, &res, proofIn(ctx, t, c, rec)); err != nil {
				t.Fatal(err)
			}
			if res.Flipped != tc.want {
				t.Fatalf("flipped %v, want %v", res.Flipped, tc.want)
			}
		})
	}
}

// TestNettingKeepsParkOnRaiseToZero checks that a block parked while Open,
// whose park released its list, stays parked at its ripe sequence when one
// netting gives it an up and a down: requeued, the free would park it again
// and give its child a second down, which takes the child to zero under the
// parent that still holds it, and the front would pop it for the sweep.
func TestNettingKeepsParkOnRaiseToZero(t *testing.T) {
	ctx := context.Background()
	child, err := block.BuildBlockRef([]byte("child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	parked, err := block.BuildBlockRef([]byte("parked"), nil)
	if err != nil {
		t.Fatal(err)
	}
	key := BlockKey(parked)
	tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	rec := NewRecords(tx, []byte("l/"))
	state := &LivenessState{
		Adoption:   Adoption_ADOPTION_ADOPTED,
		Generation: Generation_GENERATION_OPEN,
		UpSeq:      20,
		UpCursor:   20,
	}
	for _, err := range []error{
		rec.PutChildren(ctx, parked, []*block.BlockRef{child}),
		rec.SetHolders(ctx, parked, 0, 5),
		tx.Set(ctx, rec.releasedKey(5, key), nil),
		rec.PutChildren(ctx, child, nil),
		rec.SetHolders(ctx, child, 1, 0),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Net an up and a down of the parked block together.
	c := NewCollector(nil, CollectorOptions{Frees: true})
	var res CollectResult
	n, err := c.newNetting(rec, state, &res, sliceRecords)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.add(key, 1); err != nil {
		t.Fatal(err)
	}
	if err := n.add(key, -1); err != nil {
		t.Fatal(err)
	}
	if err := n.write(ctx); err != nil {
		t.Fatal(err)
	}

	// It stays parked where it was, off the queue, and a free pass gives
	// its child no second down.
	holders, ripe, err := rec.GetHolders(ctx, parked)
	if err != nil || holders != 0 || ripe != 5 {
		t.Fatalf("parked block at %d ripe %d err %v, want 0 ripe 5", holders, ripe, err)
	}
	if found, err := tx.Exists(ctx, rec.releasedKey(5, key)); err != nil || !found {
		t.Fatalf("released key found %v err %v, want kept", found, err)
	}
	if found, err := tx.Exists(ctx, rec.queueKey(20, key)); err != nil || found {
		t.Fatalf("queue entry found %v err %v, want none", found, err)
	}
	if _, _, _, err := c.freeSlice(ctx, rec, nil, &res, queuedIn(ctx, t, rec)); err != nil {
		t.Fatal(err)
	}
	if res.Parked != 0 {
		t.Fatalf("free pass parked %d, want none", res.Parked)
	}
}

// TestWalkRestartsUnreadableProgress checks that a walk whose scratch holds a
// progress record this build cannot read, as a build with another WalkMark
// leaves, drops it with the run's records and begins the run again, instead
// of failing every open until someone deletes the store.
func TestWalkRestartsUnreadableProgress(t *testing.T) {
	// Leave a progress record that does not decode, beside an edge.
	ctx := context.Background()
	scratch := store_kvtx_inmem.NewStore()
	tx, err := scratch.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	edge := walkKey(walkEdge, []byte("stale edge"))
	for key, value := range map[string][]byte{string([]byte{walkProgressKind}): {0xff, 0xff}, string(edge): nil} {
		if err := tx.Set(ctx, []byte(key), value); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Open the run: it begins again and keeps nothing of the old one.
	w, err := openWalk(ctx, nil, scratch, WalkOptions{Run: "next"})
	if err != nil {
		t.Fatalf("open over unreadable progress: %v", err)
	}
	if !w.res.Restarted || w.prog.GetPhase() != WalkPhase_WALK_PHASE_ROOTS {
		t.Fatalf("restarted %v in phase %v; want a new run", w.res.Restarted, w.prog.GetPhase())
	}
	prog, err := w.readProgress(ctx)
	if err != nil || prog.GetRun() != "next" {
		t.Fatalf("progress %v, %v; want the new run's", prog, err)
	}
	tx, err = scratch.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if _, found, err := tx.Get(ctx, edge); err != nil || found {
		t.Fatalf("stale edge found %v, %v; want it cleared", found, err)
	}
}

// queuedIn returns every entry on the free queue of rec, as a test that
// frees within one transaction proposes them to freeSlice.
func queuedIn(ctx context.Context, t *testing.T, rec *Records) []queuedBlock {
	t.Helper()
	var queued []queuedBlock
	err := rec.scanQueue(ctx, nil, func(ripe uint64, ref *block.BlockRef) (bool, error) {
		queued = append(queued, queuedBlock{ripe: ripe, key: BlockKey(ref)})
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return queued
}

// pageIn reads the first page of kind's keys in rec, as viewSlices's
// snapshot reads it, for a test that runs a slice within one transaction.
func pageIn(ctx context.Context, t *testing.T, rec *Records, kind byte) *keyPage {
	t.Helper()
	page := &keyPage{}
	if err := rec.readKeys(ctx, page, kind, nil, sliceRecords, nil); err != nil {
		t.Fatal(err)
	}
	return page
}

// proofIn reads the flip's proof in rec, as readFlipProof's snapshot reads
// it, for a test that flips within one transaction.
func proofIn(ctx context.Context, t *testing.T, c *Collector, rec *Records) flipProof {
	t.Helper()
	proof, err := c.proveFlip(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	return proof
}

// TestSettleStatusFrontCauses checks every proof branch and the collector's unchanged proof mapping.
func TestSettleStatusFrontCauses(t *testing.T) {
	// Seed a small store with one front entry for each proof branch.
	ctx := t.Context()
	data := store_kvtx_inmem.NewStore()
	keys, err := store_kvkey.NewKVKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := data.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)
	rec := NewRecords(tx, []byte("l/"))
	blocks := block_store_kvtx.NewKVTxBlock(keys, kvtx.NewTxStore(tx), 0)
	cases := []struct {
		name      string
		holders   uint64
		ripe      uint64
		listed    bool
		stored    bool
		proofKind byte
		cause     FrontCause
		proof     frontProof
	}{
		{"held", 2, 0, true, false, 0, FrontHeldByHolders, frontHeld},
		{"unqueued", 0, 5, true, false, 0, FrontZeroUnqueued, frontHeld},
		{"ripening", 0, 11, true, false, 0, FrontRipening, frontRipening},
		{"damaged", 0, 5, false, true, 0, FrontDamaged, frontDamaged},
		{"queued", 0, 5, true, false, kindQueue, FrontPoppable, frontPop},
		{"released", 0, 5, true, false, kindReleased, FrontPoppable, frontPop},
		{"parked", 0, 5, true, false, kindParked, FrontPoppable, frontPop},
		{"absent", 0, 5, false, false, 0, FrontPoppable, frontPop},
		{"unborn", 1, 0, false, false, 0, FrontUnborn, frontUnborn},
	}
	collector := NewCollector(diagnosticStore{data: data, keys: keys}, CollectorOptions{})
	for _, tc := range cases {
		// Write the count, optional list and bytes, and the proof entry.
		ref, err := block.BuildBlockRef([]byte(tc.name), nil)
		if err != nil {
			t.Fatal(err)
		}
		key := BlockKey(ref)
		if err := rec.SetHolders(ctx, ref, tc.holders, tc.ripe); err != nil {
			t.Fatal(err)
		}
		if tc.listed {
			if err := rec.PutChildren(ctx, ref, nil); err != nil {
				t.Fatal(err)
			}
		}
		if tc.stored {
			if _, _, err := blocks.PutBlock(ctx, []byte(tc.name), nil); err != nil {
				t.Fatal(err)
			}
		}
		if tc.proofKind != 0 {
			if err := tx.Set(ctx, rec.key(tc.proofKind, queuePos(tc.ripe, key)), nil); err != nil {
				t.Fatal(err)
			}
		}
		if err := rec.addFront(ctx, key); err != nil {
			t.Fatal(err)
		}

		// Confirm the detailed cause maps to the same collector proof and ripe sequence.
		cause, ripe, err := readFrontCause(ctx, rec, blocks, 10, key)
		if err != nil || cause != tc.cause || ripe != tc.ripe {
			t.Fatalf("%s cause = %v ripe %d: %v", tc.name, cause, ripe, err)
		}
		proof, ripe, err := collector.frontProven(ctx, rec, blocks, 10, key)
		if err != nil || proof != tc.proof || ripe != tc.ripe {
			t.Fatalf("%s proof = %v ripe %d: %v", tc.name, proof, ripe, err)
		}
	}

	// Publish the fixture, then classify it through an actual read-only snapshot.
	if err := rec.SetState(ctx, &LivenessState{Generation: Generation_GENERATION_OPEN, UpCursor: 10}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := collector.SettleStatus(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if status.Front != len(cases) || status.FrontMore {
		t.Fatalf("front = %d more %v", status.Front, status.FrontMore)
	}
	for cause, count := range status.FrontCauses {
		want := 1
		if FrontCause(cause) == FrontPoppable {
			want = 4
		}
		if count.Count != want || count.Sample == nil {
			t.Fatalf("cause %v = %+v, want %d and a sample", FrontCause(cause), count, want)
		}
	}

	// The run is complete only once its completion entry exists.
	if status.RunComplete {
		t.Fatal("status reports a run with no completion entry complete")
	}
	tx, err = data.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, NewRecords(tx, []byte("l/")).key(kindUnmarked, runComplete), nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if status, err = collector.SettleStatus(ctx, 100); err != nil || !status.RunComplete {
		t.Fatalf("status after the completion entry: complete %v, err %v", status.RunComplete, err)
	}

	// Bound the proof scan with the same limit as the front count.
	limited, err := collector.SettleStatus(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, count := range limited.FrontCauses {
		total += count.Count
	}
	if limited.Front != 1 || !limited.FrontMore || total != 1 {
		t.Fatalf("limited front = %d more %v classified %d", limited.Front, limited.FrontMore, total)
	}
}

// TestInspectHolders checks parent ordering, bounded traversal and roots holding retained targets.
func TestInspectHolders(t *testing.T) {
	// Seed a child with ten zero, unqueued parents and a shared grandparent.
	ctx := t.Context()
	data := store_kvtx_inmem.NewStore()
	keys, err := store_kvkey.NewKVKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := data.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)
	rec := NewRecords(tx, []byte("l/"))
	blocks := block_store_kvtx.NewKVTxBlock(keys, kvtx.NewTxStore(tx), 0)
	child, err := block.BuildBlockRef([]byte("child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	parents := make([]*block.BlockRef, 0, 10)
	for i := range 10 {
		// Store each parent's bytes, verified list, count and front entry together.
		parent, _, err := blocks.PutBlock(ctx, []byte("parent "+strconv.Itoa(i)), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, err := range []error{
			rec.PutChildren(ctx, parent, []*block.BlockRef{child}),
			rec.SetHolders(ctx, parent, 0, 5),
			rec.addFront(ctx, BlockKey(parent)),
		} {
			if err != nil {
				t.Fatal(err)
			}
		}
		parents = append(parents, parent)
	}
	grandparent, err := block.BuildBlockRef([]byte("grandparent"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.PutChildren(ctx, grandparent, parents); err != nil {
		t.Fatal(err)
	}
	if err := rec.SetState(ctx, &LivenessState{Generation: Generation_GENERATION_OPEN, UpCursor: 10}); err != nil {
		t.Fatal(err)
	}
	// Name the requested child twice and its grandparent once, including a leased root.
	for _, root := range []struct {
		bucket, name string
		ref          *block.BlockRef
		lease        string
	}{
		{"a", "child", child, ""},
		{"b", "child", child, "reader"},
		{"b", "head", grandparent, ""},
		{"b", "unrelated", new(block.BlockRef), ""},
	} {
		if err := rec.SetRoot(ctx, root.bucket, root.name, &RootEntry{Ref: BlockKey(root.ref), Lease: root.lease}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Inspect through a read-only transaction and check each streamed parent.
	collector := NewCollector(diagnosticStore{data: data, keys: keys}, CollectorOptions{})
	seen := [4]int{}
	roots := make(map[FrontRoot]*block.BlockRef)
	var previous []byte
	levels, err := collector.InspectHolders(ctx, []*block.BlockRef{child, child}, 8, func(level int, named *block.BlockRef, row HolderRow) error {
		// Keep root relationships separate from the parent-level totals.
		if row.Root != nil {
			if level != -1 || named != nil || row != (HolderRow{Ref: row.Ref, Root: row.Root}) {
				t.Fatalf("root row at level %d with child %v: %+v", level, named, row)
			}
			if roots[*row.Root] != nil {
				t.Fatalf("root emitted twice: %+v", row.Root)
			}
			roots[*row.Root] = row.Ref
			return nil
		}

		// Check the structure and order of each parent level.
		seen[level]++
		if level == 1 {
			want := HolderRow{Ref: row.Ref, Ripe: 5, Listed: true, Marked: true, Stored: true, Front: true, Cause: FrontZeroUnqueued}
			if row != want || !named.EqualVT(child) {
				t.Fatalf("parent row = %+v child %v", row, named)
			}
			key := BlockKey(row.Ref)
			if previous != nil && bytes.Compare(previous, key) >= 0 {
				t.Fatal("parents are not in key order")
			}
			previous = key
		}
		if level == 2 && (!row.Ref.EqualVT(grandparent) || row.Holders != 1 || !row.Listed || !row.Marked || row.Stored || row.Front) {
			t.Fatalf("grandparent row = %+v", row)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Match roots at the requested level and at a shared ancestor only once.
	if len(roots) != 3 || !roots[FrontRoot{BucketID: "a", Name: "child"}].EqualVT(child) ||
		!roots[FrontRoot{BucketID: "b", Name: "child"}].EqualVT(child) ||
		!roots[FrontRoot{BucketID: "b", Name: "head"}].EqualVT(grandparent) {
		t.Fatalf("roots = %v", roots)
	}
	if seen != [4]int{1, 10, 8, 0} || len(levels) != 4 || levels[1].Followed != 8 || levels[2].Followed != 1 {
		t.Fatalf("seen %v levels %+v", seen, levels)
	}
	if levels[1].States[FrontZeroUnqueued] != 10 || levels[2].States[FrontHeldByHolders] != 8 {
		t.Fatalf("states = %+v", levels)
	}
}
