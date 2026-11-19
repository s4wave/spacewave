package block_gc

import (
	"context"
	"math"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// AdoptStore is the Volume an audit runs in.
type AdoptStore interface {
	Store
	// BlockStored reports whether the Volume holds the bytes of ref.
	BlockStored(ctx context.Context, ref *block.BlockRef) (bool, error)
	// ScanBlocks calls fn with runs of the blocks whose bytes the Volume
	// holds, in key order, each run read in a snapshot of its own.
	ScanBlocks(ctx context.Context, fn func(refs []*block.BlockRef) error) error
}

// AdoptResult is what an adoption counted.
type AdoptResult struct {
	// Already reports that the Volume was adopted, so nothing was counted.
	Already bool
	// Blocks counts the blocks stored when the adoption began.
	Blocks uint64
	// Roots counts the named roots when the adoption began.
	Roots uint64
	// Counted counts the stored blocks given a count record: those with more
	// than one holder.
	Counted uint64
	// Queued counts the stored blocks no holder names, queued at zero, so
	// the collector frees them once the Volume is adopted.
	Queued uint64
	// Unknown counts the holders naming a block whose bytes the Volume stores
	// without a child list. Any refuses the adoption.
	Unknown uint64
	// UnknownSample names up to sampleSize of the blocks.
	UnknownSample []*block.BlockRef
}

// ErrAdoptUnknown is returned by Adopt when a root or a stored block names a
// block whose bytes the Volume stores without a child list: the children that
// block names would hold nothing, so its counts cannot be known. Write the
// missing child lists, then adopt again.
var ErrAdoptUnknown = errors.New("named blocks are stored without a child list")

// ErrAdoptSuperseded is returned by Adopt when another adoption, in this
// process or another on the same store, or a Volume opening, took the Volume
// from it. It writes nothing more and leaves the other run's records alone.
var ErrAdoptSuperseded = errors.New("another adoption superseded this one")

// Adopt counts the holders of every block of a Volume whose counts nobody
// trusts, from its child lists and named roots alone, and marks it adopted,
// after which the collector nets the logs and may free. It follows the
// protocol Adopt.tla checks.
//
// A claim takes the unadopted Volume under the run's name and clears every
// record the counts derive from, the logs included, while Land keeps no logs
// on an unadopted Volume. The mark then marks it adopting with the up cursor
// at the up log's end and the time stamped, so every Land after it logs its
// holders and the count needs only the Volume as it was at the mark: the
// holders of a block are the roots and the stored blocks whose child lists
// name it. It streams: it scans the roots and the child lists in key order,
// sliceRecords per transaction, yielding to waiting writers between them,
// and keeps only its scan position and one slice in memory, writing each
// holder into the block's count record as it finds it. While it runs, Land
// marks each block it gives birth to ('B') and records the first block each
// root it moves held at the mark ('A'), so a later slice skips a child list
// born after the mark and counts a moved root at its block at the mark; the
// logs past the mark carry those changes. A stored block then keeps holders
// minus one, one being implied, a stored block with none goes on the free
// queue at the up log's end, and a named block the Volume does not store
// keeps all its holders, since the put that stores it brings the holder its
// record implies.
//
// Invariant: no free runs until Grace has passed since the mark
// (Collector.freeable). A root moved off a block before the mark logged no
// down, which would have made the block wait Grace; a reader that saw the
// block may still be about to pin it. The settle's flip waits the same way,
// since that reader's sight is no seed of its walk.
//
// It refuses with ErrAdoptUnknown, returning the Volume to unadopted, when a
// root or a stored block names a block stored without a child list. It counts
// child lists an older store format left unverified as they are: such a list
// may lack a child, which then counts too few holders, and the settle
// generation keeps those blocks, as Collector describes. An adoption that
// fails or is interrupted returns the Volume to unadopted, as does reopening
// the Volume after a crash, and a later Adopt starts over and reaches the
// same counts. Each run first claims the unadopted Volume under its name,
// clears only while its claim stands, and marks it adopting only from its own
// claim; every later transaction refuses to write unless the Volume is still
// adopting in its run. A run another claimed or superseded writes nothing
// more and returns ErrAdoptSuperseded.
//
// Adoption counts only child lists and named roots. A block that older code
// held another way, such as through a bucket's ownership, has no holder here,
// so adopt a Volume only once every writer that uses it holds what it needs
// under named roots.
func Adopt(ctx context.Context, store Store, now func() time.Time) (AdoptResult, error) {
	// Report an adopted Volume, after deleting what a finished adoption may
	// have left; otherwise claim it unadopted for a new run.
	var res AdoptResult
	run := ulid.NewULID()
	adopted, err := claimAdoption(ctx, store, run)
	if err != nil {
		return res, err
	}
	if adopted {
		res.Already = true
		return res, clearKinds(ctx, store, nil, kindAdoptBorn, kindAdoptRoot)
	}

	// Clear what an earlier run and the logs left while the claim stands.
	if err := clearRun(ctx, store, run); err != nil {
		return res, err
	}

	// Mark the Volume adopting in the run, starting fresh logs.
	err = store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Refuse a lost claim.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if !claimedBy(state, run) {
			return ErrAdoptSuperseded
		}
		state.Adoption = Adoption_ADOPTION_ADOPTING
		state.AdoptionRun = run
		state.UpCursor = state.GetUpSeq()
		state.AdoptAtUnixMilli = now().UnixMilli()
		return rec.SetState(ctx, state)
	})
	if err != nil {
		return res, err
	}

	// Count the holders, refuse a block stored without a child list, and
	// settle the stored blocks' counts.
	a := &adoption{store: store, run: run, res: &res}
	if err := a.count(ctx); err != nil {
		return res, abandonAdopt(ctx, store, run, err)
	}

	// Mark the Volume adopted while the run still holds it.
	err = store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Read the state, refusing it once another run replaced this one.
		state, err := inRun(ctx, rec, run)
		if err != nil {
			return err
		}

		// Cap the pop mark clearRun raised at the up log's end. The recount
		// gave each block it left at zero the up log's end when it settled
		// that block as its ripe sequence, no later than the end now, so the
		// capped mark still covers each, while a block that dies later
		// cannot hold the flip. The recount returned each unverified child
		// of a list written from content to the front, and only a pop at the
		// child's later ripe sequence takes it off, which raises the mark
		// past its parent again.
		if err := rec.capPopMark(ctx, state.GetUpSeq()); err != nil {
			return err
		}
		state.Adoption = Adoption_ADOPTION_ADOPTED
		return rec.SetState(ctx, state)
	})
	if err != nil {
		return res, abandonAdopt(ctx, store, run, err)
	}

	// Delete the run's marks.
	return res, clearKinds(ctx, store, nil, kindAdoptBorn, kindAdoptRoot)
}

