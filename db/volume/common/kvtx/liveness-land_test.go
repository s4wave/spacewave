package kvtx

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
)

// testLivenessLand checks block admission and parent-child landing order.
func testLivenessLand(t *testing.T) {
	t.Helper()

	t.Run("child named twice", func(t *testing.T) {
		// P references the stored C twice and an empty ref. Its list
		// keeps C once, so releasing P frees both, and the audit finds
		// nothing.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: c}}); err != nil {
			t.Fatal(err)
		}
		p := tr.add(t, "P", "C", "C")
		tr["P"].Refs = append(tr["P"].Refs, &block.BlockRef{})
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "p", Ref: p}, {Name: "c", Previous: c}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "p", Previous: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "P", "C")
		requireAudit(t, v, 0)
	})

	t.Run("bytes without a list", func(t *testing.T) {
		// Bytes stored below the Volume's liveness, without a child
		// list, are garbage nothing frees. The audit reports them.
		v, _ := newLivenessVolume(t, true)
		ctx := t.Context()
		if _, _, err := v.Store.PutBlock(ctx, []byte("stored without a list"), &block.PutOpts{RefsKnown: true}); err != nil {
			t.Fatal(err)
		}
		if res, err := v.Audit(ctx); err != nil || res.Unlisted != 1 {
			t.Fatalf("audit found %d unlisted blocks err %v, want 1", res.Unlisted, err)
		}
	})

	t.Run("abandoned write", func(t *testing.T) {
		// A writer puts P -> C in two batches, child first, and never
		// lands a root on them. A Volume with frees on refuses the
		// first batch and stores nothing. One that lets the puts in
		// finds both garbage once the grace period passes.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		tr.add(t, "P", "C")
		if _, err := v.PutBlockBatch(ctx, tr.entries("C")); !errors.Is(err, block.ErrUnheldPut) {
			t.Fatalf("unheld put returned %v, want ErrUnheldPut", err)
		}
		requireStored(t, v, tr, false, "C")
		observePuts(v)
		for _, name := range []string{"C", "P"} {
			if _, err := v.PutBlockBatch(ctx, tr.entries(name)); err != nil {
				t.Fatal(err)
			}
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "P", "C")
		requireAudit(t, v, 0)
	})

	t.Run("parent put before its child", func(t *testing.T) {
		// A buffered drain puts P under main before its child C, and C lands
		// alone within the grace period. Both stay, and releasing main frees
		// both. C's put alone is unheld in its own batch.
		v, clk := newLivenessVolume(t, true)
		observePuts(v)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		p := tr.add(t, "P", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		if _, err := v.PutBlockBatch(ctx, tr.entries("C")); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "C")
		requireAudit(t, v, 0)
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: p}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "P", "C")
		requireAudit(t, v, 0)
	})

	t.Run("child put again with its stored parent", func(t *testing.T) {
		// A staged copy puts P under main before its child C, then puts
		// C in a batch with P again, P as a copy writes it, with refs it
		// did not read from content. P's stored list holds C, so a Volume
		// with frees on lets the batch in. C alone it refuses.
		v, clk := newLivenessVolume(t, true)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C")
		p := tr.add(t, "P", "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v.PutBlockBatch(ctx, tr.entries("C")); !errors.Is(err, block.ErrUnheldPut) {
			t.Fatalf("unheld put returned %v, want ErrUnheldPut", err)
		}
		copied := *tr["P"]
		copied.RefsKnown = false
		if _, err := v.PutBlockBatch(ctx, append([]*block.PutBatchEntry{&copied}, tr.entries("C")...)); err != nil {
			t.Fatalf("put again with a copied parent: %v", err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "C")
		requireAudit(t, v, 0)
	})

	t.Run("parent collected before its child is stored", func(t *testing.T) {
		// A session puts R2 under main before its child C2, logging an
		// up for C2. Main lets R2 go and the collector frees it. The
		// free takes C2's holder although C2 has no list yet. C2 then
		// lands alone and nothing holds it, so it goes after the grace
		// period.
		v, clk := newLivenessVolume(t, true)
		observePuts(v)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "C2")
		r2 := tr.add(t, "R2", "C2")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("R2"), []block.NamedRoot{{Name: "main", Ref: r2}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: r2}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R2")
		if _, err := v.PutBlockBatch(ctx, tr.entries("C2")); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "C2")
		requireAudit(t, v, 0)
	})

	for _, pinned := range []bool{true, false} {
		t.Run("long staged session over an abandoned base pinned="+strconv.FormatBool(pinned), func(t *testing.T) {
			// main holds B -> X. A staged session observes B,
			// pinning it as the World engine's StageWorldState
			// does. Main abandons B, and the session outlasts the
			// grace period before it puts S referencing X without
			// putting X again and lands S under main. With the pin
			// X stays. Without it X is freed before S references
			// it.
			v, clk := newLivenessVolume(t, true)
			ctx := t.Context()
			tr := testTree{}
			tr.add(t, "X")
			b := tr.add(t, "B", "X")
			if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "B"), []block.NamedRoot{{Name: "main", Ref: b}}); err != nil {
				t.Fatal(err)
			}
			stage, release, err := v.OpenStage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if pinned {
				unpin, err := v.PinBucketRoot(ctx, b)
				if err != nil {
					t.Fatal(err)
				}
				defer unpin()
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: b}}); err != nil {
				t.Fatal(err)
			}
			advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
			collectAll(t, v)
			s := tr.add(t, "S", "X")
			if _, err := v.PrepareStagedBlockBatch(ctx, stage, tr.entries("S")); err != nil {
				t.Fatal(err)
			}
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Ref: s}}); err != nil {
				t.Fatal(err)
			}
			release()
			requireStored(t, v, tr, pinned, "X")
		})
	}

	// The admit subtests hold a payload tree P -> L that a stage built
	// earlier, as a World's admit does: payloads moves from Q onto a new H
	// referencing P and Q, requiring its blocks held.
	t.Run("admit after the tree was freed", func(t *testing.T) {
		// The stage's release nets and frees P and L: refused.
		v, clk, tr, admit := newAdmitVolume(t, true)
		buildAndRelease(t, v, clk, tr)
		requireStored(t, v, tr, false, "P", "L")
		requireNotHeld(t, v, tr, admit())
	})

	t.Run("admit over a Dead parent", func(t *testing.T) {
		// P is stored and Dead, and still holds L, but the admit's root
		// references H over P, which nothing holds: refused.
		v, clk, tr, admit := newAdmitVolume(t, true)
		freeUnderDeadParent(t, v, clk, tr)
		requireNotHeld(t, v, tr, admit())
	})

	t.Run("admit after the collector freed a Dead parent and its child", func(t *testing.T) {
		// A collector that frees takes Dead P, and L with it. The admit
		// over H referencing P, and the admit storing P again over L,
		// are both refused, and no root references a freed block.
		v, clk, tr, admit := newAdmitVolume(t, true)
		freeUnderDeadParent(t, v, clk, tr)
		for range 3 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		}
		requireStored(t, v, tr, false, "P", "L")
		for _, name := range []string{"P", "L"} {
			if listed, held := admitRecords(t, v, tr, name); listed || held {
				t.Fatalf("freed %s listed=%v held=%v, want neither", name, listed, held)
			}
		}
		requireNotHeld(t, v, tr, admit())
		requireNotHeld(t, v, tr, admit("P"))
		if got, err := v.GetBucketRoot(t.Context(), "b", "payloads"); err != nil || !got.EqualsRef(tr["Q"].Ref) {
			t.Fatalf("payloads holds %v after the refusals: %v", got, err)
		}
		requireStored(t, v, tr, true, "Q")
		requireAudit(t, v, 0)
	})

	t.Run("put again of a Dead parent holds its child until it nets", func(t *testing.T) {
		// The release nets with frees off, so P is Dead on the free queue
		// and holds L. A put of P again takes it off the queue and holds it
		// for a grace period: a collect that frees, within it, frees nothing
		// and nets the put's up, so P is held and the admit lands, and the
		// admitted tree stays whole once the put's down nets.
		v, clk, tr, admit := newAdmitVolume(t, true)
		ctx := t.Context()
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		stagePut(t, v, stage, tr, "L", "P")
		release()
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		netAll(t, v)
		if _, err := v.PutBlockBatch(ctx, tr.entries("P")); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace - time.Second)
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("freed %d before the put again netted", res.Freed)
		}
		requireStored(t, v, tr, true, "P", "L")
		if err := admit(); err != nil {
			t.Fatalf("admit over P held by its put again = %v, want commit", err)
		}
		requireAdmitKept(t, v, clk, tr)
	})

	t.Run("admit putting a Dead parent again over its freed child", func(t *testing.T) {
		// (j) The admit's batch stores P again, which proves P's bytes, and
		// Dead P still holds L, so the admit lands.
		v, clk, tr, admit := newAdmitVolume(t, true)
		freeUnderDeadParent(t, v, clk, tr)
		requireDeadParentAdmit(t, v, clk, tr, admit("P"))
	})

	t.Run("admit giving birth to a payload root over its freed child", func(t *testing.T) {
		// (k) The tree was freed whole. The admit's batch stores P anew
		// but not L: refused.
		v, clk, tr, admit := newAdmitVolume(t, true)
		buildAndRelease(t, v, clk, tr)
		requireStored(t, v, tr, false, "P", "L")
		requireNotHeld(t, v, tr, admit("P"))
	})

	t.Run("admit carrying P only through a copied entry", func(t *testing.T) {
		// As (k), but the batch stores P through a copied entry whose refs
		// omit the freed L, as a copy from a source that lost P's list
		// would: P's refs prove nothing of its children, so the walk must
		// not trust them, and the admit is refused whatever they say, both
		// with P as H's child and as the root itself.
		v, clk, tr, _ := newAdmitVolume(t, true)
		buildAndRelease(t, v, clk, tr)
		requireStored(t, v, tr, false, "P", "L")
		p := &block.PutBatchEntry{Ref: tr["P"].Ref, Data: tr["P"].Data}
		for _, root := range []string{"H", "P"} {
			roots := []block.NamedRoot{{Name: "payloads", Ref: tr[root].Ref, Previous: tr["Q"].Ref, RequireHeld: true}}
			requireNotHeld(t, v, tr, v.SetBucketRoots(t.Context(), "b", append(tr.entries("H"), p), roots))
		}
	})

	t.Run("admit repeated with the tree in its batch", func(t *testing.T) {
		// As (j), then the writer repeats the admit storing the whole
		// tree in its batch, as ErrRootNotHeld promises: commits. The
		// repeat moves onto a new H referencing P and M over L, a
		// diamond three deep. The first admit lands, since Dead P holds
		// L, and the repeat moves on from it.
		v, clk, tr, admit := newAdmitVolume(t, true)
		freeUnderDeadParent(t, v, clk, tr)
		if err := admit("P"); err != nil {
			t.Fatalf("admit over L held by Dead P = %v, want commit", err)
		}
		prev := tr["H"].Ref
		tr.add(t, "M", "L")
		h := tr.add(t, "H", "P", "M", "Q")
		roots := []block.NamedRoot{{Name: "payloads", Ref: h, Previous: prev, RequireHeld: true}}
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("H", "M", "P", "L"), roots); err != nil {
			t.Fatal(err)
		}
		requireAdmitKept(t, v, clk, tr)
		requireStored(t, v, tr, true, "M")
	})

	for _, counted := range []bool{false, true} {
		name := "admit observed past a failed read"
		if counted {
			name += " after a count"
		}
		t.Run(name, func(t *testing.T) {
			// The check's stage read fails on P, Dead over its
			// child L. Enforcing returns the error. Observing
			// counts nothing and lands the move as with the check
			// off. With counted, the first stage read answers
			// false, so P is counted as H's child, and a second
			// strict root on P then fails its read: the count made
			// before the failure is dropped too.
			v, clk, tr, _ := newAdmitVolume(t, true)
			freeUnderDeadParent(t, v, clk, tr)
			errRead := errors.New("stage read failed")
			roots := []block.NamedRoot{{Name: "payloads", Ref: tr["H"].Ref, Previous: tr["Q"].Ref, RequireHeld: true}}
			if counted {
				roots = append(roots, block.NamedRoot{Name: "again", Ref: tr["P"].Ref, RequireHeld: true})
			}
			land := func(mode block_gc.HeldMode) error {
				reads := 0
				return v.withDirectAtomic(t.Context(), func(p *physicalTx) error {
					_, err := v.landMoves(t.Context(), p, tr.entries("H"), block_gc.BucketRoots{
						BucketID: "b",
						Roots:    roots,
						Mode:     mode,
						StageHeld: func(context.Context, *block.BlockRef) (bool, error) {
							reads++
							if counted && reads == 1 {
								return false, nil
							}
							return false, errRead
						},
						Observed: v.observeNotHeld,
					})
					return err
				})
			}
			// Enforcing refuses at the first block not held, before the
			// failing read when counted.
			want := errRead
			if counted {
				want = block.ErrRootNotHeld
			}
			if err := land(block_gc.HeldEnforce); !errors.Is(err, want) {
				t.Fatalf("enforced Land = %v, want %v", err, want)
			}
			if err := land(block_gc.HeldObserve); err != nil {
				t.Fatalf("observed Land = %v, want commit", err)
			}
			if got, err := v.GetBucketRoot(t.Context(), "b", "payloads"); err != nil || !got.EqualsRef(tr["H"].Ref) {
				t.Fatalf("payloads holds %v after the observed Land: %v", got, err)
			}
			h, _ := v.GetHealthSnapshotWithWait()
			if n := h.GetNotHeldRootBlocks() + h.GetNotHeldChildBlocks(); n != 0 {
				t.Fatalf("a failed check counted %d", n)
			}
		})
	}

	for _, freed := range []string{"put again", "born"} {
		t.Run("admit storing P "+freed+" over a child a live stage holds", func(t *testing.T) {
			// (l) As (j) and (k), with an open stage holding L: commits, and
			// a collector that frees keeps the whole tree.
			v, clk, tr, admit := newAdmitVolume(t, true)
			if freed == "born" {
				buildAndRelease(t, v, clk, tr)
			} else {
				freeUnderDeadParent(t, v, clk, tr)
			}
			stage, release, err := v.OpenStage(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			stagePut(t, v, stage, tr, "L")
			if err := admit("P"); err != nil {
				t.Fatal(err)
			}
			release()
			requireAdmitKept(t, v, clk, tr)
		})
	}

	t.Run("admit observed with frees off", func(t *testing.T) {
		// The default with frees off observes: the move lands over the
		// released tree and counts P, a child outside the batch.
		v, clk := newLivenessVolume(t, false)
		if v.heldMode != block_gc.HeldObserve {
			t.Fatalf("mode %v with frees off, want observe", v.heldMode)
		}
		tr, admit := admitTree(t, v)
		buildAndRelease(t, v, clk, tr)
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		h, _ := v.GetHealthSnapshotWithWait()
		root, child := h.GetNotHeldRootBlocks(), h.GetNotHeldChildBlocks()
		if root != 0 || child != 1 {
			t.Fatalf("observed root=%d child=%d, want 0 and 1", root, child)
		}
	})

	t.Run("admit with the check off", func(t *testing.T) {
		// Off with frees on checks nothing: the move lands and counts none.
		v, clk := newLivenessVolume(t, true)
		v.collector = v.newCollector(collectorStore{v: v}, false, false, block_gc.DefaultGrace)
		v.heldMode = rootHeldMode(store_kvtx.RootHeldCheck_ROOT_HELD_CHECK_OFF, true)
		tr, admit := admitTree(t, v)
		buildAndRelease(t, v, clk, tr)
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		h, _ := v.GetHealthSnapshotWithWait()
		if n := h.GetNotHeldRootBlocks() + h.GetNotHeldChildBlocks(); n != 0 {
			t.Fatalf("observed %d with the check off", n)
		}
	})

	t.Run("admit over bytes without a list", func(t *testing.T) {
		v, _, tr, admit := newAdmitVolume(t, true)
		ref, _, err := v.Store.PutBlock(t.Context(), tr["P"].Data, &block.PutOpts{RefsKnown: true})
		if err != nil || !ref.EqualsRef(tr["P"].Ref) {
			t.Fatalf("stored P without a list as %v: %v", ref, err)
		}
		requireNotHeld(t, v, tr, admit())
	})

	t.Run("admit after a flush cut after its first slice", func(t *testing.T) {
		// The open stage stored L only. P is missing: refused.
		v, _, tr, admit := newAdmitVolume(t, true)
		stage, release, err := v.OpenStage(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		stagePut(t, v, stage, tr, "L")
		requireNotHeld(t, v, tr, admit())
	})

	t.Run("admit retried over a rebuilt tree", func(t *testing.T) {
		// The release nets P and L to zero with frees off. A second stage
		// puts the same tree again, and the admit commits at once, before
		// netting gives P a holder. Once that stage is released and its
		// release nets, a collector that frees keeps the whole tree.
		v, clk, tr, admit := newAdmitVolume(t, false)
		buildAndRelease(t, v, clk, tr)
		stage, release, err := v.OpenStage(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		stagePut(t, v, stage, tr, "L", "P")
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		requireStored(t, v, tr, true, "H", "P", "L", "Q")
		release()
		requireAdmitKept(t, v, clk, tr)
	})

	t.Run("admit under a live hold", func(t *testing.T) {
		v, clk, tr, admit := newAdmitVolume(t, true)
		if err := v.SetBucketRoots(t.Context(), "b", tr.entries("L", "P"), []block.NamedRoot{{Name: "other", Ref: tr["P"].Ref}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		requireStored(t, v, tr, true, "H", "P", "L", "Q")
	})

	t.Run("admit after the stage was released", func(t *testing.T) {
		// The stage's release nets P and L to zero with frees off, so
		// both are stored but nothing holds them: refused. The release
		// nets first because until it does, P still reads one holder
		// and the admit commits. That commit is safe, since the admit's
		// up pair and the release's down pair net together and leave P
		// held.
		v, clk, tr, admit := newAdmitVolume(t, false)
		buildAndRelease(t, v, clk, tr)
		requireStored(t, v, tr, true, "P", "L")
		requireNotHeld(t, v, tr, admit())
	})

	t.Run("admit before the stage's release nets", func(t *testing.T) {
		// The admit commits while the release is in the logs and not
		// netted, so P still reads its implied holder. The release then
		// nets alone, older than Grace while the admit's record is not,
		// so the release brings P and L to zero. Their ripe sequence
		// lies past the admit's record, so no free runs until the admit
		// nets and holds them again.
		v, clk, tr, admit := newAdmitVolume(t, true)
		ctx := t.Context()
		stage, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		stagePut(t, v, stage, tr, "L", "P")
		release()
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace / 2)
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace/2 + time.Second)
		if res := collectAll(t, v); res.Freed != 0 {
			t.Fatalf("freed %d between the release and the admit", res.Freed)
		}
		requireStored(t, v, tr, true, "H", "P", "L", "Q")
		for range 2 {
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
		}
		requireStored(t, v, tr, true, "H", "P", "L", "Q")
		requireAudit(t, v, 0)
	})

	t.Run("admit evidenced by another holder's stage", func(t *testing.T) {
		// After the writer's release nets to zero, another holder's stage
		// puts the tree again while the writer's own stage holds nothing of
		// it. The admit commits on that evidence, the other stage is released
		// right after, and a collector that frees keeps the whole tree.
		v, clk, tr, admit := newAdmitVolume(t, false)
		ctx := t.Context()
		buildAndRelease(t, v, clk, tr)
		_, own, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer own()
		other, release, err := v.OpenStage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		stagePut(t, v, other, tr, "L", "P")
		if err := admit(); err != nil {
			t.Fatal(err)
		}
		release()
		requireAdmitKept(t, v, clk, tr)
	})
}

// BenchmarkLandCommit times one World-shaped commit on s4db: a new parent
// referencing fanout stored leaves, put with the move of its root, so its Land
// checks fanout outside children, as a commit that rewrites a tree page does.
// It reports the store reads each commit makes, a batched read counting once.
func BenchmarkLandCommit(b *testing.B) {
	const fanout = 250

	// Open the Volume over a counting s4db store, frees off as on the Mac.
	ctx := b.Context()
	db, err := s4db.Open(filepath.Join(b.TempDir(), "bench.s4db"), s4db.Options{})
	if err != nil {
		b.Fatal(err)
	}
	cs := kvtx_kvtest.NewCountingStore(db)
	v, err := NewVolume(ctx, "test/land-commit", store_kvkey.NewDefaultKVKey(), cs, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = v.Close()
		_ = db.Close()
	})
	if v.adoptDone != nil {
		<-v.adoptDone
	}

	// Store the leaves under one root.
	entry := func(data []byte, refs []*block.BlockRef) *block.PutBatchEntry {
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			b.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
	}
	var leaves []*block.PutBatchEntry
	var refs []*block.BlockRef
	for i := range fanout {
		e := entry([]byte("leaf "+strconv.Itoa(i)), nil)
		leaves, refs = append(leaves, e), append(refs, e.Ref)
	}
	base := entry([]byte("base"), refs)
	if err := v.SetBucketRoots(ctx, "b", append(leaves, base), []block.NamedRoot{{Name: "base", Ref: base.Ref}}); err != nil {
		b.Fatal(err)
	}

	// Commit a new parent of the leaves under main each iteration.
	var prev *block.BlockRef
	start := cs.Counts().Reads
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		p := entry([]byte("parent "+strconv.Itoa(i)), refs)
		if err := v.SetBucketRoots(ctx, "b", []*block.PutBatchEntry{p}, []block.NamedRoot{{Name: "main", Ref: p.Ref, Previous: prev}}); err != nil {
			b.Fatal(err)
		}
		prev = p.Ref
	}
	b.StopTimer()
	b.ReportMetric(float64(cs.Counts().Reads-start)/float64(b.N), "reads/op")
}

