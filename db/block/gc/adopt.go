package block_gc

import (
	"context"

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
	// Queued counts the stored blocks no holder names, put on the free queue.
	Queued uint64
	// Unknown counts the holders naming a block whose bytes the Volume stores
	// without a child list. Any refuses the adoption.
	Unknown uint64
	// UnknownSample names up to unknownSampleSize of the blocks.
	UnknownSample []*block.BlockRef
}

// ErrAdoptUnknown is returned by Adopt when a root or a stored block names a
// block whose bytes the Volume stores without a child list: the children that
// block names would hold nothing, so its counts cannot be known. Write the
// missing child lists, then adopt again.
var ErrAdoptUnknown = errors.New("named blocks are stored without a child list")

// ErrAdoptUnverified is returned by Adopt when the Volume's child lists came
// from a store format before 7, whose lists may lack a child the block
// names: counting from them could free a block a live block names. Rebuild
// the lists from the blocks, then adopt.
var ErrAdoptUnverified = errors.New("the child lists are unverified")

// ErrAdoptSuperseded is returned by Adopt when another adoption, in this
// process or another on the same store, or a Volume opening, took the Volume
// from it. It writes nothing more and leaves the other run's records alone.
var ErrAdoptSuperseded = errors.New("another adoption superseded this one")

// Adopt counts the holders of every block of a Volume that predates its
// journal, from its child lists and named roots alone, and marks it adopted,
// after which the collector nets the journal and may free.
//
// It marks the Volume adopting at a journal position, the cursor, so every
// write after that point journals its holders, and counts the Volume as it
// was at that point: the holders of a block are the roots and the stored
// blocks whose child lists name it. It streams: it scans the roots and the
// child lists in key order, sliceRecords per transaction, yielding to waiting
// writers between them, and keeps only its scan position and one slice in
// memory, writing each holder into the block's count record as it finds it.
// While it runs, Land marks each block it gives birth to and records the
// first block each root it moves held, so a later slice skips a child list
// born after the cursor and counts a moved root at its starting block; the
// journal past the cursor carries those changes. A stored block then keeps
// holders minus one, one being implied, a stored block with none goes on the
// free queue, and a named block the Volume does not store keeps all its
// holders, since the put that stores it brings the holder its record implies.
// It drops the journal before the cursor and marks the Volume adopted.
//
// It refuses with ErrAdoptUnknown, returning the Volume to unadopted, when a
// root or a stored block names a block stored without a child list, and with
// ErrAdoptUnverified, before it begins, while the child lists come from an
// older store format. An adoption that fails or is interrupted returns the
// Volume to unadopted, as does reopening the Volume after a crash, and a later
// Adopt starts over and reaches the same counts. Each run first claims the
// unadopted Volume under its name, clears only while its claim stands, and
// marks it adopting only from its own claim; every later transaction refuses
// to write unless the Volume is still adopting in its run. A run another
// claimed or superseded writes nothing more and returns ErrAdoptSuperseded.
//
// Adoption counts only child lists and named roots. A block that older code
// held another way, such as through a bucket's ownership, has no holder here,
// so adopt a Volume only once every writer that uses it holds what it needs
// under named roots.
func Adopt(ctx context.Context, store Store) (AdoptResult, error) {
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

	// Clear what an earlier run left while the claim stands.
	claimed := func(state *LivenessState) bool {
		return state.GetAdoption() == Adoption_ADOPTION_UNADOPTED && state.GetAdoptionRun() == run
	}
	if err := clearKinds(ctx, store, claimed, kindCount, kindQueue, kindAdoptBorn, kindAdoptRoot); err != nil {
		return res, err
	}

	// Mark the Volume adopting in the run, at the cursor.
	var cursor uint64
	err = store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Refuse a lost claim and unverified child lists.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if !claimed(state) {
			return ErrAdoptSuperseded
		}
		if state.GetUnverifiedLists() {
			return ErrAdoptUnverified
		}

		// Start the cursor at the next journal record.
		cursor = state.GetNextSeq()
		state.Adoption = Adoption_ADOPTION_ADOPTING
		state.AdoptionRun = run
		state.Cursor, state.NetOffset = cursor, 0
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

	// Drop the journal records the counts reflect, then mark the Volume
	// adopted while the run still holds it.
	if err := deleteJournalBefore(ctx, store, run, cursor); err != nil {
		return res, abandonAdopt(ctx, store, run, err)
	}
	err = store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Mark it adopted at the cursor.
		state, err := inRun(ctx, rec, run)
		if err != nil {
			return err
		}
		state.Adoption = Adoption_ADOPTION_ADOPTED
		state.Cursor = cursor
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
// it journals nothing more. A Volume calls it when it opens: the adoption
// that marked it ended with the process.
func ResetAdoption(ctx context.Context, store Store) error {
	_, err := resetAdoption(ctx, store)
	return err
}

// resetAdoption returns an adopting Volume to unadopted, superseding the run
// that marked it, and reports whether the Volume is adopted.
func resetAdoption(ctx context.Context, store Store) (bool, error) {
	return claimAdoption(ctx, store, "")
}

// claimAdoption returns a Volume that is not adopted to unadopted under run,
// superseding any run that marked or claimed it, and reports whether the
// Volume is adopted. An empty run claims it for none.
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

		// Claim it.
		state.Adoption = Adoption_ADOPTION_UNADOPTED
		state.AdoptionRun = run
		return rec.SetState(ctx, state)
	})
	return adopted, err
}