// inRun returns the state of a Volume still adopting in run, or
// ErrAdoptSuperseded.
func inRun(ctx context.Context, rec *Records, run string) (*LivenessState, error) {
	state, err := rec.GetState(ctx)
	if err != nil {
		return nil, err
	}
	if state.GetAdoption() != Adoption_ADOPTION_ADOPTING || state.GetAdoptionRun() != run {
		return nil, ErrAdoptSuperseded
	}
	return state, nil
}

// ResetAdoption returns a Volume an adoption left adopting to unadopted, so
// it logs nothing more. A Volume calls it when it opens: the adoption
// that marked it ended with the process.
func ResetAdoption(ctx context.Context, store Store) error {
	_, err := claimAdoption(ctx, store, "")
	return err
}

// SetFrees records whether the collector's frees are on, as a writable open
// does, and returns the state it leaves. Turning them on is the switch: the
// same write returns the Volume to unadopted, adopted or not, ending any
// adoption or release in progress, so the next adoption counts every holder
// again before the first free. That recount gives each named block without
// a list one holder more than its namers, a hole no audit recorded included,
// so the hole's next put leaves it held. Turning them off writes only the
// field: the collector then frees nothing, and the next switch-on counts
// again over whatever was written meanwhile. A stored switch whose frees
// mark is stale reads as off (Records.GetState), so a write by a build that
// knew nothing of the switch also makes the next switch-on count again.
func SetFrees(ctx context.Context, store Store, frees bool) (*LivenessState, error) {
	var state *LivenessState
	err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Read the state; a switch already as asked writes nothing.
		var err error
		state, err = rec.GetState(ctx)
		if err != nil || state.GetFrees() == frees {
			return err
		}

		// Record the switch, and on switching on return to unadopted.
		state.Frees = frees
		if frees {
			state.Adoption = Adoption_ADOPTION_UNADOPTED
			state.AdoptionRun = ""
			state.Release = nil
		}
		return rec.SetState(ctx, state)
	})
	return state, err
}

