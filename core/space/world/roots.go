package space_world

import (
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// The named roots a Space holds on its SharedObject's block store. Each holds
// one World root block, or one small block naming several roots, and moves
// only through SetRoots with the reference it held as Previous.
const (
	// RootReplayBase holds the World of the adopted checkpoint, which every
	// replay starts from.
	RootReplayBase = "replay-base"
	// RootCheckpoint holds the HeldRoots block of every World Root returned
	// since the last Install, so the one a checkpoint adopts stays held until
	// Install moves the replay base onto it.
	RootCheckpoint = "checkpoint"
	// RootImport holds the HeldRoots chain of the roots a writer outside the
	// Adapter stored for the Space, such as a migration's copied World, until
	// the Adapter holds each of them itself (see HoldImport).
	RootImport = "import"
	// RootReplayCursor holds the HeldRoots block of every World the replay
	// cursor was saved at since a checkpoint covered them, the last one the
	// World after the positions the saved cursor records.
	RootReplayCursor = "replay-cursor"
	// RootReplayPayloads holds the HeldRoots block naming the payload
	// roots of the operations above the checkpoint, and of the local
	// candidates queued after them.
	RootReplayPayloads = "replay-payloads"
	// RootRetained holds the RetainedRootSet block of the installed head's
	// retained roots.
	RootRetained = "retained"
	// RootPending holds the HeldRoots block of every local candidate whose
	// pages outgrew memory since a checkpoint covered them, so the pages its
	// writer's stage held stay held once the turn ends.
	RootPending = "pending"
	// RootBackfill holds what this device's backfill copied while its
	// backfill choice is on.
	RootBackfill = "backfill"
)

// heldRootNames are the roots an Adapter reads at open.
var heldRootNames = []string{
	RootReplayBase,
	RootCheckpoint,
	RootImport,
	RootReplayCursor,
	RootReplayPayloads,
	RootRetained,
	RootPending,
	RootBackfill,
}

// HoldImport holds roots, which a writer outside an Adapter stored for the
// Space, such as a migration's copied World, under the import root on store.
// Each stays held there until the Space's Adapter holds it itself: a World
// until a trim moves the replay base onto it, and any other root until the
// replay payloads root holds it. That trim's move releases it, so the hold
// passes to the Adapter's root with no gap. A root the Adapter never takes,
// such as the World of a checkpoint the Space had already passed, stays held
// until the Space is deleted.
//
// Invariants, which the Volume's collector relies on to free nothing live:
//
//  1. The caller holds roots until this returns, through a stage or another
//     root, so the append is never the first hold of a block at zero.
//  2. The append names the import root's current block as its Prev and as
//     the move's Previous, so it keeps every root the chain holds, and a
//     concurrent Adapter move of the root fails and rereads rather than
//     overwrite the append.
//  3. An Adapter releases a root from the chain only in a root move that
//     holds it elsewhere: the trim that moves the replay base onto it, or a
//     trim while the payloads root holds it. Whether it does is read from the
//     stored roots under the root lock at the move, never from a plan
//     gathered earlier, so a restart or a plan gathered before the append
//     cannot release it.
//  4. It never moves an Adapter's own roots.
//
// It skips a root the replay base or the chain holds already, and does
// nothing on a store that holds no roots, which keeps every block it stores.
// An append that loses to an Adapter's move reads the roots again and retries
// after a short backoff, at most holdImportTries times: an Adapter moves the
// root once per install, so a run of lost appends means the Space is busy,
// and the error, which wraps block.ErrRootMoved, says the caller's own retry,
// such as a migration's, can try again later.
func HoldImport(ctx context.Context, store block.StoreOps, roots ...*block.BlockRef) error {
	// A store that holds no roots needs no hold.
	roots = slices.DeleteFunc(dedupRefs(roots), func(r *block.BlockRef) bool { return r.GetEmpty() })
	if len(roots) == 0 || !block.HoldsRoots(store) {
		return nil
	}
	var err error
	delay := holdImportBackoff
	for i := range holdImportTries {
		if i != 0 {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case <-time.After(delay):
			}
			delay *= 2
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		err = appendImport(ctx, store, roots)
		if !errors.Is(err, block.ErrRootMoved) {
			return err
		}
	}
	return errors.Wrapf(err, "the Space's roots kept moving through %d tries to hold the import; the import can be retried", holdImportTries)
}

// holdImportTries bounds the appends HoldImport tries, and holdImportBackoff
// is the wait before the second, doubling after each, so the tries span about
// three seconds of an Adapter's root moves.
const (
	holdImportTries   = 8
	holdImportBackoff = 25 * time.Millisecond
)

// appendImport is one try of HoldImport.
func appendImport(ctx context.Context, store block.StoreOps, roots []*block.BlockRef) error {
	// Leave a World the replay base holds.
	base, err := store.GetRoot(ctx, RootReplayBase)
	if err != nil {
		return err
	}
	roots = slices.DeleteFunc(slices.Clone(roots), func(r *block.BlockRef) bool { return refEqual(r, base) })

	// Leave the roots the chain holds, reading it newest first.
	held, err := store.GetRoot(ctx, RootImport)
	if err != nil {
		return err
	}
	for ref := held; !ref.GetEmpty() && len(roots) != 0; {
		data, found, err := store.GetBlock(ctx, ref)
		if err != nil {
			return err
		}
		if !found {
			return errors.Errorf("import root block %s not found", ref.MarshalString())
		}
		msg := &HeldRoots{}
		if err := msg.UnmarshalVT(data); err != nil {
			return err
		}
		inBlock := make(map[string]struct{}, len(msg.GetRoots()))
		for _, r := range msg.GetRoots() {
			inBlock[r.MarshalString()] = struct{}{}
		}
		roots = slices.DeleteFunc(roots, func(r *block.BlockRef) bool {
			_, ok := inBlock[r.MarshalString()]
			return ok
		})
		ref = msg.GetPrev()
	}
	if len(roots) == 0 {
		return nil
	}

	// Append the rest, linking the block the root holds now.
	refs := slices.Clone(roots)
	if !held.GetEmpty() {
		refs = append(refs, held)
	}
	entry, err := refsEntry(&HeldRoots{Roots: roots, Prev: held}, refs)
	if err != nil {
		return err
	}
	return store.SetRoots(ctx, []*block.PutBatchEntry{entry}, []block.NamedRoot{{Name: RootImport, Ref: entry.Ref, Previous: held}})
}
