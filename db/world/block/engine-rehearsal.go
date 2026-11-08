package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
)

// NewRehearsalTransaction opens a write fork of the newest accepted World
// without the writer slot or the Authority's turn.
//
// An operation applied to it reads the blocks its locked apply will read, so
// the locked apply finds them in the store's cache. Its writes stay in the
// fork's buffer, which nothing drains. Commit folds the writes into the World
// tree, reading the pages the locked commit reads, then discards the fork and
// returns world.ErrRehearsalCommit. The fork's result is advisory: the locked
// write applies the operation again on the head its turn starts from.
func (e *Engine) NewRehearsalTransaction(ctx context.Context) (world.Tx, error) {
	// Fork from the head the next turn starts from, read without a turn.
	var base world.Head
	if e.authority != nil {
		var err error
		if base, err = e.authority.Head(ctx); err != nil {
			return nil, err
		}
	}

	// Pick the root under the engine lock: the published head, or an
	// accepted base readers do not see yet.
	locked := e.bcast.Lock()
	defer locked.Unlock()
	if e.closed {
		return nil, e.closedErr()
	}
	root, rootBlock := e.head.root, (*World)(nil)
	var private *bucket_lookup.Cursor
	if !base.Root.GetEmpty() && !base.Root.EqualsRef(e.head.root.GetRef().GetRootRef()) {
		ref := e.head.root.GetRef().Clone()
		ref.RootRef = base.Root.Clone()
		var err error
		if private, err = e.baseRoot.FollowRef(ctx, ref); err != nil {
			return nil, err
		}
		root = private
		if a := e.admitted; a != nil && a.root.EqualsRef(base.Root) {
			rootBlock = a.world
		}
	}

	// Buffer every write, recording them so Discard releases them, and never
	// drain the buffer.
	buffered := block.NewBufferedStoreWithSettings(ctx, e.writeBlockStore, &block.BufferedStoreSettings{
		RecordWrites:    true,
		WithoutPeerWait: true,
	})
	ws, err := e.newWorldState(ctx, false, root, buffered, func() {}, rootBlock)
	if err != nil {
		if private != nil {
			private.Release()
		}
		return nil, err
	}

	// Hold the published head and the private base until Discard.
	ws.ownedStore, ws.releaseOwned = buffered, true
	held := e.pub.Load().hold()
	ws.readRelease = func() {
		held()
		if private != nil {
			private.Release()
		}
	}
	return &rehearsalTx{Tx: NewTx(ws)}, nil
}

// rehearsalTx is a write fork that never commits.
type rehearsalTx struct {
	// Tx is the fork's World state.
	*Tx
}

// Commit rehearses the locked commit: it folds the fork's writes into the
// World tree, reading the tree pages the locked commit of the same writes
// reads, then discards the fork without writing a block. It returns
// world.ErrRehearsalCommit, or the failure that stopped the rehearsal.
func (t *rehearsalTx) Commit(ctx context.Context) error {
	defer t.Discard()
	if err := t.PrepareCommit(ctx); err != nil {
		return err
	}
	return world.ErrRehearsalCommit
}

// _ is a type assertion
var (
	_ world.RehearsalEngine = (*Engine)(nil)
	_ world.Tx              = (*rehearsalTx)(nil)
)
