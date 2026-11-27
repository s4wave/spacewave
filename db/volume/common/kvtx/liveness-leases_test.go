package kvtx

import (
	"bytes"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// testLivenessLeases checks reader pins, process leases, and staged holders.
func testLivenessLeases(t *testing.T) {
	t.Helper()

	t.Run("reader pin", func(t *testing.T) {
		// Store R1 -> E under main and pin it as a reader that saw it.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		r1 := tr.add(t, "R1", "E")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		unpin, err := v.PinBucketRoot(ctx, r1)
		if err != nil {
			t.Fatal(err)
		}

		// Swap main away. The pin, renewed by its live holder, keeps
		// the old tree past the grace period.
		r2 := tr.add(t, "R2")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R1", "E", "R2")

		// The pin released, the old tree goes after the grace period.
		unpin()
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R1", "E")
		requireStored(t, v, tr, true, "R2")
		requireAudit(t, v, 0)
	})

	t.Run("reader sight across an adoption", func(t *testing.T) {
		// A reader sees head R1 of main, which then moves to R2, and the
		// Volume adopts again before the reader pins R1: the adoption drops
		// the record of the move and counts R1 at zero.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		r1 := tr.add(t, "R1", "E")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		r2 := tr.add(t, "R2")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		markUnadopted(t, v)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}

		// A pass at once frees nothing the reader saw: the adoption's marker
		// record is not Grace old, so R1 has not ripened. The reader pins R1
		// within its window and reads the whole tree past the grace period.
		collectAll(t, v)
		requireStored(t, v, tr, true, "R1", "E", "R2")
		unpin, err := v.PinBucketRoot(ctx, r1)
		if err != nil {
			t.Fatal(err)
		}
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R1", "E", "R2")

		// Once the pin is released, the old tree goes after the grace
		// period.
		unpin()
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R1", "E")
		requireStored(t, v, tr, true, "R2")
		requireAudit(t, v, 0)
	})

	t.Run("lapsed lease", func(t *testing.T) {
		// A pin whose process died lapses. The collector releases it,
		// and frees the old tree only a grace period later.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		r1 := tr.add(t, "R1")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v.PinBucketRoot(ctx, r1); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		forgetPins(t, v)
		clk.Add(rootLease + time.Second)
		if res := collectAll(t, v); res.Lapsed != 1 || res.Freed != 0 {
			t.Fatalf("lapsed %d freed %d, want 1 and 0", res.Lapsed, res.Freed)
		}
		requireStored(t, v, tr, true, "R1")
		clk.Add(block_gc.DefaultGrace)
		if res := collectAll(t, v); res.Freed != 1 {
			t.Fatalf("freed %d after the lapsed pin's grace, want 1", res.Freed)
		}
	})

	t.Run("two writers", func(t *testing.T) {
		// Writer B moves main from R1 while writer A still expects R1.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		r1 := tr.add(t, "R1")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		r2 := tr.add(t, "R2")
		r3 := tr.add(t, "R3")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
			t.Fatal(err)
		}

		// A loses: its change is refused whole and stores nothing.
		err := v.SetBucketRoots(ctx, "b", tr.entries("R3"), []block.NamedRoot{{Name: "main", Ref: r3, Previous: r1}})
		if !errors.Is(err, block.ErrRootMoved) {
			t.Fatalf("stale move = %v, want ErrRootMoved", err)
		}
		requireStored(t, v, tr, false, "R3")

		// The winner's root survives collection. The old root goes.
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R2")
		requireStored(t, v, tr, false, "R1")
		requireAudit(t, v, 0)
	})

	for _, behind := range []int{0, 600} {
		t.Run("moved away and back behind "+strconv.Itoa(behind), func(t *testing.T) {
			// main moves R1 to R2 and back to R1 inside the grace period,
			// behind more commits than one net slice holds. The pass once the
			// first move's down ages nets the move back's up first, so R1
			// never reaches zero. The churn root holds X alone, so Y is
			// stored unheld.
			v, clk := newLivenessVolume(t, true)
			observePuts(v)
			ctx := t.Context()
			tr := testTree{}
			tr.add(t, "E")
			r1 := tr.add(t, "R1", "E")
			r2 := tr.add(t, "R2")
			if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2, Previous: r1}}); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace / 2)
			churn := []*block.BlockRef{tr.add(t, "X"), tr.add(t, "Y")}
			if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "Y"), []block.NamedRoot{{Name: "churn", Ref: churn[0]}}); err != nil {
				t.Fatal(err)
			}
			for i := range behind {
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "churn", Ref: churn[(i+1)%2], Previous: churn[i%2]}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: r1, Previous: r2}}); err != nil {
				t.Fatal(err)
			}

			// Net the first move's down: R1 keeps the move back's holder.
			clk.Add(block_gc.DefaultGrace/2 + time.Second)
			if res := collectAll(t, v); res.Freed != 0 {
				t.Fatalf("freed %d, want R1 kept by the move back", res.Freed)
			}
			requireStored(t, v, tr, true, "R1", "E", "R2")

			// Once the move back nets, R2 goes and R1's tree stays.
			clk.Add(block_gc.DefaultGrace)
			collectAll(t, v)
			requireStored(t, v, tr, true, "R1", "E")
			requireStored(t, v, tr, false, "R2")
			requireAudit(t, v, 0)

			// R1's tree goes once main leaves it.
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r1}}); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
			requireStored(t, v, tr, false, "R1", "E")
		})
	}

	t.Run("release and birth in different intervals", func(t *testing.T) {
		// R1 -> C -> F loses main. Just inside its grace period a new
		// R2 references F under another root. The pass once the release
		// ages nets the birth's up first, so freeing R1 and C leaves F
		// its new holder.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "F")
		tr.add(t, "C", "F")
		r1 := tr.add(t, "R1", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("F", "C", "R1"), []block.NamedRoot{{Name: "main", Ref: r1}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r1}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		r2 := tr.add(t, "R2", "F")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "other", Ref: r2}}); err != nil {
			t.Fatal(err)
		}

		// Net the release: R1 and C go, and F keeps its new holder.
		clk.Add(2 * time.Second)
		if res := collectAll(t, v); res.Freed != 2 {
			t.Fatalf("freed %d, want R1 and C", res.Freed)
		}
		requireStored(t, v, tr, false, "R1", "C")
		clk.Add(block_gc.DefaultGrace)
		collectAll(t, v)
		requireStored(t, v, tr, true, "F", "R2")
		requireAudit(t, v, 0)
	})

	t.Run("re-put of a queued block", func(t *testing.T) {
		// R -> X loses main, and just inside its grace period a writer puts X
		// again, as a writer that puts one tree across several batches does
		// for a block it already stored. The pass once the release ages
		// nets the re-put's up first, so freeing R leaves X the re-put's
		// holder, and a parent the writer puts later still finds X.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		if _, err := v.PutBlockBatch(ctx, tr.entries("X")); err != nil {
			t.Fatal(err)
		}

		// Net the release: R goes and X stays.
		clk.Add(2 * time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R")
		requireStored(t, v, tr, true, "X")

		// The writer's parent references X under main, and X stays.
		p := tr.add(t, "P", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "X")
		requireStored(t, v, tr, false, "R")
		requireAudit(t, v, 0)
	})

	t.Run("put again after the release netted", func(t *testing.T) {
		// X loses main and nets with frees off, so it is at zero on the
		// free queue. A writer then puts X again and, just inside the
		// grace period, a parent referencing it under main. The put
		// again takes X off the free queue in its own Land, and its up
		// lifts X off zero once it nets, so no pass frees X before the
		// parent's up nets.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		x := tr.add(t, "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X"), []block.NamedRoot{{Name: "main", Ref: x}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: x}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := netAll(t, v); res.Queued != 1 {
			t.Fatalf("queued %d, want X", res.Queued)
		}

		// Put X again, then the parent just inside the grace period.
		if _, err := v.PutBlockBatch(ctx, tr.entries("X")); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		p := tr.add(t, "P", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}

		// No pass frees X: first the put again holds it, then the parent.
		clk.Add(2 * time.Second)
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("freed %d of a block put again", res.Freed)
		}
		clk.Add(block_gc.DefaultGrace)
		collectAll(t, v)
		requireStored(t, v, tr, true, "X", "P")
		requireAudit(t, v, 0)
	})

	t.Run("proof of a released block", func(t *testing.T) {
		// R -> C is proved complete, then loses main and nets with frees
		// off, so R is Dead and stored. Its
		// proof goes with the release, and a proof marked while R is Dead
		// counts for nothing, so a copy neither skips nor holds R's tree on
		// a proof C may no longer back.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		r := tr.add(t, "R", "C")
		z := tr.add(t, "Z")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C", "R", "Z"), []block.NamedRoot{{Name: "main", Ref: r}, {Name: "z", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		if err := v.MarkRootsComplete(ctx, []*block.BlockRef{r, z}); err != nil {
			t.Fatal(err)
		}
		proved := func() bool {
			t.Helper()
			complete, err := v.RootsComplete(ctx, []*block.BlockRef{r})
			if err != nil {
				t.Fatal(err)
			}
			return complete[0]
		}
		if !proved() {
			t.Fatal("R not proved")
		}

		// Release R and net it with frees off, so R is Dead and queued.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		netAll(t, v)
		requireStored(t, v, tr, true, "R", "C")

		// R holds C until R itself is freed, so its proof stays true and a
		// stage may hold R on it.
		if !proved() {
			t.Fatal("released R lost its proof before its free")
		}

		// A pending proof of R counts for nothing, nor holds R under a
		// stage, and carrying the proof writes no proof. The recorded
		// proof the pending proof rides over stays. The stage opens
		// first, since its first opening starts the process lease in a
		// transaction that would carry the proof.
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := v.MarkRootsComplete(ctx, []*block.BlockRef{r}); err != nil {
			t.Fatal(err)
		}
		if proved() {
			t.Fatal("pending proof of a released R counted")
		}
		held, err := v.HoldStageCompleteRoots(ctx, stage, []*block.BlockRef{r})
		release()
		if err != nil {
			t.Fatal(err)
		}
		if held[0] {
			t.Fatal("stage held a released R on its proof")
		}
		if err := v.MarkRootsComplete(ctx, []*block.BlockRef{r, z}); err != nil {
			t.Fatal(err)
		}
		if !proved() {
			t.Fatal("carrying a proof of a released R dropped the recorded one")
		}
		requireStored(t, v, tr, true, "R", "C")

		// Once R ripens, it and C go.
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R", "C")
		requireStored(t, v, tr, true, "Z")
		requireAudit(t, v, 0)
	})

	t.Run("root moved onto a Dead block", func(t *testing.T) {
		// R -> E loses main and nets with frees off, so R is Dead and on the
		// free queue. Root k then moves onto R without a put. The move's
		// Land takes R off the free queue, so no pass frees R or E before
		// the move nets and holds them again.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		r := tr.add(t, "R", "E")
		z := tr.add(t, "Z")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("E", "R", "Z"), []block.NamedRoot{{Name: "main", Ref: r}, {Name: "z", Ref: z}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		netAll(t, v)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "k", Ref: r}}); err != nil {
			t.Fatal(err)
		}

		// The move dequeued R, so no pass frees it. Once the move nets,
		// R and E are held again.
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("freed %d of a chain a root moved onto", res.Freed)
		}
		clk.Add(2 * time.Second)
		collectAll(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "E", "Z")
		requireAudit(t, v, 0)
	})

	t.Run("shared child in one batch", func(t *testing.T) {
		// R -> {A, B}, A -> X, B -> X are born in one batch, and side holds
		// A too. X has two parents born beside it, so it is counted twice.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		a := tr.add(t, "A", "X")
		tr.add(t, "B", "X")
		r := tr.add(t, "R", "A", "B")
		roots := []block.NamedRoot{{Name: "main", Ref: r}, {Name: "side", Ref: a}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "A", "B", "R"), roots); err != nil {
			t.Fatal(err)
		}

		// Releasing main frees R and B. A and X stay under side.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R", "B")
		requireStored(t, v, tr, true, "A", "X")
		requireAudit(t, v, 0)

		// Releasing side frees the rest.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "side", Previous: a}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "A", "X")
	})

	t.Run("stage", func(t *testing.T) {
		// A stage builds C, then P -> C. main adopts P, and the stage's
		// release leaves P's tree to main. A second build the stage
		// releases unadopted goes after the grace period.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tr := testTree{}
		tr.add(t, "C")
		p := tr.add(t, "P", "C")
		tr.add(t, "Q")
		for _, name := range []string{"C", "P", "Q"} {
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries(name)); err != nil {
				t.Fatal(err)
			}
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}

		// Inside the stage's life nothing is freed. After its release
		// and the grace period only the unadopted build goes.
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "C", "P", "Q")
		release()
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "C", "P")
		requireStored(t, v, tr, false, "Q")
		requireAudit(t, v, 0)
	})

	for _, tc := range []struct {
		name string
		mode sliceMode
	}{
		{"in slices with collection between slices", sliceCollect},
		{"in slices cancelled then retried", sliceCancelRetry},
		{"in slices cancelled then released", sliceCancelRelease},
		{"in slices released between slices", sliceRelease},
		{"parent first in one transaction", sliceParentFirst},
		{"in one transaction under StageOneTransaction", sliceOneTransaction},
	} {
		t.Run("stage lands "+tc.name, func(t *testing.T) {
			testStageSlices(t, tc.mode)
		})
	}

	t.Run("underflow in one free slice", func(t *testing.T) {
		// A -> B under main, with B also at zero as if a holder had vanished
		// from its count. Releasing main brings A to zero, and its free
		// takes B below zero: the slice keeps B with one holder as an
		// underflow, and the free that follows must not free B.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		for i := 0; ; i++ {
			child := "U" + strconv.Itoa(i)
			tr.add(t, child)
			tr.add(t, "A", child)
			if bytes.Compare(block_gc.BlockKey(tr["A"].Ref), block_gc.BlockKey(tr[child].Ref)) < 0 {
				tr["B"] = tr[child]
				break
			}
		}
		a := tr["A"].Ref
		if err := v.SetBucketRoots(ctx, "b", tr.entries("B", "A"), []block.NamedRoot{{Name: "main", Ref: a}}); err != nil {
			t.Fatal(err)
		}
		err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
			return p.rec.SetHolders(ctx, tr["B"].Ref, 0, 0)
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: a}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Underflows != 1 {
			t.Fatalf("collection found %d underflows, want 1", res.Underflows)
		}
		requireStored(t, v, tr, false, "A")
		requireStored(t, v, tr, true, "B")
	})

	t.Run("read fills a tree main holds", func(t *testing.T) {
		// D is stored under old. A read fills B, which references C,
		// which references D. main moves onto B without putting its
		// tree. A later read fills C. old is released. D keeps the
		// holder C's fill gave it, and the fills are garbage only once
		// nothing holds them.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		d := tr.add(t, "D")
		tr.add(t, "C", "D")
		b := tr.add(t, "B", "C")
		fill := func(name string) {
			t.Helper()
			err := v.withDirectFill(ctx, func(p *physicalTx) error {
				_, _, err := v.landFill(ctx, p, "b", tr.entries(name))
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("D"), []block.NamedRoot{{Name: "old", Ref: d}}); err != nil {
			t.Fatal(err)
		}
		fill("B")
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: b}}); err != nil {
			t.Fatal(err)
		}
		fill("C")
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "old", Previous: d}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(fillLapse)
		collectAll(t, v)
		requireStored(t, v, tr, true, "B", "C", "D")
		requireAudit(t, v, 0)

		// Releasing main frees the whole tree.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: b}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "B", "C", "D")
	})

	t.Run("unheld fill", func(t *testing.T) {
		// A read fills E, which nothing else holds. Landed with no root
		// it is refused. The fill's leased root holds it for the grace
		// period, and once the root lapses it goes after the grace
		// period, as an unheld put does.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "E")
		err := v.withDirectFill(ctx, func(p *physicalTx) error {
			_, err := v.land(ctx, p, "", tr.entries("E"), nil)
			return err
		})
		if !errors.Is(err, block.ErrUnheldPut) {
			t.Fatalf("fill with no root returned %v, want ErrUnheldPut", err)
		}
		err = v.withDirectFill(ctx, func(p *physicalTx) error {
			_, _, err := v.landFill(ctx, p, "b", tr.entries("E"))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		collectAll(t, v)
		requireStored(t, v, tr, true, "E")
		requireAudit(t, v, 0)
		clk.Add(fillLapse)
		collectAll(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "E")
	})

	t.Run("stage lease lapses while open", func(t *testing.T) {
		// A stage holds T, and its process stalls past the lease: a collector
		// that does not renew, as another process's does, releases the
		// stage's root. The next renewal holds it again, so after the grace
		// period a parent the stage's writer puts still finds T.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tr := testTree{}
		tr.add(t, "T")
		if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("T")); err != nil {
			t.Fatal(err)
		}
		clk.Add(rootLease + time.Second)
		if res, err := v.collector.Collect(ctx); err != nil || res.Lapsed != 1 {
			t.Fatalf("stalled pass lapsed %d err %v, want the stage root", res.Lapsed, err)
		}
		if _, err := v.renewRoots(ctx, false); err != nil {
			t.Fatal(err)
		}
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "T")

		// The parent references T under main, the stage lets it go, and
		// T stays.
		p := tr.add(t, "P", "T")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		release()
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "T")
		requireAudit(t, v, 0)
	})

	t.Run("process lease released partway through a stall", func(t *testing.T) {
		// Two stages hold Y and W under the one process lease, and the
		// process stalls past it. A collector slice marks the lease
		// released and releases Y's root. W's root is still indexed
		// under the lease. No writer may hold a root onto the released
		// lease, nor renew it. The next renewal starts a new lease,
		// moves W's root onto it, and holds Y again, whose release has
		// not netted. The collector then ends the old lease without
		// releasing a root again, never releases a root of the live
		// one, and past the grace period both stay. The stage stalled
		// past the grace period, below, loses its block.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		stageRoot := func(stage string) block.NamedRoot {
			t.Helper()
			unlock, err := v.rootPinMu.Lock(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer unlock()
			for name, ref := range v.stages[stage].roots {
				return block.NamedRoot{Name: name, Ref: ref}
			}
			t.Fatalf("stage %s holds no root", stage)
			return block.NamedRoot{}
		}
		rescued, releaseRescued, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseRescued()
		kept, releaseKept, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseKept()
		tr.add(t, "Y")
		tr.add(t, "W")
		if _, err := v.PrepareStagedBlockBatch(ctx, rescued, tr.entries("Y")); err != nil {
			t.Fatal(err)
		}
		if _, err := v.PrepareStagedBlockBatch(ctx, kept, tr.entries("W")); err != nil {
			t.Fatal(err)
		}
		old := v.processLeaseID()
		y, w := stageRoot(rescued), stageRoot(kept)

		// Stall past the lease. A collector slice releases it and Y's
		// root.
		clk.Add(rootLease + time.Second)
		err = v.withDirectAtomic(ctx, func(p *physicalTx) error {
			if err := p.rec.ReleaseLease(ctx, old); err != nil {
				return err
			}
			_, err := v.land(ctx, p, "", nil, []block.NamedRoot{{Name: y.Name, Previous: y.Ref}})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		// No writer holds a root onto the released lease, nor renews it.
		err = v.withDirectAtomic(ctx, func(p *physicalTx) error {
			now := clk.Now()
			renewed, err := p.rec.RenewLease(ctx, old, now.UnixMilli(), now.Add(rootLease).UnixMilli())
			if err != nil || renewed {
				t.Fatalf("renewing the released lease returned %v err %v, want refused", renewed, err)
			}
			_, err = v.land(ctx, p, "", nil, []block.NamedRoot{{Name: "late", Ref: w.Ref, Lease: old}})
			return err
		})
		if !errors.Is(err, block_gc.ErrLeaseLapsed) {
			t.Fatalf("a root onto the released lease returned %v, want ErrLeaseLapsed", err)
		}

		// The renewal moves W's root onto a new lease and holds Y again.
		if _, err := v.renewRoots(ctx, false); err != nil {
			t.Fatal(err)
		}
		id := v.processLeaseID()
		if id == old {
			t.Fatal("the renewal kept the released lease")
		}
		err = v.readRecords(ctx, func(rec *block_gc.Records) error {
			for _, root := range []block.NamedRoot{y, w} {
				entry, err := rec.GetRoot(ctx, "", root.Name)
				if err != nil || entry.GetLease() != id || !bytes.Equal(entry.GetRef(), block_gc.BlockKey(root.Ref)) {
					t.Fatalf("root %s is %v err %v, want held under the new lease", root.Name, entry, err)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		// The collector ends the old lease and releases nothing of the new.
		if res := collectAll(t, v); res.Lapsed != 0 {
			t.Fatalf("pass after the renewal released %d roots, want none", res.Lapsed)
		}
		err = v.readRecords(ctx, func(rec *block_gc.Records) error {
			if _, found, err := rec.GetLease(ctx, old); err != nil || found {
				t.Fatalf("old lease found %v err %v, want ended", found, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		if res := collectAll(t, v); res.Lapsed != 0 {
			t.Fatalf("pass under the live lease released %d roots, want none", res.Lapsed)
		}
		requireStored(t, v, tr, true, "W", "Y")
	})

	for _, renew := range []bool{false, true} {
		t.Run("stage stalled past the grace period renew="+strconv.FormatBool(renew), func(t *testing.T) {
			// A stage holds T, and its process stalls past the
			// lease and the grace period: a collector that does not
			// renew releases the stage's root and frees T. The
			// stage's next put references T, and it is refused, as
			// is any put after a renewal that finds T gone, rather
			// than landing S over a freed block.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			stage, release, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			tr := testTree{}
			tr.add(t, "T")
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("T")); err != nil {
				t.Fatal(err)
			}
			if err := v.MarkRootsComplete(ctx, []*block.BlockRef{tr["T"].Ref}); err != nil {
				t.Fatal(err)
			}
			clk.Add(rootLease + time.Second)
			if _, err := v.collector.Collect(ctx); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			for range 3 {
				if _, err := v.collector.Collect(ctx); err != nil {
					t.Fatal(err)
				}
			}
			requireStored(t, v, tr, false, "T")

			// The check a stage copy makes before its release, with no write
			// through the stage since, finds T's proof gone and holds nothing.
			if held, err := v.HoldStageCompleteRoots(ctx, stage, []*block.BlockRef{tr["T"].Ref}); err == nil && held[0] {
				t.Fatal("the stage check held a graph the collector freed")
			}
			if renew {
				if _, err := v.renewRoots(ctx, false); err != nil {
					t.Fatal(err)
				}
			}
			tr.add(t, "S", "T")
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("S")); !errors.Is(err, block.ErrStageReleased) {
				t.Fatalf("put over a freed stage top = %v, want the stage lost", err)
			}
			requireStored(t, v, tr, false, "S")
		})
	}

	for _, audited := range []bool{true, false} {
		t.Run("hole refilled by a stage audited="+strconv.FormatBool(audited), func(t *testing.T) {
			// main -> R -> H, and the store loses H with its child list. A
			// stage then puts H again and drops it. Once an audit recorded
			// the hole, the put counts H as stored under R, so H stays at
			// exactly R's holder, through a second refill as well.
			// Refilled before any audit, H counts as a new block under the
			// stage alone, and the audit reports the undercount.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			h := tr.add(t, "H")
			r := tr.add(t, "R", "H")
			if err := v.SetBucketRoots(ctx, "b", tr.entries("H", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
				t.Fatal(err)
			}
			punchHole(t, v, h)
			if audited {
				if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
					t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
				}
			}
			refill := func() {
				stage, release, err := v.OpenStage(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("H")); err != nil {
					t.Fatal(err)
				}
				release()
				if err := v.withDirectAtomic(ctx, nil); err != nil {
					t.Fatal(err)
				}
			}
			refill()
			if !audited {
				if res, err := v.Audit(ctx); err != nil || res.Undercount != 1 {
					t.Fatalf("audit of an unrecorded refill found %d undercounts err %v, want 1", res.Undercount, err)
				}
				return
			}
			for i := range 2 {
				clk.Add(block_gc.DefaultGrace + time.Second)
				collectAll(t, v)
				requireStored(t, v, tr, true, "R", "H")
				if holders, _ := blockHolders(t, v, tr, "H"); holders != 1 {
					t.Fatalf("after refill %d H has %d holders, want R's alone", i+1, holders)
				}
				requireAudit(t, v, 0)
				if i == 0 {
					refill()
				}
			}
		})
	}

	for name, lapsed := range map[string]bool{"live": false, "lapsed": true} {
		t.Run("stage parent ahead of its child "+name, func(t *testing.T) {
			// A stage stores P, which references C, while C waits
			// in the writer's memory, and its process dies before C
			// lands. The audit records C as a hole either way. Only
			// once the stage's lease lapsed does the audit count C
			// as a hole no live holder references.
			v, clk := newLivenessVolume(t, false)
			ctx := t.Context()
			tr := testTree{}
			tr.add(t, "C")
			tr.add(t, "P", "C")
			stage, _, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("P")); err != nil {
				t.Fatal(err)
			}
			want := uint64(0)
			if lapsed {
				clk.Add(rootLease + time.Second)
				want = 1
			}
			res, err := v.Audit(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if res.Holes != 1 || res.LapsedHoles != want || uint64(len(res.Mark.LapsedRefs)) != want {
				t.Fatalf("audit recorded %d holes, %d of them lapsed, and found %d lapsed, want 1, %d and %d",
					res.Holes, res.LapsedHoles, len(res.Mark.LapsedRefs), want, want)
			}
		})
	}
}
