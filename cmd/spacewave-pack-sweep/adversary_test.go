package main

import (
	"slices"
	"testing"

	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packsweep"
)

// TestCommandAdversary runs whole cycles under the writer while an adversary
// acts on blocks the cycle's walk found dead, before each step: it links one
// from a new root, puts one again and links it, or reads one, the put and the
// read as the write-back store logs and drains or fills them. A block it
// linked or put before the trash reads through every later step and cycle,
// and one it read before the trash reads at the cycle's end. A block it linked
// or read after the trash may be gone, as the sweep's model allows: a restore
// naming the cycle's removed packs brings each such block back.
func TestCommandAdversary(t *testing.T) {
	commandRuns(t, testCommandAdversary)
}

// testCommandAdversary runs the adversary with the pack index in a file of
// the volume type kind, as a rehearsal when rehearsal is set.
func testCommandAdversary(t *testing.T, kind string, rehearsal bool) {
	// Start the run-through with the adversary acting before each step.
	r, faults, _, stopWriter := newRunThrough(t, kind, rehearsal)
	defer r.stop()
	defer stopWriter()
	a := &adversary{r: r, trashAt: slices.Index(runSteps, "trash")}
	r.before = a.act

	// Run cycles, settling the adversary's blocks after each.
	var removed, lifted uint
	for range 4 {
		res := r.cycle(-1, faults)
		removed += res.removed
		lifted += res.lifted
		a.settle()
		r.checkReads("settle")
	}
	t.Logf("removed %d, lifted %d, acts %v, lost %d", removed, lifted, a.acts, a.lost)
	if removed == 0 || lifted == 0 || slices.Contains(a.acts[:], 0) {
		t.Fatalf("removed %d, lifted %d, acts %v: want each nonzero", removed, lifted, a.acts)
	}
}

// adversary acts on dead blocks within a run-through's cycles.
type adversary struct {
	r *runThrough
	// trashAt is the index of the trash step.
	trashAt int
	// reads are the blocks read before the trash, and lateReads those read
	// after it.
	reads, lateReads []*block.BlockRef
	// acts counts the links, puts, reads and late acts; lost counts the late
	// blocks a restore brought back.
	acts [4]int
	lost int
}

// act takes a dead block and acts on it before step i: by the cycle and the
// step, it links it, puts it again and links it, or reads it.
func (a *adversary) act(i int) {
	// Take the next dead block.
	r := a.r
	r.mtx.Lock()
	if len(r.prey) == 0 {
		r.mtx.Unlock()
		return
	}
	ref := r.prey[0]
	r.prey = r.prey[1:]
	// A link is late once the recheck passed, before the trash; a read once
	// the trash's last check passed, after it. A put is never late.
	kind := (r.cycles + i) % 3
	late := (kind == 0 && i >= a.trashAt) || (kind == 2 && i > a.trashAt)
	if late {
		a.acts[3]++
	}

	// Act on it, the root linking a block linked or put from the next write.
	switch kind {
	case 0:
		if late {
			r.late = append(r.late, ref)
		} else {
			r.linked = append(r.linked, ref)
		}
	case 1:
		if err := a.put(ref); err != nil {
			r.mtx.Unlock()
			r.t.Fatalf("put of dead block before %s: %v", runSteps[i], err)
		}
		r.linked = append(r.linked, ref)
	case 2:
		found, err := a.fill(ref)
		if !late && (err != nil || !found) {
			r.mtx.Unlock()
			r.t.Fatalf("read of dead block before %s = %v, %v", runSteps[i], found, err)
		}
		if late {
			a.lateReads = append(a.lateReads, ref)
		} else {
			a.reads = append(a.reads, ref)
		}
	}
	a.acts[kind]++
	r.mtx.Unlock()
	r.write()
}

// put puts ref's data again as the write-back store does: it logs the put
// it accepts, then its drain uploads the block unless the remote holds it.
func (a *adversary) put(ref *block.BlockRef) error {
	r := a.r
	r.backend.wlog.Put(ref.GetHash())
	held, err := r.backend.marking.GetBlockExistsBatch(r.ctx, []*block.BlockRef{ref})
	if err != nil || held[0] {
		return err
	}
	_, err = r.backend.marking.PutBlockBatch(r.ctx, []*block.PutBatchEntry{{Ref: ref, Data: r.data[ref.MarshalString()], RefsKnown: true}})
	return err
}

// fill reads ref as the write-back store fills its cache from the remote: in
// a read operation, logging the fill once the read found it.
func (a *adversary) fill(ref *block.BlockRef) (bool, error) {
	r := a.r
	ops, release, err := r.backend.marking.BeginReadOperation(r.ctx)
	if err != nil {
		return false, err
	}
	defer release()
	_, found, err := ops.GetBlock(r.ctx, ref)
	if found && err == nil {
		r.backend.wlog.Fill(ref.GetHash())
	}
	return found, err
}

// settle checks the cycle's reads, restores the removed packs when a late
// block is gone, checks that every late block reads, and links the late
// links as the others.
func (a *adversary) settle() {
	// The blocks read before the trash read.
	r := a.r
	r.t.Helper()
	for _, ref := range a.reads {
		if _, found, err := r.store.GetBlock(r.ctx, ref); err != nil || !found {
			r.t.Fatalf("read of block read before the trash %s = %v, %v", ref.MarshalString(), found, err)
		}
	}

	// Restore the removed packs when a late block is gone.
	late := slices.Concat(r.late, a.lateReads)
	if gone := a.gone(late); gone != 0 {
		a.lost += gone
		r.args.Packs = *cli.NewStringSlice(a.removed()...)
		r.mustRun("restore")
		r.args.Packs = *cli.NewStringSlice()
		if gone := a.gone(late); gone != 0 {
			r.t.Fatalf("%d late blocks gone after the restore", gone)
		}
	}

	// Link the late links as the others.
	r.mtx.Lock()
	r.linked = append(r.linked, r.late...)
	r.late = nil
	r.mtx.Unlock()
	a.reads, a.lateReads = nil, nil
}

// gone counts the blocks of refs that do not read: missing, or failing as
// held in the trash.
func (a *adversary) gone(refs []*block.BlockRef) int {
	r := a.r
	n := 0
	for _, ref := range refs {
		if _, found, err := r.store.GetBlock(r.ctx, ref); err != nil || !found {
			n++
		}
	}
	return n
}

// removed reads the ids of the packs the sweep moved to the trash or removed.
func (a *adversary) removed() []string {
	r := a.r
	db, store, err := r.args.openScratch()
	if err != nil {
		r.t.Fatal(err)
	}
	defer db.Close()
	var ids []string
	err = packsweep.PackRecords(r.ctx, store, func(p *packsweep.PackRecord) error {
		if p.Stage >= packsweep.StageTrash {
			ids = append(ids, p.ID)
		}
		return nil
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return ids
}