// abandonAdopt returns the Volume to unadopted after run failed or was
// interrupted, deletes the counts, queue and marks it wrote, and returns
// cause. It runs after ctx ends, so an interrupted adoption still cleans up.
// It touches nothing once another run holds the Volume: a run superseded it,
// and the records are that run's.
func abandonAdopt(ctx context.Context, store Store, run string, cause error) error {
	// Mark the Volume unadopted, abandoned by run, while run still holds it.
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

	// Delete what it wrote, stopping once another run starts.
	if err == nil && ours {
		abandoned := func(state *LivenessState) bool {
			return state.GetAdoption() == Adoption_ADOPTION_UNADOPTED && state.GetAdoptionRun() == run
		}
		err = clearKinds(ctx, store, abandoned, kindCount, kindQueue, kindAdoptBorn, kindAdoptRoot)
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

		// Count a holder of each child, which also gives a list a cache fill
		// wrote its children's holders, so its fill mark goes.
		children, err := DecodeChildren(r.value)
		if err != nil {
			return errors.Wrapf(err, "child list of %s", ref.MarshalString())
		}
		if _, err := rec.takeFillMark(ctx, ref); err != nil {
			return err
		}
		for _, child := range children {
			if err := a.hold(ctx, rec, blocks, BlockKey(child)); err != nil {
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
	// goes on the free queue.
	return forSlices(ctx, a.store, a.run, kindChildren, func(rec *Records, _ block.StoreOps, r adoptRecord) error {
		// Skip a block born since the adoption began.
		ref, born, err := a.listed(ctx, rec, r)
		if err != nil || born {
			return err
		}

		// Queue a block no holder names, and drop the implied holder of any
		// other.
		a.res.Blocks++
		n, err := rec.GetCount(ctx, ref)
		if err != nil {
			return err
		}
		if n == 0 {
			a.res.Queued++
			return rec.Enqueue(ctx, ref)
		}
		if n > 1 {
			a.res.Counted++
		}
		return rec.SetCount(ctx, ref, n-1)
	})
}

// countRoots counts each name that held a root when the adoption began at
// the block it held then. It scans the root table and the moved roots' marks
// as one sequence of names in key order, sliceRecords names per transaction,
// so it reads each name once: a name a Land moved before the scan reached it
// has a mark holding its starting block, and one moved after had none to
// read, so the scan read its starting block from the table.
func (a *adoption) countRoots(ctx context.Context) error {
	var after []byte
	for {
		// Read the next names of either kind in one transaction.
		var last []byte
		var n int
		err := a.store.Update(ctx, func(rec *Records, blocks block.StoreOps) error {
			// Read the names while the run holds the Volume.
			if _, err := inRun(ctx, rec, a.run); err != nil {
				return err
			}
			names, err := rec.scanNamesAfter(ctx, after, sliceRecords, kindRoot, kindAdoptRoot)
			if err != nil {
				return err
			}
			n = len(names)
			for _, name := range names {
				// Read the starting block from the mark, or else the table.
				start, moved, err := rec.tx.Get(ctx, rec.key(kindAdoptRoot, name))
				if err != nil {
					return err
				}
				if !moved {
					value, found, err := rec.tx.Get(ctx, rec.key(kindRoot, name))
					if err != nil || !found {
						return err
					}
					entry := &RootEntry{}
					if err := entry.UnmarshalVT(value); err != nil {
						return err
					}
					start = entry.GetRef()
				}

				// Count the holder of a name that held a block.
				if len(start) != 0 {
					a.res.Roots++
					if err := a.hold(ctx, rec, blocks, start); err != nil {
						return err
					}
				}
				last = name
			}
			return nil
		})
		if err != nil || n < sliceRecords {
			return err
		}

		// Move past them and let a waiting writer in.
		after = last
		if err := context.Cause(ctx); err != nil {
			return err
		}
	}
}

// listed returns the block of a child list record and whether Land gave
// birth to it since the adoption began.
func (a *adoption) listed(ctx context.Context, rec *Records, r adoptRecord) (*block.BlockRef, bool, error) {
	ref, rest, err := ParseBlockKey(r.key)
	if err != nil || len(rest) != 0 {
		return nil, false, errBadBlockKey
	}
	born, err := rec.AdoptBorn(ctx, ref)
	return ref, born, err
}

// hold counts one holder of the block key names, and notes the holder as
// unknown when the Volume stores the block's bytes without a child list.
func (a *adoption) hold(ctx context.Context, rec *Records, blocks block.StoreOps, key []byte) error {
	// Count the holder.
	ref, rest, err := ParseBlockKey(key)
	if err != nil || len(rest) != 0 {
		return errBadBlockKey
	}
	n, err := rec.GetCount(ctx, ref)
	if err != nil {
		return err
	}
	if err := rec.SetCount(ctx, ref, n+1); err != nil {
		return err
	}

	// Note a block whose bytes are stored without a child list.
	listed, err := rec.HasChildren(ctx, ref)
	if err != nil || listed {
		return err
	}
	found, err := blocks.GetBlockExists(ctx, ref)
	if err != nil || !found {
		return err
	}
	a.res.Unknown++
	if len(a.res.UnknownSample) < unknownSampleSize {
		a.res.UnknownSample = append(a.res.UnknownSample, ref)
	}
	return nil
}

// forSlices calls fn with every record of kind in key order, sliceRecords
// per transaction while run holds the Volume, yielding to waiting writers
// between transactions.
func forSlices(ctx context.Context, store Store, run string, kind byte, fn func(rec *Records, blocks block.StoreOps, r adoptRecord) error) error {
	var after []byte
	for {
		// Read and handle the next slice in one transaction.
		var last []byte
		var n int
		err := store.Update(ctx, func(rec *Records, blocks block.StoreOps) error {
			// Read the slice while the run holds the Volume, then handle each
			// record.
			if _, err := inRun(ctx, rec, run); err != nil {
				return err
			}
			recs, err := rec.scanAfter(ctx, kind, after, sliceRecords)
			if err != nil {
				return err
			}
			n = len(recs)
			for _, r := range recs {
				if err := fn(rec, blocks, r); err != nil {
					return err
				}
				last = r.key
			}
			return nil
		})
		if err != nil || n < sliceRecords {
			return err
		}

		// Move past it and let a waiting writer in.
		after = last
		if err := context.Cause(ctx); err != nil {
			return err
		}
	}
}

// deleteJournalBefore deletes the journal records before cursor, which the
// adoption's counts reflect, sliceRecords per transaction while run holds the
// Volume.
func deleteJournalBefore(ctx context.Context, store Store, run string, cursor uint64) error {
	var from uint64
	for {
		// Delete the next records before the cursor.
		var n int
		err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
			// Read the sequence numbers while the run holds the Volume, then
			// delete their records.
			if _, err := inRun(ctx, rec, run); err != nil {
				return err
			}
			var seqs []uint64
			err := rec.ScanJournal(ctx, from, func(seq uint64, _ *JournalRecord) (bool, error) {
				if seq >= cursor || len(seqs) == sliceRecords {
					return false, nil
				}
				seqs = append(seqs, seq)
				return true, nil
			})
			if err != nil {
				return err
			}
			n = len(seqs)
			if n != 0 {
				from = seqs[n-1] + 1
			}
			for _, seq := range seqs {
				if err := rec.DeleteJournal(ctx, seq); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil || n < sliceRecords {
			return err
		}

		// Let a waiting writer in.
		if err := context.Cause(ctx); err != nil {
			return err
		}
	}
}

// clearKinds deletes every record of the kinds, sliceRecords per
// transaction, yielding to waiting writers between transactions. With keep
// set it stops, deleting nothing more, at the first transaction whose state
// keep refuses.
func clearKinds(ctx context.Context, store Store, keep func(*LivenessState) bool, kinds ...byte) error {
	for _, kind := range kinds {
		var after []byte
		for {
			// Delete the next records of the kind while keep accepts the
			// state, from the last key the slice before reached.
			var n int
			var stop bool
			err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
				// Stop at a state keep refuses, then delete the records.
				if keep != nil {
					state, err := rec.GetState(ctx)
					if err != nil {
						return err
					}
					if stop = !keep(state); stop {
						return nil
					}
				}
				recs, err := rec.scanAfter(ctx, kind, after, sliceRecords)
				if err != nil {
					return err
				}
				n = len(recs)
				for _, r := range recs {
					if err := rec.tx.Delete(ctx, rec.key(kind, r.key)); err != nil {
						return err
					}
				}
				if n != 0 {
					after = recs[n-1].key
				}
				return nil
			})
			if err != nil || stop {
				return err
			}
			if n < sliceRecords {
				break
			}

			// Let a waiting writer in.
			if err := context.Cause(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}
