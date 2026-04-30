package sobject_world_engine

import (
	"context"

	trace "github.com/s4wave/spacewave/db/traceutil"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/sirupsen/logrus"
)

// Engine is the world engine type.
type Engine = world.Engine

// StartEngineWithConfig starts the sobject world engine with a config.
// Waits for the controller to start.
// Returns a Release function to close the controller when done.
func StartEngineWithConfig(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
	rel func(),
) (*Controller, directive.Instance, directive.Reference, error) {
	return loader.WaitExecControllerRunningTyped[*Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(conf),
		rel,
	)
}

// blkEngine contains a world state with engine.
type blkEngine struct {
	bengine  *world_block.Engine
	cursor   *bucket_lookup.Cursor
	lookupOp world.LookupOp
}

// Release releases the engine resources.
func (w *blkEngine) Release() {
	w.cursor.Release()
}

// buildBlkEngine builds a world state with engine from a head ref.
// The caller must call Release() on the returned WorldState when done.
func (c *Controller) buildBlkEngine(
	ctx context.Context,
	le *logrus.Entry,
	so sobject.SharedObject,
	headRef *bucket.ObjectRef,
	transformConf *block_transform.Config,
) (*blkEngine, error) {
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/build-block-engine")
	defer task.End()

	// verify transform config is not empty
	if len(transformConf.GetSteps()) == 0 {
		return nil, sobject.ErrEmptyTransformConfig
	}

	// construct the transformer
	var xfrm *block_transform.Transformer
	{
		_, task := trace.NewTask(ctx, "alpha/so-engine/build-block-engine/new-transformer")
		var err error
		xfrm, err = block_transform.NewTransformer(
			controller.ConstructOpts{Logger: le},
			c.sfs,
			transformConf,
		)
		task.End()
		if err != nil {
			return nil, err
		}
	}

	// the bucket ID is equivalent to the block store id
	bucketID := so.GetBlockStore().GetID()
	headRef.BucketId = bucketID

	// build cursor with shared object block store
	var cursor *bucket_lookup.Cursor
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/build-block-engine/new-cursor")
		cursor = bucket_lookup.NewCursor(
			taskCtx,
			c.bus,
			le,
			c.sfs,
			so.GetBlockStore(),
			xfrm,
			headRef,
			&bucket.BucketOpArgs{
				BucketId: so.GetBlockStore().GetID(),
				VolumeId: so.GetBlockStore().GetID(),
			},
			transformConf,
		)
		task.End()
	}

	var lookupWorldOp world.LookupOp
	if !c.conf.GetDisableLookup() {
		lookupWorldOp = world.BuildLookupWorldOpFunc(c.bus, le, c.engineID)
	}

	// Build the world engine
	var bengine *world_block.Engine
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/build-block-engine/new-world-engine")
		var err error
		bengine, err = world_block.NewEngine(
			taskCtx,
			le,
			cursor,
			lookupWorldOp,
			nil, // no commit function needed
			c.conf.GetVerbose(),
		)
		task.End()
		if err != nil {
			cursor.Release()
			return nil, err
		}
	}

	return &blkEngine{
		bengine:  bengine,
		cursor:   cursor,
		lookupOp: lookupWorldOp,
	}, nil
}

// soEngine implements the world engine logic for the shared object.
type soEngine struct {
	// c is the controller
	c *Controller
	// so is the shared object
	so sobject.SharedObject
	// bengine is the block engine used for reading
	bengine *world_block.Engine
}

// newSoEngine constructs the shared object engine.
func newSoEngine(c *Controller, so sobject.SharedObject, engine *world_block.Engine) *soEngine {
	return &soEngine{
		c:       c,
		so:      so,
		bengine: engine,
	}
}

// wrapReleaseWithTask ends task when release is called.
func wrapReleaseWithTask(release func(), task *trace.Task) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			task.End()
			release()
		})
	}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
// Check GetReadOnly, might not return a write tx if write=true.
func (e *soEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	// Read transaction.
	if !write {
		return e.bengine.NewBlockEngineTransaction(ctx, false)
	}

	ctx, task := trace.NewTask(ctx, "alpha/so-engine/new-transaction")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "alpha/so-engine/new-transaction/lock-write-mtx")
	unlockWriteMtx, err := e.c.writeMtx.Lock(taskCtx)
	subtask.End()
	if err != nil {
		return nil, err
	}
	_, holdWriteMtxTask := trace.NewTask(ctx, "alpha/so-engine/write-tx/hold-write-mtx")
	unlockWriteMtx = wrapReleaseWithTask(unlockWriteMtx, holdWriteMtxTask)

	// Construct the block engine txn.
	var btx *world_block.Tx
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/new-transaction/fork-block-transaction")
		var err error
		btx, err = e.bengine.ForkBlockTransaction(taskCtx, true)
		task.End()
		if err != nil {
			unlockWriteMtx()
			return nil, err
		}
	}

	// Construct the txn buffer.
	var ttx *world_block_tx.WorldState
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/new-transaction/new-world-state")
		var err error
		ttx, err = world_block_tx.NewWorldState(taskCtx, btx, write)
		task.End()
		if err != nil {
			btx.Discard()
			unlockWriteMtx()
			return nil, err
		}
	}

	// Return the txn wrapper.
	return newSoEngineWriteTx(ttx, btx, e, unlockWriteMtx), nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (e *soEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return e.bengine.BuildStorageCursor(ctx)
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns a cursor pointing to the root world state.
// The lookup cursor will be released after cb returns.
func (e *soEngine) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return e.bengine.AccessWorldState(ctx, ref, cb)
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (e *soEngine) GetSeqno(ctx context.Context) (uint64, error) {
	return e.bengine.GetSeqno(ctx)
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (e *soEngine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return e.bengine.WaitSeqno(ctx, value)
}

// updateEngineState updates the engine's internal state with a new head ref
func (e *soEngine) updateEngineState(ctx context.Context, headRef *bucket.ObjectRef) error {
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/update-engine-state")
	defer task.End()

	// Clone the ref to avoid mutations
	ref := headRef.CloneVT()
	// Set bucket ID to match the block store
	ref.BucketId = e.so.GetBlockStore().GetID()
	// Update the engine state
	return e.bengine.SetRootRef(ctx, ref)
}

// _ is a type assertion
var _ Engine = ((*soEngine)(nil))
