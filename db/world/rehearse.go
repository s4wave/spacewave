package world

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/peer"
)

// ApplyRehearsed applies op to ws as ApplyWorldOp does. When ws writes through
// an Engine that can rehearse, op first runs on a rehearsal, outside the
// writer slot, so the locked apply finds the blocks it reads in memory, and a
// rejection on the rehearsal returns without taking the slot.
//
// op must be one whose apply has no effect outside the World, since it runs
// twice.
func ApplyRehearsed(ctx context.Context, ws WorldState, op Operation, sender peer.ID) (uint64, bool, error) {
	if e, ok := ws.(*engineWorldState); ok && e.write {
		if err := Rehearse(ctx, e.e, sender, op); err != nil {
			return 0, false, err
		}
	}
	return ws.ApplyWorldOp(ctx, op, sender)
}

// Rehearse applies ops in order to a rehearsal of eng's newest World, has it
// fold their writes into the World tree, and discards it, so a locked write of
// the same ops reads warm blocks. Ops that commit together rehearse together.
//
// It returns the rejection of the first op the rehearsal rejects. The
// rehearsal starts from the newest accepted head, so a rejection is the one a
// locked write would give if it ran now: a write the caller saw complete is in
// that head, and a write still in flight has not yet happened. An engine that
// cannot rehearse, and a rehearsal that fails for a reason other than a
// rejection, return nil and leave the decision to the locked write.
func Rehearse(ctx context.Context, eng Engine, sender peer.ID, ops ...Operation) error {
	var rejected error
	_ = AccessEngine(ctx, eng, func(ctx context.Context, eng Engine) error {
		// Open the rehearsal on an engine that can rehearse.
		rejected = nil
		re, ok := eng.(RehearsalEngine)
		if !ok {
			return nil
		}
		tx, err := re.NewRehearsalTransaction(ctx)
		if err != nil {
			return nil
		}
		defer tx.Discard()

		// Apply the ops in order, stopping at the first failure.
		for _, op := range ops {
			_, sysErr, err := tx.ApplyWorldOp(ctx, op, sender)
			if err == nil {
				continue
			}
			if isRehearsalRejection(ctx, sysErr, err) {
				rejected = err
			}
			return nil
		}

		// Fold the writes, reading the pages the locked commit reads.
		_ = tx.Commit(ctx)
		return nil
	})
	return rejected
}

// isRehearsalRejection reports whether err from an operation's apply on a
// rehearsal is the operation's own rejection, not a failure the locked write
// may not repeat: a system error, a canceled context, a superseded head, or a
// block the rehearsal would not wait for a peer to serve.
func isRehearsalRejection(ctx context.Context, sysErr bool, err error) bool {
	return !sysErr && ctx.Err() == nil && !retryWorldOp(err) && !errors.Is(err, block.ErrUnavailable)
}
