package sobject_world_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/peer"
)

// ErrRehearsalCommit is returned by Commit on a rehearsal transaction.
var ErrRehearsalCommit = errors.New("a rehearsal transaction cannot commit")

// RehearsalEngine is a World engine that applies operations to a write fork
// outside the writer lock.
type RehearsalEngine interface {
	// NewRehearsalTransaction opens a write fork of the newest World that
	// never commits. Always call Discard when done.
	NewRehearsalTransaction(ctx context.Context) (world.Tx, error)
}

// NewRehearsalTransaction opens a write fork of the newest World without the
// writer lock. An operation applied to it reads every block its locked apply
// will read, also after its first write, so the locked apply finds them in
// memory or on the drive. Its writes stay in the fork's buffer and Discard
// drops them. Commit reads the pages the locked commit of the writes would
// read, applies the graph writes and the change log to the fork, then
// discards it and returns ErrRehearsalCommit.
func (e *soEngine) NewRehearsalTransaction(ctx context.Context) (world.Tx, error) {
	// Fork the newest World with buffered writes.
	if e.admitted == nil {
		return nil, errors.New("the world engine is not ready")
	}
	btx, err := e.admitted.ForkBlockTransaction(ctx, true)
	if err != nil {
		return nil, err
	}

	// Record the operations applied to the fork.
	ws, err := world_block_tx.NewWorldState(ctx, btx, true)
	if err != nil {
		btx.Discard()
		return nil, err
	}
	return &rehearsalTx{WorldState: ws, btx: btx, eng: e}, nil
}

// rehearsalTx is a write fork that never commits.
type rehearsalTx struct {
	// WorldState records the operations applied to the fork.
	*world_block_tx.WorldState
	// btx contains the fork's World blocks.
	btx *world_block.Tx
	// eng attributes authenticated operations as a write would.
	eng *soEngine
}

// ApplyWorldOp applies op to the fork as soEngineWriteTx applies it to a
// candidate.
func (t *rehearsalTx) ApplyWorldOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	if _, authenticated := op.(world.AuthenticatedOperation); authenticated {
		participant, err := t.eng.operationParticipant(ctx)
		if err != nil {
			return 0, false, err
		}
		sender = t.eng.so.GetPeerID()
		ctx = withOperationAuthor(ctx, participant)
	}
	return t.WorldState.ApplyWorldOp(ctx, op, sender)
}

// Commit rehearses the locked commit: it folds the fork's writes into the
// World tree, which reads the tree pages the locked commit of the same writes
// reads, then discards the fork without writing a block. It always returns
// ErrRehearsalCommit, or the failure that stopped the rehearsal. The apply of
// an operation reads the pages of the objects it changes, but the graph tree
// takes its writes only here, and its pages are the cold ones.
func (t *rehearsalTx) Commit(ctx context.Context) error {
	defer t.Discard()
	if err := t.btx.PrepareCommit(ctx); err != nil {
		return err
	}
	return ErrRehearsalCommit
}

// Discard drops the fork and its buffered writes.
// Can be called unlimited times.
func (t *rehearsalTx) Discard() {
	t.WorldState.Discard()
	t.btx.Discard()
}

// _ is a type assertion
var (
	_ RehearsalEngine = (*soEngine)(nil)
	_ world.Tx        = (*rehearsalTx)(nil)
)
