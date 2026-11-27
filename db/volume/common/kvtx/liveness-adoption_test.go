package kvtx

import (
	"context"
	"errors"
	"maps"
	"math/rand/v2"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// testLivenessAdoptionFill checks cache fills while adoption counts holders.
func testLivenessAdoptionFill(t *testing.T) {
	t.Helper()

	t.Run("cache fill during an adoption", func(t *testing.T) {
		// main holds C on an unadopted Volume. While an adoption
		// settles its counts, after it counted the child lists, a cache
		// fill stores F referencing C, logging an up for C and a down
		// that takes F's implied holder back. The adoption treats F as
		// born and leaves it to the logs. A root r then moves onto F: F
		// stays while r holds it, and once r is released F goes and C
		// stays under main.
		store := newGuardedStore(t)
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "main", Ref: c}}); err != nil {
			t.Fatal(err)
		}
		f := tr.add(t, "F", "C")
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptSettle: func() {
				if _, _, err := v.PrepareOwnedBlock(ctx, "", tr["F"].Data, &block.PutOpts{Refs: tr["F"].Refs, RefsKnown: true, CacheFill: true}); err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe, v.now); err != nil || len(probe.at) != 0 {
			t.Fatalf("adopt = %v with %d fills left", err, len(probe.at))
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "r", Ref: f}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(fillLapse)
		collectAll(t, v)
		requireStored(t, v, tr, true, "C", "F")
		requireAudit(t, v, 0)

		// Released, F goes and C stays.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "r", Previous: f}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "F")
		requireStored(t, v, tr, true, "C")
		requireAudit(t, v, 0)
	})

	// main holds P referencing B, which the Volume lacks, on an unadopted
	// Volume, and a read fills B at a point of an adoption: while it counts
	// the child lists, or between its clear and its mark. Either way B
	// keeps P's holder after the grace period. The hooks also check the
	// probe's positions against the adoption state.
	for _, at := range []struct {
		name  string
		pos   int
		state block_gc.Adoption
	}{
		{"during the list scan", adoptCountChildren, block_gc.Adoption_ADOPTION_ADOPTING},
		{"between the clear and the mark", adoptMark, block_gc.Adoption_ADOPTION_UNADOPTED},
	} {
		t.Run("cache fill "+at.name, func(t *testing.T) {
			// Store P referencing the missing B under main.
			store := newGuardedStore(t)
			setUnadopted(t, store)
			v, clk := openUnadoptedVolume(t, store)
			ctx := t.Context()
			tr := testTree{}
			tr.add(t, "B")
			p := tr.add(t, "P", "B")
			if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
				t.Fatal(err)
			}

			// Adopt, filling B at the point.
			probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
				at.pos: func() {
					if got := livenessAdoption(t, v); got != at.state {
						t.Errorf("adoption at position %d is %v, want %v", at.pos, got, at.state)
					}
					err := v.withDirectFill(ctx, func(px *physicalTx) error {
						_, _, err := v.landFill(ctx, px, "b", tr.entries("B"))
						return err
					})
					if err != nil {
						t.Error(err)
					}
				},
			}}
			if _, err := block_gc.Adopt(ctx, probe, v.now); err != nil || len(probe.at) != 0 {
				t.Fatalf("adopt = %v with %d fills left", err, len(probe.at))
			}

			// B keeps P's holder, and goes once main lets P go.
			clk.Add(fillLapse)
			collectAll(t, v)
			requireStored(t, v, tr, true, "P", "B")
			requireAudit(t, v, 0)
			if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "main", Previous: p}}); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			collectAll(t, v)
			requireStored(t, v, tr, false, "P", "B")
		})
	}

	t.Run("background adoption", func(t *testing.T) {
		// An unadopted Volume holding main -> R -> X and an unheld G opens
		// with frees on and adopts itself in the background: R and X stay
		// and G goes after the grace period.
		store := newGuardedStore(t)
		setUnadopted(t, store)
		v1, _ := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		tr.add(t, "G")
		if err := v1.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		if _, err := v1.PutBlockBatch(ctx, tr.entries("G")); err != nil {
			t.Fatal(err)
		}
		if err := v1.Close(); err != nil {
			t.Fatal(err)
		}

		// Reopen with frees on and wait for the adoption.
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)
	})

	t.Run("cache fill of a held block during an adoption", func(t *testing.T) {
		// main holds P on an unadopted Volume, and P references B,
		// which the Volume lacks. While an adoption runs, before it
		// settles, a read fills B. The adoption counts P's holder of B
		// and leaves B to the fill's logs, so after the grace period B
		// keeps P's holder.
		store := newGuardedStore(t)
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "B")
		p := tr.add(t, "P", "B")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), []block.NamedRoot{{Name: "main", Ref: p}}); err != nil {
			t.Fatal(err)
		}
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptSettle: func() {
				err := v.withDirectFill(ctx, func(px *physicalTx) error {
					_, _, err := v.landFill(ctx, px, "b", tr.entries("B"))
					return err
				})
				if err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe, v.now); err != nil || len(probe.at) != 0 {
			t.Fatalf("adopt = %v with %d fills left", err, len(probe.at))
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "P", "B")
		requireAudit(t, v, 0)
	})
}

