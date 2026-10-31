//go:build !js

package sobject_world_engine_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
)

// TestDurableReaderSurvivesReclaim holds a reader on the durable World while
// the admitted head moves several revisions past it, deleting objects the
// durable World holds, and a sweep collects every block no root holds. Every
// block the durable root references stays readable through the reader. Once
// the durable head catches up through every admitted World, the old reader
// and one on the new durable root read every block of their roots after
// another sweep. Once the old readers leave, the old root's blocks go, so the
// earlier sweeps could have taken them.
//
// It runs with the engine storing every World, where the volume sweep is the
// collector, and with the page view keeping the World pages, where each
// commit's page sweep is: there the old root never reaches the volume, and
// the durable head and the Worlds queued for it must stay live in the view.
func TestDurableReaderSurvivesReclaim(t *testing.T) {
	t.Run("stored", func(t *testing.T) { testDurableReaderSurvivesReclaim(t, true) })
	t.Run("kept", func(t *testing.T) { testDurableReaderSurvivesReclaim(t, false) })
}

// testDurableReaderSurvivesReclaim runs TestDurableReaderSurvivesReclaim with
// the engine storing every World or keeping the World pages.
func testDurableReaderSurvivesReclaim(t *testing.T, storeEveryWorld bool) {
	// Commit the durable World the reader holds.
	w, so := startGatedWorldConf(t, t.TempDir(), nil, func(conf *sobject_world_engine.Config) {
		conf.StoreEveryWorld = storeEveryWorld
	})
	const objects = 8
	for i := range objects {
		w.create(t, "reclaim/"+strconv.Itoa(i))
	}
	durable := worldSeqno(t, w)

	// Hold a reader on it, then hold the receipts of the next publications,
	// which the volume commits at once, so the durable head lags the admitted
	// one while a sweep can run.
	old := newRead(t, w)
	defer old.Discard()
	oldRoot, oldBlocks := readWorldBlocks(t, old)
	release := so.holdReceipts()
	defer release()

	// Move the admitted head a checkpoint's worth of revisions, deleting each
	// durable object, so the durable root's blocks leave the head.
	var adms []world.Admission
	for i := range sobject.MinCheckpointOperations {
		tx := w.newWrite(t, "moved/"+strconv.Itoa(i))
		if i < objects {
			if _, err := tx.DeleteObject(t.Context(), "reclaim/"+strconv.Itoa(i)); err != nil {
				tx.Discard()
				t.Fatal(err)
			}
		}
		adm, err := world.CommitAdmit(t.Context(), tx)
		if err != nil {
			t.Fatal(err)
		}
		defer adm.Release()
		adms = append(adms, adm)
	}
	if seqno := worldSeqno(t, w); seqno != durable {
		t.Fatalf("durable seqno %d with the publication held, want %d", seqno, durable)
	}

	// Sweep, then read every block of the durable root through the reader.
	sweep(t, w)
	if root, n := readWorldBlocks(t, old); !root.EqualVT(oldRoot) || n != oldBlocks {
		t.Fatalf("lagging reader read %d blocks of %s after the sweep, want %d of %s", n, root.MarshalString(), oldBlocks, oldRoot.MarshalString())
	}
	lagging := newRead(t, w)
	defer lagging.Discard()
	if root, n := readWorldBlocks(t, lagging); !root.EqualVT(oldRoot) || n != oldBlocks {
		t.Fatalf("new reader of the lagging durable World read %d blocks of %s, want %d of %s", n, root.MarshalString(), oldBlocks, oldRoot.MarshalString())
	}
	for i := range objects {
		if !worldHas(t, old, "reclaim/"+strconv.Itoa(i)) {
			t.Fatalf("lagging reader lost reclaim/%d", i)
		}
	}

	// Let the durable head catch up through every admitted World. A World
	// whose pages are gone stalls the durable head, so bound the wait.
	release()
	waitCtx, waitCancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer waitCancel()
	for i, adm := range adms {
		if seqno, err := adm.WaitDurable(waitCtx); err != nil || seqno != adm.Seqno() {
			t.Fatalf("write %d WaitDurable = %d, %v, want %d", i, seqno, err, adm.Seqno())
		}
	}
	last := adms[len(adms)-1].Seqno()
	if seqno := worldSeqno(t, w); seqno != last {
		t.Fatalf("durable seqno %d after catching up, want %d", seqno, last)
	}

	// Wait for the checkpoint to trim the operations, so the replay save no
	// longer holds the old root and only its readers do.
	durableState, relDurable, err := so.AccessSharedObjectState(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relDurable()
	_, err = durableState.WaitValueWithValidator(t.Context(), func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		return checkpointTrimmed(t.Context(), snap)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Sweep again: the old readers and one on the new durable root read
	// every block of their roots.
	cur := newRead(t, w)
	defer cur.Discard()
	curRoot, curBlocks := readWorldBlocks(t, cur)
	if curRoot.EqualVT(oldRoot) {
		t.Fatal("durable root did not move after catching up")
	}
	sweep(t, w)
	for _, tx := range []world.Tx{old, lagging} {
		if root, n := readWorldBlocks(t, tx); !root.EqualVT(oldRoot) || n != oldBlocks {
			t.Fatalf("old reader read %d blocks of %s after catching up, want %d of %s", n, root.MarshalString(), oldBlocks, oldRoot.MarshalString())
		}
	}
	if root, n := readWorldBlocks(t, cur); !root.EqualVT(curRoot) || n != curBlocks {
		t.Fatalf("durable reader read %d blocks of %s, want %d of %s", n, root.MarshalString(), curBlocks, curRoot.MarshalString())
	}
	for i := range objects {
		if worldHas(t, cur, "reclaim/"+strconv.Itoa(i)) || !worldHas(t, cur, "moved/"+strconv.Itoa(i)) {
			t.Fatalf("durable World after catching up holds the wrong reclaim/%d or moved/%d", i, i)
		}
	}

	// Release the writes and the old readers, and commit a write that carries
	// the removal of their pins and sweeps the kept pages, then sweep the
	// volume: the old root's own blocks go, so the sweeps above could free
	// them.
	for _, adm := range adms {
		adm.Release()
	}
	old.Discard()
	lagging.Discard()
	w.create(t, "after")
	if n := sweep(t, w); storeEveryWorld && n == 0 {
		t.Fatal("sweep after releasing the old readers removed nothing")
	}
	err = cur.AccessWorldState(t.Context(), nil, func(c *bucket_lookup.Cursor) error {
		return block.WalkGraph(t.Context(), c.GetBlockStore(), []*block.BlockRef{oldRoot}, 0, nil)
	})
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("old root after its readers left: %v, want a swept block", err)
	}
	if root, n := readWorldBlocks(t, cur); !root.EqualVT(curRoot) || n != curBlocks {
		t.Fatalf("durable reader read %d blocks of %s after the last sweep, want %d of %s", n, root.MarshalString(), curBlocks, curRoot.MarshalString())
	}
}

// readWorldBlocks reads every block the World root of tx references through
// the store of tx and returns the root and the number of blocks.
func readWorldBlocks(t *testing.T, tx world.Tx) (*block.BlockRef, int) {
	// Walk the graph under the root, counting each block read.
	t.Helper()
	var root *block.BlockRef
	var n int
	err := tx.AccessWorldState(t.Context(), nil, func(cur *bucket_lookup.Cursor) error {
		root = cur.GetRef().GetRootRef()
		return block.WalkGraph(t.Context(), cur.GetBlockStore(), []*block.BlockRef{root}, 0, func(*block.BlockRef, *block.StoredBlock) error {
			n++
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return root, n
}

// sweep collects every block of the volume of w that no root holds and
// returns the number of blocks it removed.
func sweep(t *testing.T, w *localWorld) int {
	// Collect over the volume's reference graph.
	t.Helper()
	rg := w.vol.(kvtx_volume.KvtxVolume).GetRefGraph()
	stats, err := block_gc.NewCollector(rg, w.vol, nil).Collect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return stats.BlocksSwept
}