// sliceStore is an in-memory store that counts the write transactions while
// armed and runs hook before each after the first, so a test acts between the
// slices of a staged batch.
type sliceStore struct {
	*seekGuard
	// armed counts the write transactions while set.
	armed bool
	// opened counts the write transactions since the store was armed.
	opened int
	// hook runs before the write after the n-th, with no store writer held.
	// The writes it makes are not counted.
	hook func(n int)
}

// NewTransaction runs hook before an armed write after the first, then opens
// the transaction, refusing it when hook cancelled ctx.
func (s *sliceStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write && s.armed {
		s.opened++
		if s.opened > 1 {
			s.armed = false
			s.hook(s.opened - 1)
			s.armed = true
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
	}
	return s.seekGuard.NewTransaction(ctx, write)
}

// sliceMode is what testStageSlices does with a staged batch.
type sliceMode int

const (
	// sliceCollect collects past the grace period between slices.
	sliceCollect sliceMode = iota
	// sliceCancelRetry cancels after the first slice, then retries the batch.
	sliceCancelRetry
	// sliceCancelRelease cancels after the first slice, then releases the
	// stage.
	sliceCancelRelease
	// sliceRelease releases the stage after the first slice.
	sliceRelease
	// sliceParentFirst orders the parent before its children.
	sliceParentFirst
	// sliceOneTransaction opens the Volume with Config.StageOneTransaction.
	sliceOneTransaction
)

// testStageSlices lands a stage's batch of seven leaves and their parent in
// slices of three, so the parent lands two transactions after its first
// children, and between slices runs a collector pass past the grace period,
// cancels the batch or releases the stage. No block of a landed slice may go
// while the stage holds it. A retry lands the rest and finds the earlier slices
// stored. A release refuses the next slice and, like a cancellation without a
// retry, leaves the landed blocks to the collector. A batch ordered parent
// first, or under StageOneTransaction, lands in one transaction.
func testStageSlices(t *testing.T, mode sliceMode) {
	store := &sliceStore{seekGuard: newGuardedStore(t)}
	v, clk := openConfVolume(t, store, &store_kvtx.Config{
		CollectorFrees:      true,
		StageOneTransaction: mode == sliceOneTransaction,
	})
	if mode != sliceOneTransaction {
		v.stageSlice = 3
	}
	stage, release, err := v.OpenStage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	tr := testTree{}
	leaves := []string{"C0", "C1", "C2", "C3", "C4", "C5", "C6"}
	for _, name := range leaves {
		tr.add(t, name)
	}
	p := tr.add(t, "P", leaves...)
	names := append(append([]string(nil), leaves...), "P")
	order := names
	if mode == sliceParentFirst {
		order = append([]string{"P"}, leaves...)
	}

	// Between slices, collect past the grace period and check that every
	// landed block stays, or cancel or release after the first slice.
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	store.hook = func(n int) {
		switch mode {
		case sliceCancelRetry, sliceCancelRelease:
			stop()
		case sliceRelease:
			release()
		default:
			advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
			collectAll(t, v)
			requireStored(t, v, tr, true, names[:3*n]...)
		}
	}
	store.armed = true
	_, err = v.PrepareStagedBlockBatch(ctx, stage, tr.entries(order...))
	store.armed = false
	testStageSliceResult(t, mode, store.opened, err)

	// A cancelled batch leaves the first slice held by the stage, and a retry
	// finds it. A release before the second slice enters the writer refuses
	// that slice and leaves the first slice's roots for deferred release.
	if mode == sliceCancelRetry || mode == sliceCancelRelease {
		advanceRenewing(t, v, clk, block_gc.DefaultGrace+time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, names[:3]...)
		requireStored(t, v, tr, false, names[3:]...)
	}
	if mode == sliceCancelRetry {
		existed, err := v.PrepareStagedBlockBatch(t.Context(), stage, tr.entries(names...))
		if err != nil {
			t.Fatal(err)
		}
		for i, got := range existed {
			if got != (i < 3) {
				t.Fatalf("retry found entry %d existed=%v, want %v", i, got, i < 3)
			}
		}
	}
	if mode == sliceCancelRelease || mode == sliceRelease {
		// The release, applied by the next direct transaction, leaves the
		// landed leaves to the collector.
		release()
		if err := v.withDirectAtomic(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, names...)
		requireAudit(t, v, 0)
		return
	}

	// Main adopts the parent, the stage lets go, and every block stays.
	if err := v.SetBucketRoots(t.Context(), "b", nil, []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
		t.Fatal(err)
	}
	release()
	clk.Add(block_gc.DefaultGrace + time.Second)
	collectAll(t, v)
	requireStored(t, v, tr, true, names...)
	requireAudit(t, v, 0)
}

// testStageSliceResult checks the refusal and the write transactions of
// testStageSlices' batch under mode.
func testStageSliceResult(t *testing.T, mode sliceMode, writes int, err error) {
	t.Helper()
	want, wantErr := 3, error(nil)
	switch mode {
	case sliceCancelRetry, sliceCancelRelease:
		want, wantErr = 2, context.Canceled
	case sliceRelease:
		want, wantErr = 2, block.ErrStageReleased
	case sliceParentFirst, sliceOneTransaction:
		want = 1
	}
	if !errors.Is(err, wantErr) || (wantErr == nil && err != nil) {
		t.Fatalf("batch returned %v, want %v", err, wantErr)
	}
	if writes != want {
		t.Fatalf("batch opened %d write transactions, want %d", writes, want)
	}
}
