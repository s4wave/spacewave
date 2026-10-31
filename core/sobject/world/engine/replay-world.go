package sobject_world_engine

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/bucket"
	trace "github.com/s4wave/spacewave/db/traceutil"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// replayWorld serves the World a replay pass advances through one block
// engine. The engine defers block durability, so a pass makes the blocks of
// all its operations durable in one fence at close instead of one per
// operation. The owner serializes calls.
//
// A replayed World carries no completion proof. Its operations may point
// objects at block graphs their author wrote and this device never copied, so
// a complete base does not make it complete; the backfill copy proves it.
type replayWorld struct {
	// c builds the engine.
	c *Controller
	// so holds the World blocks.
	so sobject.SharedObject
	// ws serves head, or is nil before the first bind.
	ws *blkEngine
	// head is the World ws serves.
	head *bucket.ObjectRef
	// fenceErr is the error of a failed fence. The blocks the pass committed
	// before it are lost with the released engine.
	fenceErr error
}

// newReplayWorld constructs a replay World over the blocks of so.
func newReplayWorld(c *Controller, so sobject.SharedObject) *replayWorld {
	return &replayWorld{c: c, so: so}
}

// bind returns the engine serving head. It rebuilds the engine when the pass
// moved to a World the engine did not commit.
func (w *replayWorld) bind(ctx context.Context, head *bucket.ObjectRef) (*blkEngine, error) {
	// Reuse the engine that committed head.
	if w.ws != nil && w.head.EqualVT(head) {
		return w.ws, nil
	}

	// Fence the previous World before binding the next.
	if err := w.close(ctx); err != nil {
		return nil, err
	}
	ws, err := w.c.buildBlkEngine(ctx, w.c.le, w.so, head, head.GetTransformConf(), world_block.WithDeferredDurability())
	if err != nil {
		return nil, err
	}
	w.ws, w.head = ws, head.CloneVT()
	return ws, nil
}

// advance records that the engine committed head.
func (w *replayWorld) advance(head *bucket.ObjectRef) {
	w.head = head.CloneVT()
}

// close makes the committed blocks durable and releases the engine. A failed
// fence is kept in fenceErr, since a bind that fences returns its error
// through the operation it binds for.
func (w *replayWorld) close(ctx context.Context) error {
	// Detach the engine.
	if w.ws == nil {
		return nil
	}
	ws := w.ws
	w.ws, w.head = nil, nil
	defer ws.Release()

	// Fence the pass's writes.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/replay/fence")
	defer task.End()
	if _, err := ws.bengine.Sync(ctx); err != nil {
		w.fenceErr = err
		return err
	}
	return nil
}
