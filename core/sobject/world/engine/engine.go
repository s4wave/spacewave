package sobject_world_engine

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
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
	// bengine serves the World rooted at cursor.
	bengine *world_block.Engine
	// lookupOp resolves operations supported by this World.
	lookupOp world.LookupOp
}

// Release releases the engine resources.
func (w *blkEngine) Release() {
	_ = w.bengine.Close()
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
	return buildBlockEngine(ctx, le, c.bus, c.sfs, so, headRef, transformConf, c.buildLookupWorldOp(le), c.conf.GetVerbose())
}

// buildBlockEngine binds a World root to its SharedObject block store.
func buildBlockEngine(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	headRef *bucket.ObjectRef,
	transformConf *block_transform.Config,
	lookupWorldOp world.LookupOp,
	verbose bool,
) (*blkEngine, error) {
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/build-block-engine")
	defer task.End()

	// verify transform config is not empty
	if len(transformConf.GetSteps()) == 0 {
		return nil, sobject.ErrEmptyTransformConfig
	}

	// construct the transformer
	var xfrm block.Transformer
	{
		_, task := trace.NewTask(ctx, "alpha/so-engine/build-block-engine/new-transformer")
		var err error
		xfrm, err = newWorldTransformer(
			controller.ConstructOpts{Logger: le},
			sfs,
			transformConf,
		)
		task.End()
		if err != nil {
			return nil, err
		}
	}

	blockStore := so.GetBlockStore()
	decodedBlocks := blockStore.GetDecodedBlockCache()
	if decodedBlocks == nil {
		decodedBlocks = block.NewDecodedBlockCache()
	}

	// the bucket ID is equivalent to the block store id
	bucketID := blockStore.GetID()
	headRef.BucketId = bucketID

	// build cursor with shared object block store
	var cursor *bucket_lookup.Cursor
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/build-block-engine/new-cursor")
		cursor = bucket_lookup.NewCursor(
			taskCtx,
			b,
			le,
			sfs,
			blockStore,
			xfrm,
			headRef,
			&bucket.BucketOpArgs{
				BucketId: bucketID,
				VolumeId: bucketID,
			},
			transformConf,
		)
		// A shared-object copy mounts its complete DAG through its local bucket.
		// Preserve explicit cross-store references, but resolve implicit authoring
		// bucket references through that local mirror and its DEX read-through.
		cursor.SetBucketIDOverride(bucketID)
		cursor.SetDecodedBlockCache(decodedBlocks)
		task.End()
	}

	// Transfer cursor ownership to the World engine, including constructor failure.
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
			verbose,
		)
		task.End()
		if err != nil {
			return nil, err
		}
	}

	return &blkEngine{
		bengine:  bengine,
		lookupOp: lookupWorldOp,
	}, nil
}

// soEngine implements the world engine logic for the shared object.
type soEngine struct {
	// c serializes writes and accepted-root adoption.
	c *Controller
	// so supplies authority snapshots and accepts submitted operations.
	so sobject.SharedObject
	// bengine serves the accepted World and forks write candidates.
	bengine *world_block.Engine
	// retained is the last head updateEngineState installed and retained,
	// guarded by the controller's writer lock.
	retained *bucket.ObjectRef
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
	var fired atomic.Bool
	return func() {
		if !fired.CompareAndSwap(false, true) {
			return
		}
		task.End()
		release()
	}
}

// NewTransaction opens a read snapshot or a serialized write candidate.
// Writes refresh from one accepted SharedObject snapshot before forking and
// retain that authority base until Commit. Always call Discard when done.
func (e *soEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	// Read transaction.
	if !write {
		return e.bengine.NewBlockEngineTransaction(ctx, false)
	}

	// Serialize the write fork with other writes and accepted-root adoption.
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

	// Refresh both transaction bases from one accepted snapshot. The watcher
	// may still be waiting for writeMtx after a remote root has advanced.
	snapshot, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		unlockWriteMtx()
		return nil, err
	}
	baseRoot, err := snapshot.GetRootState(ctx)
	if err != nil {
		unlockWriteMtx()
		return nil, err
	}
	if baseRoot == nil {
		unlockWriteMtx()
		return nil, errors.New("base SharedObject root is missing")
	}
	head, err := finalizationWorldRoot(ctx, snapshot)
	if err != nil {
		unlockWriteMtx()
		return nil, err
	}
	if err := e.updateEngineState(ctx, head); err != nil {
		unlockWriteMtx()
		return nil, err
	}

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
	return newSoEngineWriteTx(ttx, btx, e, baseRoot.CloneVT(), unlockWriteMtx), nil
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

// Sync fences durable storage and advances the durable head via the engine.
func (e *soEngine) Sync(ctx context.Context) (bool, error) {
	return e.bengine.Sync(ctx)
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (e *soEngine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return e.bengine.WaitSeqno(ctx, value)
}

// updateEngineState installs an accepted head using this participant's block store.
func (e *soEngine) updateEngineState(ctx context.Context, headRef *bucket.ObjectRef) error {
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/update-engine-state")
	defer task.End()

	// Preserve the authority reference while resolving its blocks locally.
	// The accepted watch, the next write and the committing write all
	// install the same head; retain it once.
	ref := headRef.CloneVT()
	ref.BucketId = e.so.GetBlockStore().GetID()
	if e.retained.EqualVT(ref) && e.bengine.GetRootRef().EqualVT(ref) {
		return nil
	}
	if err := e.bengine.SetRootRef(ctx, ref); err != nil {
		return err
	}
	store := e.so.GetBlockStore()
	if ref.GetRootRef().GetEmpty() || !block.SupportsRootRetention(store) {
		e.retained = ref
		return nil
	}
	// Retire only accepted, completely local graphs. This runs under the
	// controller's existing writer lock, including accepted-root watches.
	proofs, release, err := e.so.AccessLocalStateStore(ctx, "accepted-world-retention", nil)
	if err != nil {
		return err
	}
	defer release()
	if err := RetainWorld(ctx, e.c.le, e.c.sfs, e.so, ref, proofs, nil); err != nil {
		return err
	}
	if err := block.SetRetainedRoot(ctx, store, "accepted-world", ref.GetRootRef()); err != nil {
		return err
	}
	e.retained = ref
	return nil
}

// _ is a type assertion
var _ Engine = (*soEngine)(nil)
