package world_block

import "github.com/s4wave/spacewave/db/block"

// ReadRoots returns the World roots the engine's readers may still read: every
// published head a holder still holds, including one a reader acquired and has
// not registered yet, the root of each open read snapshot, the base of the
// write transaction, and the heads the change watch compares. A store that forgets
// pages no root reaches, such as a PendingStore sweep, keeps what these reach,
// so a reader of a replaced head never finds its pages gone before the
// engine pins them. It reads one entry per open transaction.
func (e *Engine) ReadRoots() []*block.BlockRef {
	// Gather the published head, the writer's base and the watched heads.
	var roots []*block.BlockRef
	locked := e.bcast.Lock()
	if p := e.pub.Load(); p != nil {
		roots = append(roots, p.ref.GetRootRef())
	}
	if w := e.writeTx; w != nil {
		roots = append(roots, w.baseHeadRef.GetRootRef())
	}
	for _, h := range e.watchedHeads {
		roots = append(roots, h.root.GetRef().GetRootRef())
	}

	// Gather each open snapshot's root.
	e.snapMu.Lock()
	for tx := range e.snapshotTxs {
		roots = append(roots, tx.readRoot.GetRootRef())
	}
	e.snapMu.Unlock()
	locked.Unlock()

	// Gather every head a holder still holds.
	e.liveMu.Lock()
	for p := range e.live {
		roots = append(roots, p.ref.GetRootRef())
	}
	e.liveMu.Unlock()
	return roots
}