// testLivenessAdoption checks legacy child lists and adoption recovery.
func testLivenessAdoption(t *testing.T) {
	t.Helper()

	t.Run("unadopted", func(t *testing.T) {
		// A Volume that predates adoption logs and frees nothing.
		store := newGuardedStore(t)
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
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

		// Pk under root pk references K, which no put has stored yet.
		k := tr.add(t, "K")
		pk := tr.add(t, "Pk", "K")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("Pk"), []block.NamedRoot{{Name: "pk", Ref: pk}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 0 || res.Netted != 0 {
			t.Fatalf("unadopted pass freed %d netted %d, want nothing", res.Freed, res.Netted)
		}
		requireStored(t, v, tr, true, "R1", "E", "R2", "Pk")

		// An adoption interrupted while it settles the counts, with
		// main moving onto a new R3 referencing E while it counts,
		// leaves the Volume adopting at its cursor, and a reset returns
		// it to unadopted.
		r3 := tr.add(t, "R3", "E")
		probe := &adoptProbe{Store: collectorStore{v: v}}
		probeCtx, cancel := context.WithCancel(ctx)
		probe.at = map[int]func(){
			adoptCountRoots: func() {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("R3"), []block.NamedRoot{{Name: "main", Ref: r3, Previous: r2}}); err != nil {
					t.Error(err)
				}
			},
			adoptSettle: cancel,
		}
		if _, err := block_gc.Adopt(probeCtx, probe, v.now); !errors.Is(err, context.Canceled) || len(probe.at) != 0 {
			t.Fatalf("interrupted adopt = %v with %d writes left, want canceled after both", err, len(probe.at))
		}
		if state := livenessState(t, v); state.GetAdoption() != block_gc.Adoption_ADOPTION_ADOPTING ||
			state.GetAdoptCursor().GetPhase() != block_gc.AdoptPhase_ADOPT_PHASE_SETTLE {
			t.Fatalf("interrupted adopt left %v at %v, want adopting at the settle", state.GetAdoption(), state.GetAdoptCursor())
		}
		if err := block_gc.ResetAdoption(ctx, collectorStore{v: v}); err != nil {
			t.Fatal(err)
		}
		if state := livenessState(t, v); state.GetAdoption() != block_gc.Adoption_ADOPTION_UNADOPTED || state.GetAdoptCursor() != nil {
			t.Fatalf("reset left %v at %v, want unadopted without a cursor", state.GetAdoption(), state.GetAdoptCursor())
		}

		// A rerun counts every holder once while main moves onto a new
		// R4 referencing E before the rerun counts the roots, a root
		// aux moves onto R1 and a put stores K under a root k before
		// the rerun counts the child lists, and a put stores Y under a
		// root late before it settles. The next pass frees R2 and R3,
		// and keeps Y, born after the rerun began, and K, which Pk
		// referenced before it began.
		r4 := tr.add(t, "R4", "E")
		y := tr.add(t, "Y")
		probe = &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptCountRoots: func() {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("R4"), []block.NamedRoot{{Name: "main", Ref: r4, Previous: r3}}); err != nil {
					t.Error(err)
				}
			},
			adoptCountChildren: func() {
				if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "aux", Ref: r1}}); err != nil {
					t.Error(err)
				}
				if err := v.SetBucketRoots(ctx, "b", tr.entries("K"), []block.NamedRoot{{Name: "k", Ref: k}}); err != nil {
					t.Error(err)
				}
			},
			adoptSettle: func() {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("Y"), []block.NamedRoot{{Name: "late", Ref: y}}); err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe, v.now); err != nil || len(probe.at) != 0 {
			t.Fatalf("rerun adopt = %v with %d writes left", err, len(probe.at))
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R2", "R3")
		requireStored(t, v, tr, true, "R1", "E", "R4", "Y", "Pk", "K")
		requireAudit(t, v, 0)

		// K keeps Pk's holder once root k lets it go.
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "k", Previous: k}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "Pk", "K")
		requireAudit(t, v, 0)

		// A fresh adoption of the same Volume reaches the same counts and
		// queue.
		netted := livenessRecords(t, store, 'H', 'Q')
		markUnadopted(t, v)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		if again := livenessRecords(t, store, 'H', 'Q'); !maps.Equal(again, netted) {
			t.Fatalf("readoption counts %v, want %v", again, netted)
		}

		// Releasing every root frees everything.
		releases := []block.NamedRoot{{Name: "main", Previous: r4}, {Name: "aux", Previous: r1}, {Name: "late", Previous: y}, {Name: "pk", Previous: pk}}
		if err := v.SetBucketRoots(ctx, "b", nil, releases); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, false, "R1", "E", "R4", "Y", "Pk", "K")
		requireAudit(t, v, 0)
	})

	t.Run("incomplete format 6 list", func(t *testing.T) {
		// A format 6 store holds R -> P, X, Y and P -> Z under a named
		// root, but R's child list lacks X and Y, as a crash between
		// two slices of the old graph's edge batch left it. The
		// adoption counts no holder of X or Y and queues both, so the
		// generation alone keeps them under the root: while it is Open
		// the free parks every block it takes, the young N included.
		// The walk then runs on the adopted Volume: it refuses a
		// decoded list that drops P's child, and rewrites R's list,
		// which records that the counts are short of R's holders of X
		// and Y. The flip finds that record and returns the Volume to
		// unadopted. The recount deletes the parked and queued records,
		// and the settle frees only N.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := newGuardedStore(t)
		tr := testTree{}
		tr.add(t, "Z")
		p := tr.add(t, "P", "Z")
		tr.add(t, "X")
		y := tr.add(t, "Y")
		r := tr.add(t, "R", "P", "X", "Y")
		writeLegacyStore(t, store, keys, tr, "R", true, "Z", "P", "X", "Y", "R")
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, append([]byte{livenessPrefix[0], 'c'}, block_gc.BlockKey(r)...), block_gc.EncodeChildren([]*block.BlockRef{p})); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 2 {
			t.Fatalf("adopt of an incomplete list = %+v err %v, want X and Y queued", res, err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "y", Ref: y}}); err != nil {
			t.Fatal(err)
		}
		n := tr.add(t, "N")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("N"), []block.NamedRoot{{Name: "new", Ref: n}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "new", Previous: n}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Parked != 2 || res.Freed != 0 || res.Flipped {
			t.Fatalf("collect while Open = %+v, want X and N parked and no flip", res)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "y", Previous: y}}); err != nil {
			t.Fatal(err)
		}
		requireStored(t, v, tr, true, "R", "P", "Z", "X", "Y", "N")
		kind := func(k byte) []byte { return append(slices.Clone(livenessPrefix), k) }

		// A walk that decodes X and a P that lost its child refuses P,
		// keeps P's stored list, and writes nothing that calls for a
		// recount. The hold of Y left Y on the front.
		rebuild := func(entries ...*block.PutBatchEntry) block_gc.ListRebuildResult {
			t.Helper()
			rb, err := v.OpenListRebuild(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if err := rb.Verify(ctx, entry.Ref, entry.Refs); err != nil {
					t.Fatal(err)
				}
			}
			res, err := rb.Finish(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return res
		}
		res := rebuild(tr["X"], &block.PutBatchEntry{Ref: p})
		if res.Dropped != 1 || !res.DroppedSample[0].EqualsRef(p) || res.Verified != 1 || res.Front == 0 {
			t.Fatalf("rebuild with P dropping Z = %+v, want P refused and the front standing", res)
		}
		if got := livenessRecords(t, store, 'c')[string(kind('c'))+string(block_gc.BlockKey(p))]; got != string(block_gc.EncodeChildren(tr["P"].Refs)) {
			t.Fatal("refused rebuild changed P's stored list")
		}
		if n := countPrefix(t, store, kind('Y')); n != 0 {
			t.Fatalf("rebuild that rewrote nothing left %d recount records", n)
		}

		// A rerun that decodes R, P and the leaves rewrites R's list, which
		// records the recount the counts now owe.
		if res := rebuild(tr["X"], tr["R"], tr["P"], tr["Z"], tr["Y"]); res.Rewritten != 1 || res.Dropped != 0 {
			t.Fatalf("rerun rebuild = %+v, want R rewritten", res)
		}
		if n := countPrefix(t, store, kind('Y')); n != 1 {
			t.Fatalf("rewrite on an adopted Volume left %d recount records, want 1", n)
		}

		// The flip unadopts rather than flip. The recount counts R's
		// holders of X and Y and deletes every record from before, and
		// the settle then frees N alone.
		total := settleAll(t, v, clk, store, tr)
		if !total.Unadopted {
			t.Fatalf("settle = %+v, want the rewrite to unadopt the Volume", total)
		}
		for _, k := range []byte{'X', 'Y'} {
			if n := countPrefix(t, store, kind(k)); n != 0 {
				t.Fatalf("settle left %d records of kind %c", n, k)
			}
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "P", "Z", "X", "Y")
		requireStored(t, v, tr, false, "N")
		requireAudit(t, v, 0)

		// A rebuild of a Volume whose old generation settled is refused, since
		// a rewrite then would leave the counts short with no flip to come.
		if _, err := v.OpenListRebuild(ctx); !errors.Is(err, block_gc.ErrListsVerified) {
			t.Fatalf("rebuild of a settled Volume err %v, want ErrListsVerified", err)
		}
	})

	t.Run("leaf first, then decoded", func(t *testing.T) {
		// X's stored list is empty and unverified though its bytes
		// reference C. The walker verifies X twice in one run, as it
		// does a block it reaches under two types: first as a leaf,
		// with no refs, which marks the empty list, then with its
		// decoded refs, which must still add C, or C has no holder and
		// the settle frees it.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := newGuardedStore(t)
		tr := testTree{}
		tr.add(t, "C")
		x := tr.add(t, "X", "C")
		tr.add(t, "R", "X")
		writeLegacyStore(t, store, keys, tr, "R", true, "R", "C")
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		key := append(slices.Clone(livenessPrefix), 'c')
		if err := tx.Set(ctx, append(key, block_gc.BlockKey(x)...), block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + 1)
		collectAll(t, v)

		// Verify R, X as a leaf, X decoded, and C, then settle.
		rb, err := v.OpenListRebuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range []*block.PutBatchEntry{tr["R"], {Ref: x}, tr["X"], {Ref: tr["C"].Ref}} {
			if err := rb.Verify(ctx, entry.Ref, entry.Refs); err != nil {
				t.Fatal(err)
			}
		}
		res, err := rb.Finish(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res.MarkedLacking != 1 {
			t.Fatalf("rebuild of X leaf first = %+v, want X's marked list lacking C", res)
		}
		settleAll(t, v, clk, store, tr)
		requireStored(t, v, tr, true, "R", "X", "C")
	})

	t.Run("reput of an older block stored without a list", func(t *testing.T) {
		// A format 6 store holds Q under main, referencing P, but a
		// crash left Q's list without P and P stored without a list of
		// its own, so the adoption passes and counts nothing for P. A
		// put of P again under the new N writes P's list, and Land
		// keeps one holder more for P, since its bytes were stored. P's
		// bytes predate the put, so P is older: once a later adoption
		// recounts the lists and finds P at zero, the free parks it,
		// and Q still reaches it.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := newGuardedStore(t)
		tr := testTree{}
		tr.add(t, "P")
		q := tr.add(t, "Q", "P")
		writeLegacyStore(t, store, keys, tr, "Q", true, "Q")
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, append([]byte{livenessPrefix[0], 'c'}, block_gc.BlockKey(q)...), block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}

		// Put P again under N, then release N: while the generation is Open
		// the free parks N, and keeps P at the holder the reput kept.
		n := tr.add(t, "N", "P")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("P", "N"), []block.NamedRoot{{Name: "new", Ref: n}}); err != nil {
			t.Fatal(err)
		}
		if err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "new", Previous: n}}); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Freed != 0 || res.Parked != 1 {
			t.Fatalf("collect after the reput = %+v, want N parked and P kept", res)
		}
		requireStored(t, v, tr, true, "Q", "P", "N")

		// A recount finds N at zero, and the free parks it. P keeps N's
		// holder.
		markUnadopted(t, v)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 1 {
			t.Fatalf("readopt = %+v err %v, want N queued", res, err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Parked == 0 || res.Freed != 0 {
			t.Fatalf("collect after the recount = %+v, want N parked and nothing freed", res)
		}
		requireStored(t, v, tr, true, "Q", "P", "N")

		// The settle frees N, whose release leaves P at zero, and the walk
		// from main decodes Q and gives P its holder back before the flip,
		// so P stays.
		settleAll(t, v, clk, store, tr)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "Q", "P")
		requireStored(t, v, tr, false, "N")
		requireAudit(t, v, 0)
	})

	t.Run("parked block requeued once the generation flips", func(t *testing.T) {
		// A format 6 store holds R -> X under main and an unreached old
		// block G that also references X. Adopted with the generation
		// Open, the free parks G. Once the walk verifies R and X the
		// generation flips, and the parked G comes back as unmarked.
		// The requeued free leaves it to the sweep, which releases G's
		// holder on X once, so R's holder keeps X.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := newGuardedStore(t)
		tr := testTree{}
		tr.add(t, "X")
		tr.add(t, "R", "X")
		tr.add(t, "G", "X")
		writeLegacyStore(t, store, keys, tr, "R", true, "X", "R", "G")
		v, clk := openUnadoptedVolume(t, store)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 1 {
			t.Fatalf("adopt = %+v err %v, want G queued", res, err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Parked != 1 || res.Freed != 0 {
			t.Fatalf("collect while Open = %+v, want G parked", res)
		}
		requireStored(t, v, tr, true, "R", "X", "G")
		rb, err := v.OpenListRebuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"R", "X"} {
			if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := rb.Finish(ctx); err != nil {
			t.Fatal(err)
		}
		if res := settleAll(t, v, clk, store, tr); res.Swept != 1 || res.Freed != 0 {
			t.Fatalf("settle = %+v, want G swept and nothing freed", res)
		}
		requireStored(t, v, tr, true, "R", "X")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)
	})

	t.Run("parked block a commit names before the flip", func(t *testing.T) {
		// A format 6 store holds the root A and an unreached block C ->
		// F whose list lacks F. The adoption counts no holder of C or
		// F, and the free parks both. A commit referencing F from a new
		// block C2 is refused until it puts F again, and the walk
		// verifies A and F with C's list still cut, since no root
		// reaches C. The settle deletes C alone.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := newGuardedStore(t)
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "F")
		c := tr.add(t, "C", "F")
		writeLegacyStore(t, store, keys, tr, "A", true, "A", "F", "C")
		kind := func(k byte, parts ...[]byte) []byte {
			return slices.Concat(append(slices.Clone(livenessPrefix), k), slices.Concat(parts...))
		}
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, kind('c', block_gc.BlockKey(c)), block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 2 {
			t.Fatalf("adopt = %+v err %v, want C and F queued", res, err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		if res := collectAll(t, v); res.Parked != 2 || res.Freed != 0 {
			t.Fatalf("collect while Open = %+v, want C and F parked", res)
		}

		// C2 -> F alone is refused while F is parked at zero. With F,
		// the put gives birth to F again under C2.
		c2 := tr.add(t, "C2", "F")
		c2root := []block.NamedRoot{{Name: "c2", Ref: c2}}
		var parked *block_gc.AbsentError
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C2"), c2root); !errors.As(err, &parked) || !parked.Ref.EqualsRef(tr["F"].Ref) {
			t.Fatalf("put of C2 alone err %v, want the parked F refused", err)
		}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C2", "F"), c2root); err != nil {
			t.Fatal(err)
		}
		rb, err := v.OpenListRebuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"A", "F"} {
			if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
				t.Fatal(err)
			}
		}
		if res, err := rb.Finish(ctx); err != nil || res.Rewritten != 0 {
			t.Fatalf("rebuild = %+v err %v, want nothing rewritten", res, err)
		}
		settleAll(t, v, clk, store, tr)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "A", "C2", "F")
		requireStored(t, v, tr, false, "C")
		requireAudit(t, v, 0)
	})

	for _, verified := range []bool{true, false} {
		t.Run("refilled hole names a parked child verified="+strconv.FormatBool(verified), func(t *testing.T) {
			// A format 6 store holds the root H -> P -> C and has
			// lost P with its list, so nothing holds C. The audit
			// records P as a hole, the walk verifies H, and C too
			// when verified is set, and the free parks C. A put of
			// P lands between a pass's netting and its settle. P
			// alone is refused: its up nets only in the next pass,
			// and this pass would flip with the hole gone, leaving
			// a marked C in the run and popping an unverified C off
			// the front on its parked key. With C, the put gives
			// birth to C again under P, and the sweep deletes
			// nothing.
			ctx := t.Context()
			store := newGuardedStore(t)
			tr := testTree{}
			tr.add(t, "C")
			tr.add(t, "P", "C")
			tr.add(t, "H", "P")
			legacy := testTree{"C": tr["C"], "H": tr["H"]}
			writeLegacyStore(t, store, store_kvkey.NewDefaultKVKey(), legacy, "H", true, "C", "H")
			v, clk := openUnadoptedVolume(t, store)
			if _, err := v.Adopt(ctx); err != nil {
				t.Fatal(err)
			}
			if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
				t.Fatalf("audit recorded %d holes err %v, want P's", res.Holes, err)
			}
			walk := []string{"H"}
			if verified {
				walk = append(walk, "C")
			}
			rb, err := v.OpenListRebuild(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range walk {
				if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := rb.Finish(ctx); err != nil {
				t.Fatal(err)
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			if res := collectAll(t, v); res.Parked != 1 || res.Freed != 0 || res.Flipped {
				t.Fatalf("collect while Open = %+v, want C parked and no flip", res)
			}

			// Put P as the pass turns from netting to settling: alone, then
			// with C.
			refilled := false
			phase := func(ph block_gc.Phase) func() {
				if ph == block_gc.PhaseSweep && !refilled {
					refilled = true
					var parked *block_gc.AbsentError
					if err := v.SetBucketRoots(ctx, "b", tr.entries("P"), nil); !errors.As(err, &parked) || !parked.Ref.EqualsRef(tr["C"].Ref) {
						t.Errorf("put of P alone err %v, want the parked C refused", err)
					}
					if err := v.SetBucketRoots(ctx, "b", tr.entries("C", "P"), nil); err != nil {
						t.Error(err)
					}
				}
				return func() {}
			}
			collector := block_gc.NewCollector(collectorStore{v: v}, block_gc.CollectorOptions{Frees: true, Now: v.now, Phase: phase})
			if _, err := collector.Collect(ctx); err != nil || !refilled {
				t.Fatalf("collect err %v, refilled %v", err, refilled)
			}
			requireReads(t, v, tr, "H", "P", "C")

			// C stays through the flip and a whole sweep.
			for range 6 {
				clk.Add(3 * (block_gc.DefaultGrace + time.Second))
				collectAll(t, v)
			}
			if gen := livenessGeneration(t, v); gen != block_gc.Generation_GENERATION_SETTLED {
				t.Fatalf("generation %v, want Settled", gen)
			}
			requireReads(t, v, tr, "H", "P", "C")
			requireAudit(t, v, 0)
		})
	}

	t.Run("old list a put rewrites", func(t *testing.T) {
		// A format 6 store holds the root A and unreached blocks C ->
		// F, whose list lacks F, G and H. The walk verifies A and F. A
		// put of G, whose list is whole, writes nothing that calls for
		// a recount. A put of C under a root rewrites C's list to
		// reference F and records the recount. The flip then returns
		// the Volume to unadopted, and once the recount runs, the
		// settle deletes H, which no put revived, and keeps F under C.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := newGuardedStore(t)
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "F")
		tr.add(t, "G")
		tr.add(t, "H")
		c := tr.add(t, "C", "F")
		writeLegacyStore(t, store, keys, tr, "A", true, "A", "F", "G", "H", "C")
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		cut := slices.Concat(livenessPrefix, []byte{'c'}, block_gc.BlockKey(c))
		if err := tx.Set(ctx, cut, block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)
		if res, err := v.Adopt(ctx); err != nil || res.Queued != 4 {
			t.Fatalf("adopt = %+v err %v, want C, F, G and H queued", res, err)
		}
		rb, err := v.OpenListRebuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"A", "F"} {
			if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := rb.Finish(ctx); err != nil {
			t.Fatal(err)
		}
		rewritten := slices.Concat(livenessPrefix, []byte{'Y'})

		// G's list is whole: the put leaves no recount record.
		g := tr["G"].Ref
		if err := v.SetBucketRoots(ctx, "b", tr.entries("G"), []block.NamedRoot{{Name: "g", Ref: g}}); err != nil {
			t.Fatal(err)
		}
		if a := livenessAdoption(t, v); a != block_gc.Adoption_ADOPTION_ADOPTED || countPrefix(t, store, rewritten) != 0 {
			t.Fatalf("after the put of G the Volume is %v, want adopted and no recount", a)
		}

		// C's list lacks F: the put rewrites it and records the recount.
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: c}}); err != nil {
			t.Fatal(err)
		}
		if a := livenessAdoption(t, v); a != block_gc.Adoption_ADOPTION_ADOPTED || countPrefix(t, store, rewritten) != 1 {
			t.Fatalf("after the put of C the Volume is %v, want adopted and a recount recorded", a)
		}
		if found, err := v.GetBlockExists(ctx, tr["H"].Ref); err != nil || !found {
			t.Fatalf("exists H while Open = %v err %v, want found", found, err)
		}
		if res := settleAll(t, v, clk, store, tr); !res.Unadopted {
			t.Fatalf("settle = %+v, want the rewrite to unadopt the Volume", res)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "A", "C", "F", "G")
		requireStored(t, v, tr, false, "H")
		if found, err := v.GetBlockExists(ctx, tr["H"].Ref); err != nil || found {
			t.Fatalf("exists H after the settle = %v err %v, want absent", found, err)
		}
		requireAudit(t, v, 0)
	})

	t.Run("rewriting put supersedes an overlapping adoption", func(t *testing.T) {
		// The store of "old list a put rewrites", unadopted after the
		// format 7 upgrade, with a walk that verified A and F. Adoption
		// A marks the Volume adopting, then B claims it and counts the
		// lists, C's still lacking F. Before B settles, a put of C
		// rewrites C's list, and since B counted it, logs F's new
		// holder. A is superseded by B's claim. B settles owing no
		// recount, the settle takes G, which nothing references, and F
		// keeps its holder.
		ctx := t.Context()
		keys := store_kvkey.NewDefaultKVKey()
		store := newGuardedStore(t)
		tr := testTree{}
		tr.add(t, "A")
		tr.add(t, "F")
		tr.add(t, "G")
		c := tr.add(t, "C", "F")
		writeLegacyStore(t, store, keys, tr, "A", true, "A", "F", "G", "C")
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		cut := slices.Concat(livenessPrefix, []byte{'c'}, block_gc.BlockKey(c))
		if err := tx.Set(ctx, cut, block_gc.EncodeChildren(nil)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		v, clk := openUnadoptedVolume(t, store)

		// B runs inside A, after A's mark, and takes A's run over. The
		// put of C lands inside B, after B counted the lists.
		var errB error
		probeB := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			resumeSettle: func() {
				if err := v.SetBucketRoots(ctx, "b", tr.entries("C"), []block.NamedRoot{{Name: "c", Ref: c}}); err != nil {
					t.Error(err)
				}
			},
		}}
		probeA := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptCountRoots: func() { _, errB = block_gc.Adopt(ctx, probeB, v.now) },
		}}
		_, errA := block_gc.Adopt(ctx, probeA, v.now)
		if !errors.Is(errA, block_gc.ErrAdoptSuperseded) || errB != nil {
			t.Fatalf("adoptions = %v and %v with %d and %d hooks left, want A superseded and B adopted", errA, errB, len(probeA.at), len(probeB.at))
		}
		if n := countPrefix(t, store, slices.Concat(livenessPrefix, []byte{'Y'})); n != 0 {
			t.Fatalf("the put of C left %d recount records, want none", n)
		}
		verify := func(names ...string) {
			rb, err := v.OpenListRebuild(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range names {
				if err := rb.Verify(ctx, tr[name].Ref, tr[name].Refs); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := rb.Finish(ctx); err != nil {
				t.Fatal(err)
			}
		}
		verify("A", "F")
		if res := settleAll(t, v, clk, store, tr); res.Unadopted {
			t.Fatalf("settle = %+v, want no recount", res)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "A", "C", "F")
		requireStored(t, v, tr, false, "G")
		requireAudit(t, v, 0)
	})

	t.Run("adoption claimed by another", func(t *testing.T) {
		// main -> R -> X on an unadopted Volume. Adoption A clears,
		// then B, another process, starts while the Volume is still
		// unadopted. A marks the Volume adopting and counts the roots.
		// B clears A's counts and dies before it marks. A must not
		// settle the cleared counts: B's claim superseded it, and
		// nothing main reaches is freed.
		store := newGuardedStore(t)
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}

		// B claims, waits before its first clear, clears on release, and dies
		// before it marks.
		claimed, cleared, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
		releaseB := sync.OnceFunc(func() { close(release) })
		defer releaseB()
		bCtx, bCancel := context.WithCancel(ctx)
		defer bCancel()
		probeB := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptClearCounts: func() { close(claimed); <-release },
			adoptMark:        bCancel,
		}}
		go func() {
			defer close(cleared)
			_, _ = block_gc.Adopt(bCtx, probeB, v.now)
		}()

		// A starts B before its mark and lets B clear before it settles.
		probeA := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptMark: func() { <-claimed },
			adoptSettle: func() {
				releaseB()
				<-cleared
			},
		}}
		_, err := block_gc.Adopt(ctx, probeA, v.now)
		releaseB()
		<-cleared
		if !errors.Is(err, block_gc.ErrAdoptSuperseded) {
			t.Fatalf("claimed adoption = %v, want superseded", err)
		}

		// Nothing is freed, and a clean adoption keeps main's tree.
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		requireAudit(t, v, 0)
	})

	t.Run("overlapping adoptions", func(t *testing.T) {
		// main -> R -> X on an unadopted Volume. Adoption A counts the
		// holders, and before it settles them a second adoption B takes A's
		// run over and finishes it. A must not settle the counts again: it
		// refuses, B's adoption stands, and nothing main reaches is freed.
		store := newGuardedStore(t)
		setUnadopted(t, store)
		v, clk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		r := tr.add(t, "R", "X")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "R"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			adoptSettle: func() {
				if _, err := block_gc.Adopt(ctx, collectorStore{v: v}, v.now); err != nil {
					t.Error(err)
				}
			},
		}}
		if _, err := block_gc.Adopt(ctx, probe, v.now); !errors.Is(err, block_gc.ErrAdoptSuperseded) || len(probe.at) != 0 {
			t.Fatalf("superseded adopt = %v with %d adoptions left, want superseded", err, len(probe.at))
		}
		if adoption := livenessAdoption(t, v); adoption != block_gc.Adoption_ADOPTION_ADOPTED {
			t.Fatalf("overlapping adoptions left %v, want adopted", adoption)
		}
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireStored(t, v, tr, true, "R", "X")
		requireAudit(t, v, 0)
	})

	t.Run("adoption waited for under a context", func(t *testing.T) {
		// A caller that finds an adoption running, as a command does while
		// the background adoption counts, leaves when its own context ends
		// instead of waiting the run out.
		v, _ := openUnadoptedVolume(t, newGuardedStore(t))
		unlock, err := v.adoptMu.Lock(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := v.Adopt(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("adopt under a held turn = %v, want canceled", err)
		}
	})

	t.Run("adoption resumed across reopens", func(t *testing.T) {
		// An unadopted Volume holds about twenty slices of records
		// across the passes. Adoptions are cancelled at random slices
		// three times, each taking over the cursor the one before left,
		// with the Volume reopening and Land moving, adding and
		// releasing roots in each gap. A reopen's background adoption
		// then finishes the run. Its counts, queue and front equal
		// those a clean adoption reaches on a copy.
		seed := uint64(time.Now().UnixNano())
		rng := rand.New(rand.NewPCG(seed, 0))
		t.Logf("seed %d", seed)
		store := newGuardedStore(t)
		setUnadopted(t, store)
		v, clk := openLivenessVolume(t, store, true)
		waitAdopted(t, v)
		ctx := t.Context()

		// Fifty roots each hold a parent of forty leaves of its own and
		// two of twenty shared ones, beside a hundred blocks no holder
		// references.
		tr := testTree{}
		var names []string
		leaf := func(name string) string {
			tr.add(t, name)
			names = append(names, name)
			return name
		}
		for i := range 20 {
			leaf("S" + strconv.Itoa(i))
		}
		var roots []block.NamedRoot
		for i := range 50 {
			kids := []string{"S" + strconv.Itoa(i%20), "S" + strconv.Itoa((i+7)%20)}
			for j := range 40 {
				kids = append(kids, leaf("L"+strconv.Itoa(i)+"."+strconv.Itoa(j)))
			}
			p := tr.add(t, "P"+strconv.Itoa(i), kids...)
			names = append(names, "P"+strconv.Itoa(i))
			roots = append(roots, block.NamedRoot{Name: "r" + strconv.Itoa(i), Ref: p})
		}
		for i := range 100 {
			leaf("G" + strconv.Itoa(i))
		}
		observePuts(v)
		if err := v.SetBucketRoots(ctx, "b", tr.entries(names...), roots); err != nil {
			t.Fatal(err)
		}
		markUnadopted(t, v)

		// Each round stops the reopened Volume's background adoption, which
		// took the run left at the previous cancel over, and cancels the run
		// before a random slice, then lands a move, an add and a release
		// before the next reopen. A run the background finished, or one that
		// finishes before its cancel, is marked unadopted so a fresh run is
		// cancelled instead.
		for round := range 3 {
			// Stop the background adoption, which leaves the run at its
			// cursor if it did not finish it.
			v.adoptCancel()
			<-v.adoptDone
			first := resumeCountRoots
			if livenessState(t, v).GetAdoption() != block_gc.Adoption_ADOPTION_ADOPTING {
				markUnadopted(t, v)
				first = adoptCountRoots
			}

			// Cancel before a random slice of the passes.
			for {
				at := first + 1 + rng.IntN(16)
				adoptCtx, cancel := context.WithCancel(ctx)
				probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){at: cancel}}
				_, err := block_gc.Adopt(adoptCtx, probe, v.now)
				cancel()
				if livenessState(t, v).GetAdoption() == block_gc.Adoption_ADOPTION_ADOPTED && first == resumeCountRoots {
					// The resumed run finished before the cancel, which at
					// most interrupted its deletion of the marks.
					t.Logf("round %d: resumed run finished before transaction %d", round, at)
					markUnadopted(t, v)
					first = adoptCountRoots
					continue
				}
				if !errors.Is(err, context.Canceled) || len(probe.at) != 0 {
					t.Fatalf("round %d: adopt = %v with %d cancels left, want canceled", round, err, len(probe.at))
				}
				t.Logf("round %d: canceled before transaction %d of a run from transaction %d", round, at, first)
				break
			}
			state := livenessState(t, v)
			if state.GetAdoption() != block_gc.Adoption_ADOPTION_ADOPTING || state.GetAdoptCursor() == nil {
				t.Fatalf("round %d: cancel left %v at %v, want adopting at a cursor", round, state.GetAdoption(), state.GetAdoptCursor())
			}

			// Move a root onto a new parent of a new leaf and two
			// stored leaves, reference an unheld block, and release
			// another root.
			r := strconv.Itoa(round)
			tr.add(t, "N"+r)
			m := tr.add(t, "M"+r, "N"+r, "L"+r+".0", "S"+r)
			err := v.SetBucketRoots(ctx, "b", tr.entries("N"+r, "M"+r), []block.NamedRoot{
				{Name: "r" + r, Ref: m, Previous: tr["P"+r].Ref},
				{Name: "g" + r, Ref: tr["G"+r].Ref},
				{Name: "r" + strconv.Itoa(49-round), Previous: tr["P"+strconv.Itoa(49-round)].Ref},
			})
			if err != nil {
				t.Fatal(err)
			}

			// Reopen. The next round takes the run over.
			if err := v.Close(); err != nil {
				t.Fatal(err)
			}
			v, clk = openLivenessVolume(t, store, true)
		}

		// The reopened Volume's background adoption finishes the run, and
		// the logs net once Grace has passed.
		waitAdopted(t, v)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireAudit(t, v, 0)
		resumed := adoptedRecords(t, store)

		// A clean adoption of a copy reaches the same counts, queue and
		// front.
		cp := copyAll(t, store)
		vc, _ := openLivenessVolume(t, cp, true)
		markUnadopted(t, vc)
		if _, err := vc.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		if clean := adoptedRecords(t, cp); !maps.Equal(clean, resumed) {
			t.Fatalf("resumed adoption left %d records, a clean one %d; differing: %v",
				len(resumed), len(clean), recordsDiff(resumed, clean))
		}
	})

	t.Run("adoption resumed at a phase change", func(t *testing.T) {
		// An adoption cancelled after the last slice of the child list count
		// commits and before the settle's first resumes at the settle: the
		// taken-over run counts no child list again, so its counts equal a
		// clean adoption's.
		store := newGuardedStore(t)
		setUnadopted(t, store)
		v, _ := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		tr.add(t, "Y")
		r := tr.add(t, "R", "X", "Y")
		tr.add(t, "G")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "Y", "R", "G"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		clean := copyAll(t, store)

		// Cancel before the settle's first slice.
		adoptCtx, cancel := context.WithCancel(ctx)
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){adoptSettle: cancel}}
		_, err := block_gc.Adopt(adoptCtx, probe, v.now)
		cancel()
		if !errors.Is(err, context.Canceled) || len(probe.at) != 0 {
			t.Fatalf("adopt = %v with %d cancels left, want canceled", err, len(probe.at))
		}
		cur := livenessState(t, v).GetAdoptCursor()
		if cur.GetPhase() != block_gc.AdoptPhase_ADOPT_PHASE_SETTLE || len(cur.GetAfter()) != 0 {
			t.Fatalf("cancel left the cursor at %v, want the settle's start", cur)
		}
		counted := livenessRecords(t, store, 'H')

		// The run taken over leaves the counts alone until it settles.
		probe = &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){
			resumeSettle: func() {
				if got := livenessRecords(t, store, 'H'); !maps.Equal(got, counted) {
					t.Errorf("resume changed the counts before the settle: %v, want %v", got, counted)
				}
			},
		}}
		res, err := block_gc.Adopt(ctx, probe, v.now)
		if err != nil || len(probe.at) != 0 {
			t.Fatalf("resumed adopt = %v with %d hooks left", err, len(probe.at))
		}
		if res.Roots != 1 || res.Blocks != 4 || res.Queued != 1 {
			t.Fatalf("resumed adopt counted %+v, want 1 root, 4 blocks and 1 queued", res)
		}

		// A clean adoption of the copy reaches the same records.
		vc, _ := openUnadoptedVolume(t, clean)
		if _, err := vc.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		if got, want := adoptedRecords(t, store), adoptedRecords(t, clean); !maps.Equal(got, want) {
			t.Fatalf("resumed adoption differs from a clean one: %v", recordsDiff(got, want))
		}
	})

	t.Run("adoption refuses a cursor another run left", func(t *testing.T) {
		// A run cancelled at the settle leaves its cursor. A build
		// without the cursor then claims the Volume and marks its own
		// run adopting, keeping the field it does not know and clearing
		// the counts. An Adopt finding that cursor under a run it does
		// not identify starts over from a claim instead of settling the
		// empty counts.
		store := newGuardedStore(t)
		setUnadopted(t, store)
		v, _ := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "X")
		tr.add(t, "Y")
		r := tr.add(t, "R", "X", "Y")
		tr.add(t, "G")
		if err := v.SetBucketRoots(ctx, "b", tr.entries("X", "Y", "R", "G"), []block.NamedRoot{{Name: "main", Ref: r}}); err != nil {
			t.Fatal(err)
		}
		clean := copyAll(t, store)

		// Cancel before the settle's first slice.
		adoptCtx, cancel := context.WithCancel(ctx)
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){adoptSettle: cancel}}
		_, err := block_gc.Adopt(adoptCtx, probe, v.now)
		cancel()
		if !errors.Is(err, context.Canceled) || len(probe.at) != 0 {
			t.Fatalf("adopt = %v with %d cancels left, want canceled", err, len(probe.at))
		}

		// The older build clears the counts and marks its run adopting,
		// carrying the cursor through as an unknown field.
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for key := range livenessRecords(t, store, 'H') {
			if err := tx.Delete(ctx, []byte(key)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		err = collectorStore{v: v}.Update(ctx, func(rec *block_gc.Records, _ block.StoreOps) error {
			state, err := rec.GetState(ctx)
			if err != nil {
				return err
			}
			state.AdoptionRun = "older-build-run"
			return rec.SetState(ctx, state)
		})
		if err != nil {
			t.Fatal(err)
		}

		// The Adopt counts every list again, as a clean one does.
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		vc, _ := openUnadoptedVolume(t, clean)
		if _, err := vc.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		if got, want := adoptedRecords(t, store), adoptedRecords(t, clean); !maps.Equal(got, want) {
			t.Fatalf("adoption over a foreign cursor differs from a clean one: %v", recordsDiff(got, want))
		}
	})

	t.Run("adoption cleanup spares a newer run's marks", func(t *testing.T) {
		// An Adopt that finds the Volume adopted deletes the marks a
		// finished run left. Before its cleanup runs, the Volume
		// returns to unadopted and a newer run passes its roots, and a
		// Land gives birth to X under a new root, marking X born. The
		// stale cleanup must leave that mark, or the newer run's settle
		// counts X as an old block no list references and queues it.
		store := newGuardedStore(t)
		setUnadopted(t, store)
		v, _ := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		tr.add(t, "Y")
		r := tr.add(t, "R", "Y")
		x := tr.add(t, "X")
		main := block.NamedRoot{Name: "main", Ref: r}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("Y", "R"), []block.NamedRoot{main}); err != nil {
			t.Fatal(err)
		}
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}

		// Run the newer run to its child lists and give birth to X before
		// the stale run's first cleanup transaction.
		newer := func() {
			markUnadopted(t, v)
			newerCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){adoptCountChildren: cancel}}
			if _, err := block_gc.Adopt(newerCtx, probe, v.now); !errors.Is(err, context.Canceled) {
				t.Fatalf("newer adopt = %v, want canceled", err)
			}
			if err := v.SetBucketRoots(ctx, "b", tr.entries("X"), []block.NamedRoot{{Name: "x", Ref: x}}); err != nil {
				t.Fatal(err)
			}
		}
		stale := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){2: newer}}
		res, err := block_gc.Adopt(ctx, stale, v.now)
		if err != nil || !res.Already || len(stale.at) != 0 {
			t.Fatalf("stale adopt = %+v, %v with %d probes left", res, err, len(stale.at))
		}

		// The newer run finishes and agrees with a clean recount.
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		clean := copyAll(t, store)
		vc, _ := openUnadoptedVolume(t, clean)
		markUnadopted(t, vc)
		if _, err := vc.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		if got, want := adoptedRecords(t, store), adoptedRecords(t, clean); !maps.Equal(got, want) {
			t.Fatalf("adoption after a stale cleanup differs from a clean one: %v", recordsDiff(got, want))
		}
	})

	t.Run("adoption counts a hole refilled behind its scan", func(t *testing.T) {
		// P references X, which references C, and a root holds C too.
		// The store loses X and an audit records the hole. An adoption
		// counts the child lists while X has none, and a put refills X
		// before the settle: X's list gives C a holder the scan never
		// counted, so the refill logs it, and the adoption agrees with
		// a clean recount.
		store := newGuardedStore(t)
		setUnadopted(t, store)
		v, vclk := openUnadoptedVolume(t, store)
		ctx := t.Context()
		tr := testTree{}
		c := tr.add(t, "C")
		x := tr.add(t, "X", "C")
		p := tr.add(t, "P", "X")
		roots := []block.NamedRoot{{Name: "main", Ref: p}, {Name: "c", Ref: c}}
		if err := v.SetBucketRoots(ctx, "b", tr.entries("C", "X", "P"), roots); err != nil {
			t.Fatal(err)
		}
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		punchHole(t, v, x)
		if res, err := v.Audit(ctx); err != nil || res.Holes != 1 {
			t.Fatalf("audit recorded %d holes err %v, want 1", res.Holes, err)
		}

		// Count the lists with X missing, then refill X before the settle.
		markUnadopted(t, v)
		adoptCtx, cancel := context.WithCancel(ctx)
		probe := &adoptProbe{Store: collectorStore{v: v}, at: map[int]func(){adoptSettle: cancel}}
		_, err := block_gc.Adopt(adoptCtx, probe, v.now)
		cancel()
		if !errors.Is(err, context.Canceled) || len(probe.at) != 0 {
			t.Fatalf("adopt = %v with %d cancels left, want canceled", err, len(probe.at))
		}
		if _, err := v.PutBlockBatch(ctx, tr.entries("X")); err != nil {
			t.Fatal(err)
		}

		// The adoption settles, and once the logs net, the refill's own
		// hold of X included, its counts match a clean recount's.
		if _, err := v.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		clean := copyAll(t, store)
		vc, clk := openUnadoptedVolume(t, clean)
		markUnadopted(t, vc)
		if _, err := vc.Adopt(ctx); err != nil {
			t.Fatal(err)
		}
		vclk.Add(block_gc.DefaultGrace + time.Second)
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		collectAll(t, vc)
		if got, want := adoptedRecords(t, store), adoptedRecords(t, clean); !maps.Equal(got, want) {
			t.Fatalf("adoption over a refilled hole differs from a clean one: %q", recordsDiff(got, want))
		}
	})
}
