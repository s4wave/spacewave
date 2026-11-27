package kvtx

import (
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// testLivenessSettleWriters checks SQLite writers across the settle.
func testLivenessSettleWriters(t *testing.T,
	oldStoreOn func(t *testing.T, store kvtx.Store, tr testTree, root string, listed []string, cut ...string) (*Volume, *testClock),
	verify func(t *testing.T, v *Volume, tr testTree, names ...string) block_gc.ListRebuildResult,
	flipped func(res block_gc.CollectResult) bool,
	settled func(res block_gc.CollectResult) bool,
	pairs func(t *testing.T, tr testTree, n int) (listed, cut []string),
	run []byte,
) {
	t.Helper()

	t.Run("writers across the settle on SQLite", func(t *testing.T) {
		// B<i> -> C<i> -> D<i> for 150 i, with each C's list cut, on a
		// SQLite store, whose readers do not wait on its writer. Two writers
		// run through the whole settle. One commits after every collector
		// transaction, so between every pair of slices and across the flip:
		// it roots a new name onto B<step mod 50>, putting its chain again, and
		// releases the name it rooted five steps before. The other runs
		// beside the collector, rooting c<k> onto B<k+50> and, for even k,
		// e<k> onto B<k>. Every block a held root reaches stays, the audit
		// is clean, and B, C and D from 100 to 139, which nothing roots, are
		// swept.
		ctx := t.Context()
		store, err := store_kvtx_sqlite.Open(ctx, filepath.Join(t.TempDir(), "volume.db"), "kv")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		})
		tr := testTree{}
		listed, cut := pairs(t, tr, 150)
		v, clk := oldStoreOn(t, newOrderedSeekGuard(t, store), tr, "A", listed, cut...)
		verify(t, v, tr, "A")
		reached := func(b int) []string {
			s := strconv.Itoa(b)
			return []string{"B" + s, "C" + s, "D" + s}
		}
		// setRoot roots name onto B<b>, putting B, C and D again: a
		// chain let go before the flip is unmarked, and a put of B
		// alone would reference a C the sweep deletes, as a put
		// referencing a child never stored.
		setRoot := func(name string, b int) error {
			names := reached(b)
			return v.SetBucketRoots(ctx, "b", tr.entries(names...), []block.NamedRoot{{Name: name, Ref: tr[names[0]].Ref}})
		}

		// The paced writer: step's root holds B<step mod 50>, and the root
		// five steps back is released. While Sweeping, it also roots r<j>
		// onto B<140+j>, unmarked at the flip, putting B, C and D again.
		var steps, reborn int
		probe := &paceProbe{Store: collectorStore{v: v}, limit: 1 << 20, after: func() {
			requireFront(t, v)
			if err := setRoot("s"+strconv.Itoa(steps), steps%50); err != nil {
				t.Fatal(err)
			}
			if reborn < 10 && livenessGeneration(t, v) == block_gc.Generation_GENERATION_SWEEPING {
				names := reached(140 + reborn)
				err := v.SetBucketRoots(ctx, "b", tr.entries(names...), []block.NamedRoot{{Name: "r" + strconv.Itoa(reborn), Ref: tr[names[0]].Ref}})
				if err != nil {
					t.Fatal(err)
				}
				reborn++
			}
			if old := steps - 5; old >= 0 {
				err := v.SetBucketRoots(ctx, "b", nil, []block.NamedRoot{{Name: "s" + strconv.Itoa(old), Previous: tr["B"+strconv.Itoa(old%50)].Ref}})
				if err != nil {
					t.Fatal(err)
				}
			}
			steps++
		}}

		// The concurrent writer.
		var wg sync.WaitGroup
		var werr error
		wg.Go(func() {
			for k := range 50 {
				if werr = setRoot("c"+strconv.Itoa(k), k+50); werr != nil {
					return
				}
				if k%2 == 0 {
					if werr = setRoot("e"+strconv.Itoa(k), k); werr != nil {
						return
					}
				}
				time.Sleep(time.Millisecond)
			}
		})

		// Settle through the paced store.
		c := block_gc.NewCollector(probe, block_gc.CollectorOptions{Frees: true, Now: v.now})
		var flipped, settled bool
		var freed, swept uint64
		for round := 0; livenessGeneration(t, v) != block_gc.Generation_GENERATION_SETTLED; round++ {
			if round == 32 {
				t.Fatalf("the old generation never settled: it is %v", livenessGeneration(t, v))
			}
			if livenessGeneration(t, v) == block_gc.Generation_GENERATION_OPEN && livenessAdoption(t, v) == block_gc.Adoption_ADOPTION_ADOPTED {
				walkFront(t, v, store, tr)
			}
			if livenessAdoption(t, v) == block_gc.Adoption_ADOPTION_UNADOPTED {
				if _, err := v.Adopt(ctx); err != nil {
					t.Fatal(err)
				}
			}
			clk.Add(block_gc.DefaultGrace + time.Second)
			for {
				res, err := c.Collect(ctx)
				if err != nil {
					t.Fatal(err)
				}
				flipped = flipped || res.Flipped
				settled = settled || res.Settled
				freed += res.Freed
				swept += res.Swept
				if !res.More {
					break
				}
			}
		}
		wg.Wait()
		if werr != nil {
			t.Fatal(werr)
		}
		if !flipped || !settled {
			t.Fatalf("flipped %v, settled %v, want both", flipped, settled)
		}
		t.Logf("settled in %d collector transactions, %d swept, %d freed, %d rooted while Sweeping", steps, swept, freed, reborn)
		if reborn == 0 {
			t.Fatal("no writer rooted an unmarked block while Sweeping")
		}

		// Every held root's blocks stay. Nothing rooted past 100 does.
		var held []string
		for k := range 50 {
			held = append(held, reached(k+50)...)
			if k%2 == 0 {
				held = append(held, reached(k)...)
			}
		}
		for step := max(steps-5, 0); step < steps; step++ {
			held = append(held, reached(step%50)...)
		}
		for j := range reborn {
			held = append(held, reached(140+j)...)
		}
		requireStored(t, v, tr, true, held...)
		for b := 100; b < 140; b++ {
			requireStored(t, v, tr, false, reached(b)...)
		}

		// The paced writer's last releases follow the collector's last
		// transaction. Net them, with the writers stopped, before the
		// audit.
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		requireAudit(t, v, 0)
	})
}
