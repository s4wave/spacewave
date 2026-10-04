package sobject_world_engine

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/routine"
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
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Engine is the world engine type.
type Engine = world.Engine

// StartEngineWithConfig starts the sobject world engine with a config and
// waits for the controller to run. A failed execution is transient, such as a
// predecessor engine still holding the write lease or World blocks still in
// transit, so the wait continues through the loader's retries until ctx ends.
// rel is called when the controller stops running. Release the reference to
// stop the controller.
func StartEngineWithConfig(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
	rel func(),
) (*Controller, directive.Instance, directive.Reference, error) {
	return loader.WaitExecControllerRunningRetryTyped[*Controller](
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
	// Trace the build.
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

	// Use the object's block store and its decoded block cache.
	blockStore := so.GetBlockStore()
	decodedBlocks := blockStore.GetDecodedBlockCache()
	if decodedBlocks == nil {
		decodedBlocks = block.NewDecodedBlockCache()
	}

	// The bucket ID is the block store ID. Replay keeps headRef, so bind a copy.
	bucketID := blockStore.GetID()
	headRef = headRef.CloneVT()
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
	// c serializes writes and replay.
	c *Controller
	// so supplies the operation set and accepts local operations.
	so sobject.SharedObject
	// bengine serves the replayed World and forks write candidates.
	bengine *world_block.Engine
	// replay computes the World from the operation set, guarded by the
	// controller's writer lock.
	replay *replayer
	// retained is the last head updateEngineState installed and held,
	// guarded by the controller's writer lock.
	retained *bucket.ObjectRef
	// retainedRoots are the retained roots updateEngineState last held,
	// guarded by the controller's writer lock.
	retainedRoots []*RetainedRoot
	// backfill copies the installed World and the retained roots into the
	// local store while this device's backfill choice is on.
	backfill *routine.StateRoutineContainer[bool]
	// rejected are the rejected edits last reported, guarded by the
	// controller's writer lock.
	rejected []*sobject.SORejectedEdit
	// mismatch is the checkpoint mismatch last reported, guarded by the
	// controller's writer lock.
	mismatch *sobject.SOCheckpointMismatch
	// unordered is the count of unplaced operations last reported, guarded by
	// the controller's writer lock.
	unordered uint32
	// control is the local voter in the group's decisions, or nil when the
	// shared object is not hosted here. Set before the engine is published.
	control *sobject.Control
}

// newSoEngine constructs the shared object engine.
func newSoEngine(c *Controller, so sobject.SharedObject, engine *world_block.Engine, replay *replayer) *soEngine {
	e := &soEngine{
		c:       c,
		so:      so,
		bengine: engine,
		replay:  replay,
	}
	e.backfill = routine.NewStateRoutineContainer[bool](
		nil,
		routine.WithExitLogger(c.le.WithField("routine", "world-backfill")),
		routine.WithRetry(backfillBackoff),
	)
	e.backfill.SetStateRoutine(e.executeBackfill)
	return e
}

// OperationAuthor returns the participant signing device and accepted entity.
// A participant without an entity uses its device as the person.
func (e *soEngine) OperationAuthor(ctx context.Context) (peer.ID, string, error) {
	// Resolve the person behind this device from the shared object state.
	snapshot, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return "", "", err
	}
	device := e.so.GetPeerID()
	person, err := operationPerson(ctx, snapshot, device)
	if err != nil {
		return "", "", err
	}
	return device, person, nil
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
// Writes replay the current operation set before forking and hold the writer
// lock until Commit or Discard. Always call Discard when done.
func (e *soEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	// Read transaction.
	if !write {
		return e.bengine.NewBlockEngineTransaction(ctx, false)
	}

	// Serialize the write fork with other writes and replay.
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

	// Fork the World of the current operation set. The watcher may still be
	// waiting for writeMtx after operations arrived.
	snapshot, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		unlockWriteMtx()
		return nil, err
	}
	if _, err := e.advance(ctx, snapshot, nil); err != nil {
		unlockWriteMtx()
		return nil, err
	}
	base, head, index := e.replay.head()

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
	fork := &replayFork{base: base, index: index, state: head}
	return newSoEngineWriteTx(ttx, btx, e, fork, unlockWriteMtx), nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (e *soEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return e.bengine.BuildStorageCursor(ctx)
}

// StageWorldState opens a staging scope on the block engine.
func (e *soEngine) StageWorldState(ctx context.Context) (world.WorldStage, error) {
	return e.bengine.StageWorldState(ctx)
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

// WaitObjectRev waits until the object at key reaches rev.
func (e *soEngine) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	return e.bengine.WaitObjectRev(ctx, key, rev, ignoreNotFound)
}

// advance replays snap, installs the World after its last placed operation
// and reports this device's rejected edits. fork, when set, supplies the World
// after a local write. The caller holds the writer lock.
func (e *soEngine) advance(ctx context.Context, snap sobject.SharedObjectStateSnapshot, fork *replayFork) ([]replayOutcome, error) {
	// Replay and install the World.
	state, outcomes, err := e.replay.sync(ctx, snap, fork)
	if err != nil {
		return nil, err
	}
	if err := e.updateEngineState(ctx, state); err != nil {
		return nil, err
	}

	// Report the edits it rejected.
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, err
	}
	e.reportRejectedEdits(set, outcomes)
	e.reportCheckpointMismatch()
	e.reportUnordered(set)

	// Trim the history every member has built on.
	if err := e.checkpointStable(ctx, snap, set); err != nil {
		e.c.le.WithError(err).Warn("failed to checkpoint the stable point")
	}
	return outcomes, nil
}