// claimAdoption returns a Volume that is not adopted to unadopted under run,
// superseding any run that marked or claimed it, and reports whether it left
// the Volume adopted. An empty run claims it for none.
func claimAdoption(ctx context.Context, store Store, run string) (bool, error) {
	var adopted bool
	err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Leave an adopted Volume, and one already in the claim.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if adopted = state.GetAdoption() == Adoption_ADOPTION_ADOPTED; adopted {
			return nil
		}
		if state.GetAdoption() == Adoption_ADOPTION_UNADOPTED && state.GetAdoptionRun() == run {
			return nil
		}

		// Claim it, ending any release in progress: the run recounts every
		// holder, and the claim's clear deletes the release pages.
		state.Adoption = Adoption_ADOPTION_UNADOPTED
		state.AdoptionRun = run
		state.Release = nil
		return rec.SetState(ctx, state)
	})
	return adopted, err
}

// abandonAdopt returns the Volume to unadopted after run failed or was
// interrupted, deletes the counts, queue and marks it wrote
// after a failure, and returns cause. An interrupted run only returns the
// Volume to unadopted, in one transaction, so a Close that cancels a large
// adoption does not wait for its cleanup: nothing reads those records while
// the Volume is unadopted, and the next run's claim clears them. It touches
// nothing once another run holds the Volume: a run superseded it, and the
// records are that run's.
func abandonAdopt(ctx context.Context, store Store, run string, cause error) error {
	// Mark the Volume unadopted, abandoned by run, while run still holds it,
	// even after ctx ended.
	interrupted := ctx.Err() != nil
	ctx = context.WithoutCancel(ctx)
	var ours bool
	err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Leave a Volume another run holds.
		state, err := inRun(ctx, rec, run)
		if errors.Is(err, ErrAdoptSuperseded) {
			return nil
		}
		if err != nil {
			return err
		}
		ours = true
		state.Adoption = Adoption_ADOPTION_UNADOPTED
		return rec.SetState(ctx, state)
	})

	// Delete what a failed run wrote, stopping once another run starts.
	if err == nil && ours && !interrupted {
		err = clearRun(ctx, store, run)
	}
	if err != nil {
		return errors.Wrapf(cause, "and returning the volume to unadopted failed: %v", err)
	}
	return cause
}

// adoption is the state of one Adopt call.
type adoption struct {
	// store is the Volume.
	store Store
	// run names the adoption's run.
	run string
	// res is the result.
	res *AdoptResult
}

