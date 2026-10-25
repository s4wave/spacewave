package sobject_world_engine

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Engine is the world engine type.
type Engine = world.Engine

// blkEngine contains a world state with engine.
type blkEngine struct {
	// bengine serves the World rooted at cursor.
	bengine *world_block.Engine
	// bcast guards transaction admission and release.
	bcast broadcast.Broadcast
	// transactions counts read snapshots holding the engine open. Guarded by bcast.
	transactions uint64
	// releasing prevents new read snapshots once Release starts. Guarded by bcast.
	releasing bool
	// lookupOp resolves operations supported by this World.
	lookupOp world.LookupOp
}

// Release stops read admission and closes the engine after its last read
// transaction finishes. The caller retains the SharedObject until this returns.
func (w *blkEngine) Release() {
	// Retire the engine and wait for its read transactions to release their snapshots.
	locked := w.bcast.Lock()
	w.releasing = true
	for w.transactions != 0 {
		wait := locked.WaitCh()
		locked.Unlock()
		<-wait
		locked = w.bcast.Lock()
	}
	locked.Unlock()

	// Close only after no transaction can still read from the block engine.
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
	opts ...world_block.EngineOption,
) (*blkEngine, error) {
	return buildBlockEngine(ctx, le, c.bus, c.sfs, so, headRef, transformConf, c.buildLookupWorldOp(le), c.conf.GetVerbose(), opts...)
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
	opts ...world_block.EngineOption,
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
			opts...,
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
	// lifetime ends when the controller stops serving this engine. A commit
	// that started ends with it.
	lifetime context.Context
	// c serializes writes and replay.
	c *Controller
	// so supplies the operation set and accepts local operations.
	so sobject.SharedObject
	// block owns transaction admission and the block engine lifetime.
	block *blkEngine
	// bengine serves the replayed World and forks write candidates.
	bengine *world_block.Engine
	// replay computes the World from the operation set, guarded by the
	// controller's writer lock.
	replay *replayer
	// retained is the last head held under the accepted World root, by
	// updateEngineState or an admitted write, guarded by the controller's
	// writer lock.
	retained *bucket.ObjectRef
	// admissions are the admitted local writes whose receipts are not settled,
	// in admission order, guarded by the controller's writer lock.
	admissions []*writeAdmission
	// retainedRoots are the retained roots updateEngineState last held,
	// guarded by the controller's writer lock.
	retainedRoots []*RetainedRoot
	// backfill copies the installed World and the retained roots into the
	// local store while this device's backfill choice is on.
	backfill *routine.StateRoutineContainer[bool]
	// backfillChoice is this device's backfill choice, written under the
	// controller's writer lock.
	backfillChoice *ccontainer.CContainer[bool]
	// backfillState is the outcome of the last finished copy while backfill
	// is on, written under the controller's writer lock.
	backfillState *ccontainer.CContainer[*BackfillState]
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
	// stableNext is the latest prepared checkpoint at the stable point that
	// the checkpoint routine has not taken, and stableWake wakes the routine.
	stableNext atomic.Pointer[stableCheckpoint]
	stableWake chan struct{}
}

// newSoEngine constructs the shared object engine. lifetime bounds the commits
// the engine finishes after their callers stop waiting.
func newSoEngine(lifetime context.Context, c *Controller, so sobject.SharedObject, engine *blkEngine, replay *replayer) *soEngine {
	// The engine serves the World the replay reached, and replay must not
	// leave it for a World behind it.
	replay.served = true
	e := &soEngine{
		lifetime:       lifetime,
		c:              c,
		so:             so,
		block:          engine,
		bengine:        engine.bengine,
		replay:         replay,
		backfillChoice: ccontainer.NewCContainer(false),
		backfillState:  ccontainer.NewCContainerVT[*BackfillState](nil),
		stableWake:     make(chan struct{}, 1),
	}
	e.backfill = routine.NewStateRoutineContainer[bool](
		nil,
		routine.WithExitLogger(c.le.WithField("routine", "world-backfill")),
		routine.WithRetry(backfillBackoff),
	)
	e.backfill.SetStateRoutine(e.executeBackfill)
	go e.executeStableCheckpoints()
	return e
}