// reportRejectedEdits shows this device's revoked operations in the health of
// the SharedObject. An operation that replay never applied was rejected when it
// was written, and its writer was told then.
func (e *soEngine) reportRejectedEdits(set *sobject.SOOperationSet, outcomes []replayOutcome) {
	// Find the local operations that lost their place.
	self := e.so.GetPeerID().String()
	var edits []*sobject.SORejectedEdit
	for i, outcome := range outcomes {
		if !outcome.revoked || set.Get(outcome.hash).GetPeerId() != self {
			continue
		}
		edit := &sobject.SORejectedEdit{OpHash: outcome.hash, Reason: outcome.reason}
		if outcome.conflict {
			edit.LostToPeerIds = concurrentAuthors(set, outcomes[:i], outcome.hash, self)
		}
		edits = append(edits, edit)
	}

	// Show them when they changed.
	reporter, ok := e.so.(sobject.ReplayReporter)
	if !ok || slices.EqualFunc(edits, e.rejected, (*sobject.SORejectedEdit).EqualVT) {
		return
	}
	e.rejected = edits
	reporter.SetRejectedEdits(edits)
}

// reportCheckpointMismatch shows in the health of the SharedObject whether the
// latest checkpoint the replay judged holds a World other than its own.
func (e *soEngine) reportCheckpointMismatch() {
	// Skip an unchanged mismatch.
	mismatch := e.replay.mismatch
	reporter, ok := e.so.(sobject.ReplayReporter)
	if !ok || mismatch.EqualVT(e.mismatch) {
		return
	}

	// Log and show the change.
	if mismatch != nil {
		e.c.le.WithField("checkpoint-height", mismatch.GetHeight()).Warn("checkpoint World differs from the replayed World")
	}
	e.mismatch = mismatch
	reporter.SetCheckpointMismatch(mismatch)
}

// reportUnordered shows in the health of the SharedObject how many operations
// wait for the appointed sequencer to place them.
func (e *soEngine) reportUnordered(set *sobject.SOOperationSet) {
	// Report only a changed count.
	n := uint32(set.Unordered()) //nolint:gosec // the operations held in memory.
	reporter, ok := e.so.(sobject.ReplayReporter)
	if !ok || n == e.unordered {
		return
	}
	e.unordered = n
	reporter.SetUnorderedCount(n)
}

// concurrentAuthors returns the sorted authors, other than self, of the applied
// operations in earlier that the operation with hash h does not descend from.
func concurrentAuthors(set *sobject.SOOperationSet, earlier []replayOutcome, h []byte, self string) []string {
	// Collect the authors of applied operations h does not descend from.
	ancestors := set.Ancestors(h)
	var authors []string
	for _, outcome := range earlier {
		if outcome.reason != "" {
			continue
		}
		if _, ok := ancestors[string(outcome.hash)]; ok {
			continue
		}
		if author := set.Get(outcome.hash).GetPeerId(); author != self && !slices.Contains(authors, author) {
			authors = append(authors, author)
		}
	}

	// Sort them so every member names them alike.
	slices.Sort(authors)
	return authors
}

