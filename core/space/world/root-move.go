package space_world

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// landHook, when set, runs before each admission's queued root moves land,
// with whether it queued one that has not landed. An error drops every queued
// move unstored, as a crash before the durable step loses them, and fails the
// admission.
var landHook atomic.Pointer[func(queued bool) error]

// lockRootMtx takes rootMtx. When a dropped queued move left the held roots
// and root sets ahead of the store, it lands what is queued and reads them
// again, before the caller builds a move from them.
func (a *Adapter) lockRootMtx(ctx context.Context) (func(), error) {
	// Take the lock, and return at once unless a dropped move left the view
	// stale.
	unlock, err := a.rootMtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	if !a.deferMoves || !a.moves.isStale() {
		return unlock, nil
	}
	if err := a.reloadRootsLocked(ctx); err != nil {
		unlock()
		return nil, err
	}
	return unlock, nil
}

// reloadRootsLocked lands or drops the queued moves, then reads the held
// roots and root sets from the store. rootMtx keeps the queue empty until the
// caller releases it. The caller holds rootMtx.
func (a *Adapter) reloadRootsLocked(ctx context.Context) error {
	// Empty the queue, then read the view the store holds.
	if err := a.moves.landAll(ctx, a.pages); err != nil && ctx.Err() != nil {
		return err
	}
	if err := a.readHeldRoots(ctx, heldRootNames); err != nil {
		return err
	}
	if err := a.readRootSets(ctx); err != nil {
		return err
	}
	a.moves.fresh()
	return nil
}

// landQueued lands adm's queued root moves, and those queued before them,
// ahead of the state write that queues its operation, ordered behind that
// write when the adapter orders holds. It returns the error that dropped one
// of adm's moves.
func (a *Adapter) landQueued(ctx context.Context, adm *spaceAdmission) error {
	if !a.deferMoves {
		return nil
	}
	if hook := landHook.Load(); hook != nil {
		if err := (*hook)(a.moves.queued(&adm.moves)); err != nil {
			if derr := a.moves.drop(ctx, err); derr != nil {
				return derr
			}
			return err
		}
	}
	return a.landMoves(ctx, adm)
}

// landMoves lands adm's queued root moves, and those queued before them,
// ordered behind adm's state write when the adapter orders holds. It returns
// the error of the drop that failed adm.
func (a *Adapter) landMoves(ctx context.Context, adm *spaceAdmission) error {
	if a.orderedHolds {
		ctx = kvtx.WithOrderedCommitContext(ctx)
	}
	return a.moves.landFor(ctx, a.pages, &adm.moves)
}

// landShutdown lands the queued root moves once no turn queues more. A move
// that cannot land is lost as in a crash: its operation's state was never
// written.
func (a *Adapter) landShutdown(ctx context.Context) {
	// Land under rootMtx, past the run's cancellation.
	if !a.deferMoves {
		return
	}
	ctx = context.WithoutCancel(ctx)
	unlock, err := a.rootMtx.Lock(ctx)
	if err == nil {
		err = a.moves.landAll(ctx, a.pages)
		unlock()
	}
	if err != nil {
		a.le.WithError(err).Warn("queued admission root moves did not land at close")
	}
}

// moveRootsLocked swaps roots through the kept pages. The caller holds rootMtx.
func (a *Adapter) moveRootsLocked(ctx context.Context, entries []*block.PutBatchEntry, roots ...block.NamedRoot) error {
	return a.moveRootsOn(ctx, a.pages, entries, roots...)
}

// moveRootsOn swaps changed held roots under the caller's rootMtx hold.
func (a *Adapter) moveRootsOn(ctx context.Context, store block.RootOps, entries []*block.PutBatchEntry, roots ...block.NamedRoot) error {
	// A transient replay holds nothing.
	if a.opts.transient {
		return nil
	}

	// Move only the roots that change, from what they hold.
	moves := make([]block.NamedRoot, 0, len(roots))
	for _, root := range roots {
		prev := a.held[root.Name]
		if refEqual(prev, root.Ref) {
			continue
		}
		root.Previous = prev
		moves = append(moves, root)
	}
	if len(moves) == 0 && len(entries) == 0 {
		return nil
	}

	// Queue an admission's move on the kept pages, or land the queued moves
	// before a move that builds on what they hold.
	if owner := queuing(ctx); owner != nil && store == block.RootOps(a.pages) {
		if err := a.queueMove(ctx, owner, entries, moves); err != nil {
			return err
		}
		a.noteHeld(moves)
		return nil
	}
	if a.deferMoves {
		if err := a.moves.landAll(ctx, a.pages); err != nil {
			return err
		}
	}
	if err := store.SetRoots(ctx, entries, moves); err != nil {
		return err
	}
	a.noteHeld(moves)
	return nil
}

// queueMove queues the moves for owner's durable step once the blocks they
// require held are held now, as block.NamedRoot.RequireHeld decides. It
// refuses with block.ErrRootNotHeld as SetRoots would. When the store cannot
// tell, it queues the moves and lands them at once, so SetRoots checks them.
// The caller holds rootMtx.
func (a *Adapter) queueMove(ctx context.Context, owner *admissionMoves, entries []*block.PutBatchEntry, moves []block.NamedRoot) error {
	// Find the stored blocks the strict moves reach past the blocks they and
	// the queued moves store.
	var need []*block.BlockRef
	for _, root := range moves {
		if root.RequireHeld && !root.Ref.GetEmpty() {
			need = append(need, a.storedBoundary(root.Ref, entries)...)
		}
	}

	// Require them held in one reader transaction.
	if len(need) != 0 {
		held, err := block.HeldRoots(ctx, a.pages, need)
		if errors.Is(err, block.ErrHeldUnknown) {
			a.heldUnknown.Do(func() {
				a.le.Info("block store cannot report held blocks, admission root moves land in the turn")
			})
			if err := a.moves.push(owner, entries, moves); err != nil {
				return err
			}
			return a.moves.landFor(ctx, a.pages, owner)
		}
		if err != nil {
			return err
		}
		if i := slices.Index(held, false); i >= 0 {
			return errors.Wrap(block.ErrRootNotHeld, need[i].MarshalString())
		}
	}
	return a.moves.push(owner, entries, moves)
}

// storedBoundary returns the blocks the tree under ref reaches outside entries
// and the queued moves' entries, which a landing requires held.
func (a *Adapter) storedBoundary(ref *block.BlockRef, entries []*block.PutBatchEntry) []*block.BlockRef {
	// Index the blocks the moves store.
	stores := a.moves.entries()
	for _, e := range entries {
		stores[e.Ref.MarshalString()] = e
	}

	// Walk the stored tree down to the blocks they do not store.
	var out []*block.BlockRef
	walk := []*block.BlockRef{ref}
	seen := make(map[string]struct{})
	for len(walk) != 0 {
		next := walk[len(walk)-1]
		walk = walk[:len(walk)-1]
		key := next.MarshalString()
		if _, ok := seen[key]; ok || next.GetEmpty() {
			continue
		}
		seen[key] = struct{}{}
		if e := stores[key]; e != nil {
			walk = append(walk, e.Refs...)
		} else {
			out = append(out, next)
		}
	}
	return out
}