// OperationAuthor returns the participant signing device and accepted entity.
// A participant without an entity uses its device as the person.
func (e *soEngine) OperationAuthor(ctx context.Context) (peer.ID, string, error) {
	participant, err := e.operationParticipant(ctx)
	if err != nil {
		return "", "", err
	}
	return e.so.GetPeerID(), participantPerson(participant), nil
}

// operationParticipant returns this device's participant in the accepted
// shared object config.
func (e *soEngine) operationParticipant(ctx context.Context) (*sobject.SOParticipantConfig, error) {
	snapshot, err := getAdmittedState(ctx, e.so)
	if err != nil {
		return nil, err
	}
	return snapshot.GetParticipantConfigForPeer(ctx, e.so.GetPeerID().String())
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
// lock until Commit installs their World or Discard. A write forks from the
// World of the writes admitted before it, whose receipts may be pending. A
// write fails with the replay's error when an operation's block is not
// available, and holds no lock. A participant whose role cannot write
// operations, such as a reader, gets tx.ErrNotWrite instead of a write, since
// every replay would reject the operation it commits. Always call Discard when
// done.
func (e *soEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	// Read snapshots depend on the block engine until Discard. Write candidates
	// own independent forks and end with the controller's lifetime instead.
	if !write {
		ref, err := e.block.acquireTransaction()
		if err != nil {
			return nil, err
		}
		tx, err := e.bengine.NewBlockEngineTransaction(ctx, false)
		if err != nil {
			ref.Release()
			return nil, err
		}
		return world.NewRefCountTx(tx, ref), nil
	}

	// Time the write from the writer lock wait.
	clock := newCommitClock()

	// Serialize the write fork with other writes and replay.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/new-transaction")
	defer task.End()
	unlockWriteMtx, err := e.lockReplayed(ctx, clock)
	if err != nil {
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
	fork := &replayFork{base: base, index: index, start: head, state: head}
	clock.end(stageForkReplay)
	return newSoEngineWriteTx(ttx, btx, e, fork, unlockWriteMtx, clock), nil
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

// SyncThrough fences durable storage through seqno via the engine.
func (e *soEngine) SyncThrough(ctx context.Context, seqno uint64) (uint64, error) {
	return e.bengine.SyncThrough(ctx, seqno)
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

// WatchChanges calls cb with each later seqno and the changes since the
// previous one. Replay publishes every World head through the block engine, so
// its head comparison covers local commits and remote operations alike.
func (e *soEngine) WatchChanges(ctx context.Context, afterSeqno uint64, touches world.ChangeFilter, cb func(seqno uint64, changes *world.ChangeSet) error) error {
	return e.bengine.WatchChanges(ctx, afterSeqno, touches, cb)
}

// lockReplayed takes the writer lock and installs the World after every
// operation of the current set, which the watcher may not have replayed yet.
// A write replays even when this device follows the leader, since its own
// replay places it after every operation the set holds. Replay reads only
// local blocks, so when it stops at a block a peer must serve, lockReplayed
// releases the lock, fetches that block and tries again. It fails with
// tx.ErrNotWrite when the current config does not let this participant write
// operations. On success the caller holds the lock and releases it with the
// returned function.
func (e *soEngine) lockReplayed(ctx context.Context, clock *commitClock) (func(), error) {
	for {
		// Take the writer lock, tracing the hold until the write releases it.
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/new-transaction/lock-write-mtx")
		unlockWriteMtx, err := e.c.writeMtx.Lock(taskCtx, "write")
		task.End()
		if err != nil {
			return nil, err
		}
		clock.end(stageLockWait)
		_, holdWriteMtxTask := trace.NewTask(ctx, "alpha/so-engine/write-tx/hold-write-mtx")
		unlockWriteMtx = wrapReleaseWithTask(unlockWriteMtx, holdWriteMtxTask)

		// Replay and install the World of the current operation set.
		snapshot, err := e.admittedState(ctx)
		if err == nil {
			err = checkWritesOps(ctx, snapshot)
		}
		if err == nil {
			_, err = e.install(ctx, snapshot, nil, false)
		}
		if err == nil {
			return unlockWriteMtx, nil
		}
		missing := e.replay.missing
		unlockWriteMtx()
		if !block.IsNotAvailable(err) || missing == nil {
			return nil, err
		}

		// Fetch the block replay stopped at with a read that waits for a peer.
		if _, found, err := e.so.GetBlockStore().GetBlock(ctx, missing); err != nil {
			return nil, err
		} else if !found {
			return nil, errors.Wrap(block.ErrNotFound, missing.MarshalString())
		}
	}
}

// checkWritesOps returns tx.ErrNotWrite when the config of snap does not let
// this participant write operations.
func checkWritesOps(ctx context.Context, snap sobject.SharedObjectStateSnapshot) error {
	participant, err := snap.GetParticipantConfig(ctx)
	if err != nil {
		return err
	}
	if !sobject.CanWriteOps(participant.GetRole()) {
		return tx.ErrNotWrite
	}
	return nil
}

// advance replays snap, installs the World after its last placed operation
// and reports this device's rejected edits. A device that follows the leader
// installs the leader's checkpoint instead of replaying, unless fork is set.
// The caller holds the writer lock.
func (e *soEngine) advance(ctx context.Context, snap sobject.SharedObjectStateSnapshot, fork *replayFork) ([]replayOutcome, error) {
	return e.install(ctx, snap, fork, fork == nil)
}

// install reaches the World for snap, installs it and reports this device's
// rejected edits. follow lets a device that follows the leader install the
// leader's checkpoint instead of replaying. fork, when set, supplies the World
// after a local write. When replay stops at an operation whose block is not
// available, install installs the World before it and returns the replay's
// error, so the next advance resumes at that operation. It installs nothing
// when that World would lack an operation the installed World applied: the
// installed World stays until replay places every such operation. The caller
// holds the writer lock.
func (e *soEngine) install(ctx context.Context, snap sobject.SharedObjectStateSnapshot, fork *replayFork, follow bool) ([]replayOutcome, error) {
	// Reach and install the World.
	state, outcomes, err := e.reach(ctx, snap, fork, follow)
	if state != nil && block.IsNotAvailable(err) {
		if installErr := e.updateEngineState(ctx, state); installErr != nil {
			return nil, installErr
		}
		return nil, err
	}
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
	if err := e.queueStableCheckpoint(ctx, snap, set); err != nil {
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

// detach returns ctx's values without its cancellation, ended when the engine's
// lifetime ends. The caller must call cancel.
func (e *soEngine) detach(ctx context.Context) (context.Context, context.CancelFunc) {
	detached, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(e.lifetime, cancel)
	return detached, func() {
		stop()
		cancel()
	}
}

// queueOperation adds opData to the operation set as the local peer, replays
// the set and installs the World after it. fork, when set, supplies the World
// after the operation. It returns sobject.ErrRejectedOp wrapped with the
// reason when replay rejected the operation. Once the SharedObject admits the
// operation it is durable, so the reads and the replay that follow ignore ctx's
// cancellation and the result says whether the operation was placed. They end
// with the engine's lifetime, and the outcome is then unknown. An operation
// admitted to a publication is placed before its receipt resolves; the caller
// waits for the receipt. clock, when set, times the write: admission ends here
// unless a publication already ended it. The caller holds the writer lock.
func (e *soEngine) queueOperation(ctx context.Context, opData []byte, fork *replayFork, clock *commitClock) error {
	// Add the operation, naming it in the admission it joined, if any. A
	// failed publication still reports the id, which marks the snapshots
	// showing the operation as stale.
	admitted := len(e.admissions)
	localID, err := e.so.QueueOperation(ctx, opData)
	if len(e.admissions) > admitted {
		e.admissions[admitted].localID = localID
	}
	if err != nil {
		return err
	}
	clock.end(stagePublicationAdmission)

	// The operation is admitted, so finish placing it whatever ctx does.
	ctx, cancel := e.detach(ctx)
	defer cancel()

	// Find it in the set.
	snap, err := e.admittedState(ctx)
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

// updateEngineState installs a replayed World, then holds it, its retained
// roots and the replay that reached it in this participant's block store and
// saves the replay, in one commit when the store can publish. While an
// admission waits for its receipt it only installs the World: the pending
// publications hold its blocks, and the update after the last receipt settles
// holds and saves what changed. The caller holds the writer lock.
func (e *soEngine) updateEngineState(ctx context.Context, state *InnerState) error {
	// Trace the update.
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/update-engine-state")
	defer task.End()

	// Install a changed head once. The watcher, the next write and the
	// committing write all install the same head.
	ref := state.GetHeadRef().CloneVT()
	if ref == nil {
		ref = &bucket.ObjectRef{}
	}
	ref.BucketId = e.so.GetBlockStore().GetID()
	installed := !e.bengine.GetRootRef().EqualVT(ref)
	if installed {
		if err := e.bengine.SetRootRef(ctx, ref); err != nil {
			return err
		}
		e.c.notifyWrite()
	}

	// Hold nothing while the pending publications hold the World.
	if e.pending() {
		return nil
	}

	// Hold a changed head once; an admitted write already holds its World.
	hold := newRootHold(e.so.GetBlockStore())
	headChanged := !e.retained.EqualVT(ref)
	if headChanged {
		if err := hold.world(ctx, acceptedWorldRootName, ref.GetRootRef()); err != nil {
			return err
		}
	}

	// Hold changed retained roots and the Worlds the replay reads.
	rootsChanged := !slices.EqualFunc(state.GetRetainedRoots(), e.retainedRoots, (*RetainedRoot).EqualVT)
	if rootsChanged {
		if err := hold.rootSet(retainedRootsName, state.GetRetainedRoots()); err != nil {
			return err
		}
	}
	if err := e.replay.holdSpan(hold); err != nil {
		return err
	}
	if err := e.applyHold(ctx, hold); err != nil {
		return err
	}

	// Record what is held.
	if headChanged {
		e.retained = ref
	}
	if rootsChanged {
		e.retainedRoots = state.GetRetainedRoots()
	}
	return nil
}

// applyHold applies hold and saves the replay, in one publication when the
// store can publish, so an update commits to the volume once. Otherwise it
// applies hold, then saves the replay, so the save names held Worlds. The
// caller holds the writer lock and no admission is pending.
func (e *soEngine) applyHold(ctx context.Context, hold *rootHold) error {
	// Fall back when there is no replay save to carry or the state or the
	// store cannot join a publication.
	publisher, publishes := e.so.(sobject.StatePublisher)
	store, ok := e.so.GetBlockStore().(block.AtomicPublisher)
	if !e.replay.unsaved() || !publishes || !ok || !store.SupportsAtomicPublication() {
		return e.holdThenSave(ctx, hold)
	}

	// Submit the held blocks and roots with the replay cursor.
	cursor, err := e.replay.encodeCursor()
	if err != nil {
		return err
	}
	receipt, err := store.SubmitAtomic(ctx, &block.AtomicPublication{
		Entries: hold.entries,
		Heads:   []*block.AtomicHeadUpdate{publisher.LocalStateHead(replayCursorStoreID, replayCursorKey, cursor)},
		Roots:   hold.roots,
	})
	if errors.Is(err, block.ErrAtomicPublicationUnsupported) || errors.Is(err, block.ErrPublicationTooLarge) {
		return e.holdThenSave(ctx, hold)
	}
	if err != nil {
		return err
	}

	// Record the save once it is durable.
	err = receipt.Wait(ctx)
	receipt.Release()
	if err != nil {
		return err
	}
	e.replay.saved()
	return nil
}

// holdThenSave applies hold, then saves the replay now that its World is
// held.
func (e *soEngine) holdThenSave(ctx context.Context, hold *rootHold) error {
	if err := hold.apply(ctx); err != nil {
		return err
	}
	return e.replay.save(ctx)
}

// acceptedWorldRootName names the local root that holds the installed World.
const acceptedWorldRootName = "accepted-world"

// _ is a type assertion
var (
	_ Engine              = (*soEngine)(nil)
	_ world.ChangeWatcher = (*soEngine)(nil)
)