// queueOperation adds opData to the operation set as the local peer, replays
// the set and installs the World after it. fork, when set, supplies the World
// after the operation. It returns sobject.ErrRejectedOp wrapped with the
// reason when replay rejected the operation. The caller holds the writer lock.
func (e *soEngine) queueOperation(ctx context.Context, opData []byte, fork *replayFork) error {
	// Add the operation and find it in the set.
	localID, err := e.so.QueueOperation(ctx, opData)
	if err != nil {
		return err
	}
	snap, err := e.so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return err
	}
	h := set.Find(e.so.GetPeerID().String(), localID)
	if h == nil {
		return errors.New("queued operation is missing from the operation set")
	}

	// Replay it and install the World after it.
	if fork != nil {
		fork.hash = h
	}
	outcomes, err := e.advance(ctx, snap, fork)
	if err != nil {
		return err
	}

	// Report its outcome. It names every head, so replay places it.
	reason, placed := outcomeReason(outcomes, h)
	if !placed {
		return errors.New("replay did not place the queued operation")
	}
	if reason != "" {
		return errors.Wrap(sobject.ErrRejectedOp, reason)
	}
	return nil
}

// acknowledge queues an acknowledgment after every write transaction this
// device has started.
func (e *soEngine) acknowledge(ctx context.Context) error {
	// Queue it under the writer lock.
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlockWriteMtx()
	_, err = e.so.QueueOperation(ctx, nil)
	return err
}

// updateEngineState installs a replayed World, holds it and its retained roots
// in this participant's block store, and saves the replay that reached it. The
// caller holds the writer lock.
func (e *soEngine) updateEngineState(ctx context.Context, state *InnerState) error {
	// Trace the update.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/update-engine-state")
	defer task.End()

	// Install and hold a changed head once. The watcher, the next write and
	// the committing write all install the same head.
	ref := state.GetHeadRef().CloneVT()
	if ref == nil {
		ref = &bucket.ObjectRef{}
	}
	ref.BucketId = e.so.GetBlockStore().GetID()
	if !e.retained.EqualVT(ref) || !e.bengine.GetRootRef().EqualVT(ref) {
		if err := e.bengine.SetRootRef(ctx, ref); err != nil {
			return err
		}
		if err := holdWorldRoot(ctx, e.so, acceptedWorldRootName, ref.GetRootRef()); err != nil {
			return err
		}
		e.retained = ref
		e.c.notifyWrite()
	}

	// Hold changed retained roots.
	if !slices.EqualFunc(state.GetRetainedRoots(), e.retainedRoots, (*RetainedRoot).EqualVT) {
		if err := holdRootSet(ctx, e.so, retainedRootsName, state.GetRetainedRoots()); err != nil {
			return err
		}
		e.retainedRoots = state.GetRetainedRoots()
	}

	// Save the replay now that its World is held.
	return e.replay.save(ctx)
}

// acceptedWorldRootName names the local root that holds the installed World.
const acceptedWorldRootName = "accepted-world"

// holdWorldRoot holds the World graph under root in the local block store
// under the local root name, or releases the name when root is empty. It
// stores only the root block: the volume keeps every block this device wrote
// or read that the root still reaches, and reads fetch the rest on demand.
// Callers serialize calls through the writer lock.
func holdWorldRoot(ctx context.Context, so sobject.SharedObject, name string, root *block.BlockRef) error {
	// Release the name of an empty World.
	store := so.GetBlockStore()
	if !block.SupportsRootRetention(store) {
		return nil
	}
	if root.GetEmpty() {
		return block.SetRetainedRoot(ctx, store, name, nil)
	}

	// Store the root block before it replaces the named root.
	if err := storeRootBlock(ctx, so, root); err != nil {
		return err
	}
	return block.SetRetainedRoot(ctx, store, name, root)
}

// _ is a type assertion
var _ Engine = (*soEngine)(nil)
