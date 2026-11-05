package block_gc

import (
	"context"
	"maps"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// adoptBatch bounds the records one adoption transaction writes.
const adoptBatch = 4096

// AdoptStore is the Volume an adoption runs in.
type AdoptStore interface {
	Store
	// BlockStored reports whether the Volume holds the bytes of ref.
	BlockStored(ctx context.Context, ref *block.BlockRef) (bool, error)
}

// AdoptResult is what an adoption counted.
type AdoptResult struct {
	// Already reports that the Volume was adopted, so nothing was counted.
	Already bool
	// Blocks counts the stored blocks, which have a child list.
	Blocks uint64
	// Roots counts the named roots.
	Roots uint64
	// Counted counts the count records written.
	Counted uint64
	// Queued counts the stored blocks no holder names, put on the free queue.
	Queued uint64
	// Pending counts the blocks a holder names that the Volume does not store.
	Pending uint64
	// Unknown counts the named blocks whose bytes the Volume stores without a
	// child list. Any refuses the adoption.
	Unknown uint64
	// UnknownSample names up to unknownSampleSize of them.
	UnknownSample []*block.BlockRef
}

// ErrAdoptUnknown is returned by Adopt when a root or a stored block names a
// block whose bytes the Volume stores without a child list: the children that
// block names would hold nothing, so its counts cannot be known. Write the
// missing child lists, then adopt again.
var ErrAdoptUnknown = errors.New("named blocks are stored without a child list")

// Adopt counts the holders of every block of a Volume that predates its
// journal, from its child lists and named roots alone, and marks it adopted,
// after which the collector nets the journal and may free.
//
// It first marks the Volume adopting, so every write after that point
// journals its holders. It then reads one snapshot: the holders of a block
// are the roots and the stored blocks whose child lists name it, and the
// journal records the snapshot holds are reflected in it, so the adopted
// cursor starts after them and the collector nets only the later ones.
// Nothing but the adoption writes count records while it runs.
//
// A stored block with holders keeps holders minus one in its count record,
// one being implied; a stored block with none goes on the free queue; a
// named block the Volume does not store keeps all its holders, since the put
// that stores it brings the holder its record implies.
//
// The tally is one map entry per named block, so an adoption of a Volume of
// many blocks needs memory in proportion; it suits a copy host or a quiet
// server.
//
// Adoption counts only child lists and named roots. A block that older code
// held another way, such as through a bucket's ownership, has no holder here,
// so adopt a Volume only once every writer that uses it holds what it needs
// under named roots. Adopt refuses with ErrAdoptUnknown, writing no count and
// returning the Volume to unadopted, when a root or a stored block names a
// block stored without a child list.
func Adopt(ctx context.Context, store AdoptStore) (AdoptResult, error) {
	// Mark the Volume adopting, so later writes journal their holders.
	var res AdoptResult
	err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		switch state.GetAdoption() {
		case Adoption_ADOPTION_ADOPTED:
			res.Already = true
			return nil
		case Adoption_ADOPTION_UNADOPTED:
			state.Adoption = Adoption_ADOPTION_ADOPTING
			state.Cursor = state.GetNextSeq()
			return rec.SetState(ctx, state)
		}
		return nil
	})
	if err != nil || res.Already {
		return res, err
	}

	// Clear the counts and queue an adoption that stopped before it finished
	// left: an unadopted Volume keeps none.
	for _, kind := range []byte{kindCount, kindQueue} {
		if err := deletePrefix(ctx, store, kind); err != nil {
			return res, abandonAdopt(ctx, store, err)
		}
	}

	// Tally every holder in one snapshot.
	var cursor uint64
	var tally map[string]uint64
	err = store.View(ctx, func(rec *Records) error {
		// Start the cursor after the records the snapshot reflects.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		cursor = state.GetNextSeq()

		// Count the holders.
		tally, err = tallyHolders(ctx, rec, &res)
		return err
	})
	if err != nil {
		return res, abandonAdopt(ctx, store, err)
	}

	// Refuse when a named block is stored without a child list.
	if err := checkUnknown(ctx, store, tally, &res); err != nil {
		return res, abandonAdopt(ctx, store, err)
	}

	// Write the counts and the free queue from the tallies.
	if err := writeCounts(ctx, store, tally, &res); err != nil {
		return res, abandonAdopt(ctx, store, err)
	}

	// Drop the journal records the snapshot reflects and mark the Volume
	// adopted.
	if err := deleteJournalBefore(ctx, store, cursor); err != nil {
		return res, err
	}
	err = store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Read the state, then mark it adopted at the snapshot's cursor.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		state.Adoption = Adoption_ADOPTION_ADOPTED
		state.Cursor = cursor
		return rec.SetState(ctx, state)
	})
	return res, err
}

// abandonAdopt returns the Volume to unadopted after a failed adoption, so it
// journals nothing more, deletes the counts and queue it wrote, and returns
// cause.
func abandonAdopt(ctx context.Context, store AdoptStore, cause error) error {
	err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		state, err := rec.GetState(ctx)
		if err != nil || state.GetAdoption() != Adoption_ADOPTION_ADOPTING {
			return err
		}
		state.Adoption = Adoption_ADOPTION_UNADOPTED
		return rec.SetState(ctx, state)
	})
	for _, kind := range []byte{kindCount, kindQueue} {
		if err == nil {
			err = deletePrefix(ctx, store, kind)
		}
	}
	if err != nil {
		return errors.Wrapf(cause, "and returning the volume to unadopted failed: %v", err)
	}
	return cause
}