// count counts every holder into the count records, refuses a block stored
// without a child list, then settles each stored block's count.
func (a *adoption) count(ctx context.Context) error {
	// Count each root at the block it held when the adoption began.
	if err := a.countRoots(ctx); err != nil {
		return err
	}

	// Count the children of each block stored when the adoption began.
	err := forSlices(ctx, a.store, a.run, kindChildren, func(rec *Records, blocks block.StoreOps, r adoptRecord) error {
		// Skip a block born since the adoption began.
		ref, born, err := a.listed(ctx, rec, r)
		if err != nil || born {
			return err
		}

		// Count a holder of each child. While Open a list written from
		// content returns each unverified child to the settle's front: the
		// free proof may have popped the child while a short list left it
		// Dead, and once this count holds it again the walk must verify it
		// before the flip, or the sweep deletes it.
		children, err := DecodeChildren(r.value)
		if err != nil {
			return errors.Wrapf(err, "child list of %s", ref.MarshalString())
		}
		returns := listMarked(r.value)
		if returns {
			gen, err := rec.generation(ctx)
			if err != nil {
				return err
			}
			returns = gen == Generation_GENERATION_OPEN
		}
		for _, child := range children {
			if err := a.hold(ctx, rec, blocks, BlockKey(child), returns); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Refuse when a holder names a block stored without a child list.
	if a.res.Unknown != 0 {
		return errors.Wrapf(ErrAdoptUnknown, "%d holders, sample %v", a.res.Unknown, a.res.UnknownSample)
	}

	// Settle each stored block: one holder is implied, and a block with none
	// is at zero with children that count it, on the free queue, so the free
	// releases them.
	return forSlices(ctx, a.store, a.run, kindChildren, func(rec *Records, _ block.StoreOps, r adoptRecord) error {
		// Skip a block born since the adoption began.
		ref, born, err := a.listed(ctx, rec, r)
		if err != nil || born {
			return err
		}

		// Write the holders of a block a holder names.
		a.res.Blocks++
		n, err := rec.tally(ctx, ref)
		if err != nil {
			return err
		}
		if n != 0 {
			if n > 1 {
				a.res.Counted++
			}
			return rec.SetHolders(ctx, ref, n, 0)
		}

		// Settle a block no holder names at zero, ripe at the up log's end.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		a.res.Queued++
		ripe := state.GetUpSeq()
		if err := rec.SetHolders(ctx, ref, 0, ripe); err != nil {
			return err
		}
		return rec.tx.Set(ctx, rec.queueKey(ripe, BlockKey(ref)), nil)
	})
}

// countRoots counts each name that held a root when the adoption began at
// the block it held then. It scans the root table and the moved roots' marks
// as one sequence of names in key order, sliceRecords names per transaction,
// so it reads each name once: a name a Land moved before the scan reached it
// has a mark holding its starting block, and one moved after had none to
// read, so the scan read its starting block from the table.
func (a *adoption) countRoots(ctx context.Context) error {
	_, err := eachSlice(ctx, a.store.Update, []byte(nil), nil, func(rec *Records, blocks block.StoreOps, after []byte) ([]byte, bool, error) {
		// Read the next names of either kind while the run holds the Volume.
		if _, err := inRun(ctx, rec, a.run); err != nil {
			return nil, false, err
		}
		names, err := rec.scanNamesAfter(ctx, after, sliceRecords, kindRoot, kindAdoptRoot)
		if err != nil {
			return nil, false, err
		}
		for _, name := range names {
			// Read the starting block from the mark, or else the table.
			start, moved, err := rec.tx.Get(ctx, rec.key(kindAdoptRoot, name))
			if err != nil {
				return nil, false, err
			}
			if !moved {
				value, _, err := rec.tx.Get(ctx, rec.key(kindRoot, name))
				if err != nil {
					return nil, false, err
				}
				entry := &RootEntry{}
				if err := entry.UnmarshalVT(value); err != nil {
					return nil, false, err
				}
				start = entry.GetRef()
			}

			// Count the holder of a name that held a block.
			if len(start) != 0 {
				a.res.Roots++
				if err := a.hold(ctx, rec, blocks, start, false); err != nil {
					return nil, false, err
				}
			}
			after = name
		}
		return after, len(names) == sliceRecords, nil
	})
	return err
}

// listed returns the block of a child list record and whether Land gave
// birth to it since the mark, so its holders come from the logs and the
// adoption neither counts its children nor settles it.
func (a *adoption) listed(ctx context.Context, rec *Records, r adoptRecord) (*block.BlockRef, bool, error) {
	ref, rest, err := ParseBlockKey(r.key)
	if err != nil || len(rest) != 0 {
		return nil, false, errBadBlockKey
	}
	born, err := rec.tx.Exists(ctx, rec.key(kindAdoptBorn, BlockKey(ref)))
	return ref, born, err
}

// hold counts one holder of the block key names, and notes the holder as
// unknown when the Volume stores the block's bytes without a child list.
// While open, a holder from a list written from content returns the block to
// the front through returnHeld.
func (a *adoption) hold(ctx context.Context, rec *Records, blocks block.StoreOps, key []byte, open bool) error {
	// Count the holder.
	ref, rest, err := ParseBlockKey(key)
	if err != nil || len(rest) != 0 {
		return errBadBlockKey
	}
	n, err := rec.tally(ctx, ref)
	if err != nil {
		return err
	}
	if err := rec.setTally(ctx, ref, n+1); err != nil {
		return err
	}

	// Return an unverified block to the front, and note one whose bytes are
	// stored without a child list.
	listed, _, err := rec.returnHeld(ctx, key, open)
	if err != nil || listed {
		return err
	}
	found, err := blocks.GetBlockExists(ctx, ref)
	if err != nil || !found {
		return err
	}
	a.res.Unknown++
	a.res.UnknownSample = sample(a.res.UnknownSample, ref)
	return nil
}

// forSlices calls fn with every record of kind in key order, sliceRecords
// per transaction while run holds the Volume, yielding to waiting writers
// between transactions.
func forSlices(ctx context.Context, store Store, run string, kind byte, fn func(rec *Records, blocks block.StoreOps, r adoptRecord) error) error {
	_, err := eachSlice(ctx, store.Update, []byte(nil), nil, func(rec *Records, blocks block.StoreOps, after []byte) ([]byte, bool, error) {
		// Read the slice while the run holds the Volume, then handle each
		// record.
		if _, err := inRun(ctx, rec, run); err != nil {
			return nil, false, err
		}
		recs, err := rec.scanAfter(ctx, kind, after, sliceRecords)
		if err != nil {
			return nil, false, err
		}
		for _, r := range recs {
			if err := fn(rec, blocks, r); err != nil {
				return nil, false, err
			}
			after = r.key
		}
		return after, len(recs) == sliceRecords, nil
	})
	return err
}

// adoptionKinds are the kinds of record an adoption writes or replaces, which
// it clears before it counts the lists and after a failed run: the counts,
// free queue and parked records, the up and down logs, its marks, release
// pages, the record that a recount is due, and the counts the settle froze
// at the flip.
var adoptionKinds = []byte{
	kindCount, kindQueue, kindParked, kindUpLog, kindDownLog, kindAdoptBorn,
	kindAdoptRoot, kindReleasePage, kindRewritten, kindUnmarkedCount,
}

// claimedBy reports whether state is unadopted under run's claim.
func claimedBy(state *LivenessState, run string) bool {
	return state.GetAdoption() == Adoption_ADOPTION_UNADOPTED && state.GetAdoptionRun() == run
}

// clearRun takes each parked block out of the run, then deletes every
// record of the adoptionKinds, stopping, deleting nothing more, once the
// Volume leaves run's claim. Land writes none of them while the Volume is
// unadopted, so the clear ends with none left. A parked block is in the run, as Records.park describes, and the recount may
// find it held, so it leaves the run with its parked record; if it is Dead
// again, the free parks it again.
func clearRun(ctx context.Context, store Store, run string) error {
	// Take the parked blocks out of the run while claimed. The recount gives
	// a block Dead before it a new ripe sequence, so a pop mark covers every
	// queued block until Adopt caps it once the recount is done.
	claimed := func(state *LivenessState) bool { return claimedBy(state, run) }
	err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		state, err := rec.GetState(ctx)
		if err != nil || !claimed(state) {
			return err
		}
		if _, popped, err := rec.popMark(ctx); err != nil || !popped {
			return err
		}
		return rec.raisePopMark(ctx, math.MaxUint64)
	})
	if err != nil {
		return err
	}
	_, err = eachSlice(ctx, store.Update, []byte(nil), nil, func(rec *Records, _ block.StoreOps, after []byte) ([]byte, bool, error) {
		state, err := rec.GetState(ctx)
		if err != nil || !claimed(state) {
			return nil, false, err
		}
		return rec.unrunParked(ctx, after)
	})
	if err != nil {
		return err
	}

	// Clear the adoption's records.
	return clearKinds(ctx, store, claimed, adoptionKinds...)
}

// clearKinds deletes every record of the kinds, sliceRecords per
// transaction, yielding to waiting writers between transactions. With keep
// set it stops, deleting nothing more, at the first transaction whose state
// keep refuses.
func clearKinds(ctx context.Context, store Store, keep func(*LivenessState) bool, kinds ...byte) error {
	for _, kind := range kinds {
		var stop bool
		_, err := eachSlice(ctx, store.Update, []byte(nil), nil, func(rec *Records, _ block.StoreOps, after []byte) ([]byte, bool, error) {
			// Stop at a state keep refuses, then delete the records of the
			// kind from the last key the slice before reached.
			stop = false
			if keep != nil {
				state, err := rec.GetState(ctx)
				if err != nil {
					return nil, false, err
				}
				if stop = !keep(state); stop {
					return nil, false, nil
				}
			}
			recs, err := rec.scanAfter(ctx, kind, after, sliceRecords)
			if err != nil {
				return nil, false, err
			}
			for _, r := range recs {
				if err := rec.tx.Delete(ctx, rec.key(kind, r.key)); err != nil {
					return nil, false, err
				}
				after = r.key
			}
			return after, len(recs) == sliceRecords, nil
		})
		if err != nil || stop {
			return err
		}
	}
	return nil
}