// tallyHolders counts, for each block, the roots and the stored blocks whose
// child lists name it, in rec, one snapshot.
func tallyHolders(ctx context.Context, rec *Records, res *AdoptResult) (map[string]uint64, error) {
	// Count the roots.
	tally := make(map[string]uint64)
	err := rec.ScanRoots(ctx, func(_, _ string, entry *RootEntry) error {
		if _, _, err := ParseBlockKey(entry.GetRef()); err != nil {
			return err
		}
		res.Roots++
		tally[string(entry.GetRef())]++
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Count the stored blocks' children.
	err = rec.ScanChildren(ctx, func(_ *block.BlockRef, children []*block.BlockRef) error {
		res.Blocks++
		for _, child := range children {
			tally[string(BlockKey(child))]++
		}
		return nil
	})
	return tally, err
}

// checkUnknown counts the tallied blocks without a child list whose bytes the
// Volume stores, and returns ErrAdoptUnknown when there are any.
func checkUnknown(ctx context.Context, store AdoptStore, tally map[string]uint64, res *AdoptResult) error {
	keys := slices.Sorted(maps.Keys(tally))
	for batch := range slices.Chunk(keys, adoptBatch) {
		err := store.View(ctx, func(rec *Records) error {
			for _, key := range batch {
				ref, _, err := ParseBlockKey([]byte(key))
				if err != nil {
					return err
				}
				listed, err := rec.HasChildren(ctx, ref)
				if err != nil || listed {
					if err != nil {
						return err
					}
					continue
				}
				found, err := store.BlockStored(ctx, ref)
				if err != nil {
					return err
				}
				if !found {
					continue
				}
				res.Unknown++
				if len(res.UnknownSample) < unknownSampleSize {
					res.UnknownSample = append(res.UnknownSample, ref)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if res.Unknown != 0 {
		return errors.Wrapf(ErrAdoptUnknown, "%d blocks, sample %v", res.Unknown, res.UnknownSample)
	}
	return nil
}

// writeCounts writes the count record of each tallied block and queues each
// stored block no holder names, a batch per transaction.
func writeCounts(ctx context.Context, store AdoptStore, tally map[string]uint64, res *AdoptResult) error {
	// Write the tallied blocks' counts.
	keys := slices.Sorted(maps.Keys(tally))
	for batch := range slices.Chunk(keys, adoptBatch) {
		err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
			for _, key := range batch {
				ref, _, err := ParseBlockKey([]byte(key))
				if err != nil {
					return err
				}
				listed, err := rec.HasChildren(ctx, ref)
				if err != nil {
					return err
				}
				count := tally[key]
				if listed {
					count--
				} else {
					res.Pending++
				}
				if count != 0 {
					res.Counted++
				}
				if err := rec.SetCount(ctx, ref, count); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	// Queue the stored blocks no holder names.
	var after []byte
	for {
		var n int
		err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
			// Find the next stored blocks without a tally.
			var unheld []*block.BlockRef
			err := rec.scanChildrenAfter(ctx, after, adoptBatch, func(ref *block.BlockRef) error {
				n++
				after = BlockKey(ref)
				if _, held := tally[string(after)]; !held {
					unheld = append(unheld, ref)
				}
				return nil
			})
			if err != nil {
				return err
			}

			// Queue them.
			for _, ref := range unheld {
				res.Queued++
				if err := rec.Enqueue(ctx, ref); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if n < adoptBatch {
			return nil
		}
	}
}

// deleteJournalBefore deletes the journal records before cursor, which the
// adoption's snapshot reflects.
func deleteJournalBefore(ctx context.Context, store AdoptStore, cursor uint64) error {
	for {
		var n int
		err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
			// Find the next records before the cursor.
			var seqs []uint64
			err := rec.ScanJournal(ctx, 0, func(seq uint64, _ *JournalRecord) (bool, error) {
				if seq >= cursor || len(seqs) == adoptBatch {
					return false, nil
				}
				seqs = append(seqs, seq)
				return true, nil
			})
			if err != nil {
				return err
			}

			// Delete them.
			n = len(seqs)
			for _, seq := range seqs {
				if err := rec.DeleteJournal(ctx, seq); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil || n < adoptBatch {
			return err
		}
	}
}

// deletePrefix deletes every record of kind, a batch per transaction.
func deletePrefix(ctx context.Context, store AdoptStore, kind byte) error {
	for {
		var n int
		err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
			// Collect the next keys of the kind.
			var keys [][]byte
			it := rec.tx.Iterate(ctx, rec.key(kind), true, false)
			for it.Next(); it.Valid() && len(keys) < adoptBatch; it.Next() {
				keys = append(keys, slices.Clone(it.Key()))
			}
			err := it.Err()
			it.Close()
			if err != nil {
				return err
			}

			// Delete them.
			n = len(keys)
			for _, key := range keys {
				if err := rec.tx.Delete(ctx, key); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil || n < adoptBatch {
			return err
		}
	}
}

// scanChildrenAfter calls fn with up to limit stored blocks after the block
// key after, in key order.
func (r *Records) scanChildrenAfter(ctx context.Context, after []byte, limit int, fn func(ref *block.BlockRef) error) error {
	// Position the sorted iterator past after.
	prefix := r.key(kindChildren)
	it := r.tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := it.Seek(append(r.key(kindChildren, after), 0)); err != nil {
		return err
	}

	// Collect the blocks, then call fn outside the iterator.
	var refs []*block.BlockRef
	for ; it.Valid() && len(refs) < limit; it.Next() {
		ref, rest, err := ParseBlockKey(it.Key()[len(prefix):])
		if err != nil || len(rest) != 0 {
			return errBadBlockKey
		}
		refs = append(refs, ref)
	}
	if err := it.Err(); err != nil {
		return err
	}
	it.Close()
	for _, ref := range refs {
		if err := fn(ref); err != nil {
			return err
		}
	}
	return nil
}
