package sobject_world_engine

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CheckpointWorld replays the operation set of state as the World engine
// engineID, resolving operations with opTypes, and returns state advanced to
// the checkpoint, signed as the owner of privKey, whose World is the replayed
// World. The returned state holds no operations. Whole-state
// copies such as transfer and publication carry it, so the receiver needs
// only the blocks of that World.
func CheckpointWorld(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	engineID string,
	opTypes OpTypes,
	privKey crypto.PrivKey,
	state *sobject.SOState,
) (*sobject.SOState, *InnerState, error) {
	// Replay the operation set from the checkpoint's World.
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, nil, err
	}
	sharedObjectID := so.GetSharedObjectID()
	snap := sobject.NewSOStateParticipantHandle(le, sfs, sharedObjectID, state, privKey, peerID)
	if host, ok := so.(sobject.StateHost); ok {
		snap = snap.WithConfigHistory(host.GetSOHost().ReadConfigEntry).WithOperationVerifier(host.GetSOHost().GetOperationVerifier()).WithGrantKeyring(host.GetSOHost().GetGrantKeyring())
	}
	replayed, err := ReplayWorld(ctx, le, b, sfs, so, engineID, opTypes, snap)
	if err != nil {
		return nil, nil, err
	}

	// Encrypt the World with the current key and sign the checkpoint.
	data, err := replayed.MarshalVT()
	if err != nil {
		return nil, nil, err
	}
	xfrm, err := snap.GetTransformer(ctx)
	if err != nil {
		return nil, nil, err
	}
	dataEnc, err := xfrm.EncodeBlock(data)
	if err != nil {
		return nil, nil, err
	}
	checkpoint, err := state.BuildNextCheckpoint(sharedObjectID, privKey, dataEnc)
	if err != nil {
		return nil, nil, err
	}

	// Adopt it, which drops every covered operation.
	next := state.CloneVT()
	if err := next.AdoptCheckpoint(sharedObjectID, checkpoint); err != nil {
		return nil, nil, err
	}
	return next, replayed, nil
}

// WorldReplay replays the operation sets of successive states of one
// SharedObject World. Each replay resumes after the longest prefix its order
// shares with the previous replay, so a caller following the state replays
// only the operations each state adds. so supplies the blocks the operations
// reference and receives the blocks replay writes. The owner serializes calls.
type WorldReplay struct {
	// r is the replayer.
	r *replayer
}

// NewWorldReplay constructs a replay of the World of so. Replay resolves
// operations as the World engine engineID does: with opTypes first, then
// through the bus. It must resolve every operation the engine does, or replay
// rejects operations the engine applied.
func NewWorldReplay(
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	engineID string,
	opTypes OpTypes,
) (*WorldReplay, error) {
	// Replay through a controller of the engine.
	c, err := NewController(le, b, &Config{EngineId: engineID}, sfs)
	if err != nil {
		return nil, err
	}
	c.SetOpTypes(opTypes)
	return &WorldReplay{r: newReplayer(c, so)}, nil
}

// Resume restores the replay the World engine last saved on this device, so
// the next Replay starts after the longest prefix it shares instead of at the
// checkpoint. The replay never saves; the engine keeps the saved replay and
// the blocks it reaches.
func (w *WorldReplay) Resume(ctx context.Context) error {
	return w.r.load(ctx)
}

// Replay replays the operation set of snap from its checkpoint's World and
// returns the World after the last operation it can place.
func (w *WorldReplay) Replay(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (*InnerState, error) {
	replayed, _, err := w.r.sync(ctx, snap, nil)
	return replayed, err
}

// ReplayWorld replays the operation set of snap once, as a new WorldReplay
// does.
func ReplayWorld(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	engineID string,
	opTypes OpTypes,
	snap sobject.SharedObjectStateSnapshot,
) (*InnerState, error) {
	replay, err := NewWorldReplay(le, b, sfs, so, engineID, opTypes)
	if err != nil {
		return nil, err
	}
	return replay.Replay(ctx, snap)
}

// errStableCheckpointStale reports that the held state moved past the stable
// point a checkpoint was prepared for.
var errStableCheckpointStale = errors.New("held state moved past the stable point")

// stableCheckpoint is a checkpoint at the stable point, prepared under the
// writer lock and adopted after it.
type stableCheckpoint struct {
	host   sobject.StateHost
	prefix [][]byte
	data   []byte
	// height is the height of the checkpoint it follows.
	height uint64
	// named resolves once the store holds the World whole, or is nil when it
	// already does.
	named *block.PublicationReceipt
}

// nameWorld names world, the World after the first n positions, under the
// replay base root with every kept page it reaches, in a publication after the
// admitted ones, so the store holds it whole before a checkpoint above height
// adopts it. It returns the receipt that resolves once the store holds it, or
// nil when it already does. ok is false when the World is not readable. The
// receipt is only waited on. The caller holds the writer lock.
//
// The interval since the last named World waits for the publication's puts
// before its deadlist record is encoded. A page of the tree that interval
// began at, written back to its old ref, is put counted: the store already
// counts its holders.
func (e *soEngine) nameWorld(ctx context.Context, world *InnerState, n int, height uint64) (*block.PublicationReceipt, bool, error) {
	// Join the interval since the last named World. Without the page view
	// every World is stored.
	r := e.replay
	if r.pages != nil && !r.readable(n) {
		return nil, false, nil
	}
	root := world.GetHeadRef().GetRootRef()
	net := r.composeDeaths(root)
	if r.pages == nil {
		return nil, true, nil
	}

	// Seal the interval with the naming publication's puts, or none when it
	// is not submitted.
	var named *worldPut
	if net != nil {
		defer func() { net.seal(named) }()
	}

	// A World a save carried is named once that save lands.
	if !r.baseUnheld && slices.ContainsFunc(r.baseRoots, root.EqualsRef) {
		return e.lastReceipt(), true, nil
	}

	// Gather it with the base hold and the replay save.
	r.pendingWorlds = append(r.pendingWorlds, pendingWorld{root: root, height: height})
	if err := r.holdBase(ctx); err != nil {
		return nil, false, err
	}

	// Collect the pending save.
	entries, roots, cursor, err := r.pendingSave(ctx, e.so.(sobject.StatePublisher), nil)
	if err != nil {
		return nil, false, err
	}

	// Read the seqno of the World writers fork from, which the admission
	// records, before submitting, so a submitted publication is admitted.
	seqno, err := e.admitted.GetSeqno(ctx)
	if err != nil {
		return nil, false, err
	}

	// Publish it after the admitted publications, splitting the kept pages
	// from the save first: a receipt may store and drop them meanwhile. Mark
	// the original pages on copies, leaving the kept pages unchanged.
	pages, save := r.pages.split(entries)
	if net != nil {
		net.steps.MarkOriginal(entries)
	}
	receipt, err := r.pages.SubmitAtomic(ctx, &block.AtomicPublication{
		Entries: entries,
		Heads:   []*block.AtomicHeadUpdate{countHead(cursor, WriteKindReplay)},
		Roots:   roots,
		After:   e.lastReceipt(),
	})
	if err != nil {
		return nil, false, err
	}
	countPublication(nil, pages, save)

	// Hand the naming put to the interval's record.
	put := newWorldPut(receipt, entries, pages)
	named = &put
	e.admit(receipt, e.retained, seqno)
	go receipt.Release()
	return receipt, true, nil
}

// waitNamed waits for receipt, from nameWorld, and returns the publication's
// error.
func waitNamed(ctx context.Context, receipt *block.PublicationReceipt) error {
	if receipt == nil {
		return nil
	}
	if err := receipt.Wait(ctx); err != nil {
		return err
	}
	return receipt.Err()
}

// queueStableCheckpoint prepares the checkpoint at the stable point and hands
// it to the checkpoint routine, which signs and adopts it without the writer
// lock: the state write can wait on the store for as long as a commit takes.
// The caller holds the writer lock.
func (e *soEngine) queueStableCheckpoint(ctx context.Context, snap sobject.SharedObjectStateSnapshot, set *sobject.SOOperationSet) error {
	// Prepare the checkpoint while the replay is held.
	ckpt, err := e.prepareStableCheckpoint(ctx, snap, set)
	if err != nil || ckpt == nil {
		return err
	}

	// Replace any checkpoint the routine has not taken and wake it.
	e.stableNext.Store(ckpt)
	select {
	case e.stableWake <- struct{}{}:
	default:
	}
	return nil
}

// executeStableCheckpoints adopts the latest prepared checkpoint each time one
// is queued, until the engine's lifetime ends.
func (e *soEngine) executeStableCheckpoints() {
	for {
		select {
		case <-e.lifetime.Done():
			return
		case <-e.stableWake:
		}
		ckpt := e.stableNext.Swap(nil)
		if ckpt == nil {
			continue
		}
		err := e.adoptStable(e.lifetime, ckpt)
		e.stableBusy.Store(false)
		if err != nil && e.lifetime.Err() == nil {
			e.c.le.WithError(err).Warn("failed to checkpoint the stable point")
		}
	}
}

// prepareStableCheckpoint returns the checkpoint at the stable point when the
// local peer is the checkpointer and at least MinCheckpointOperations
// operations are stable, or any operation is when the local peer leads, or nil.
// The World after the stable prefix comes from the replay, so nothing replays
// again. Under group control the voters decide the checkpoint instead, so the
// replay only wakes the local voter. The caller holds the writer lock.
func (e *soEngine) prepareStableCheckpoint(ctx context.Context, snap sobject.SharedObjectStateSnapshot, set *sobject.SOOperationSet) (*stableCheckpoint, error) {
	// Only the checkpointer signs, and only from a host it can sign through.
	host, ok := e.so.(sobject.StateHost)
	if !ok {
		return nil, nil
	}
	cfg, err := snap.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.IsGroupControl() {
		if e.control != nil {
			e.control.Wake()
		}
		return nil, nil
	}
	self := e.so.GetPeerID().String()
	if cfg.Checkpointer() != self {
		return nil, nil
	}

	// An owner handing over never signs again; its successor checkpoints once
	// it commits the departure.
	own, err := e.ownershipOf(ctx, snap)
	if err != nil {
		return nil, err
	}
	if own.Departing(self) {
		return nil, nil
	}

	// Wait for enough stable operations whose World the replay holds. The
	// leader publishes every ordered batch, since the other participants
	// install its checkpoints instead of replaying.
	minStable := sobject.MinCheckpointOperations
	if cfg.Leader() == self {
		minStable = 1
	}
	prefix := set.StablePoint(cfg.TrimRoster())
	if len(prefix) < minStable {
		return nil, nil
	}

	// Skip while an earlier checkpoint is in flight. Without the page view
	// skip while an admission waits for its receipt too: the checkpoint
	// writes the state directly, naming a World only the pending publications
	// hold, and the update after the last receipt settles checkpoints again.
	// With it the checkpoint names its World in a publication after them.
	if e.stableBusy.Load() || (e.replay.pages == nil && e.pending()) {
		countSkip(CheckpointSkipPending)
		return nil, nil
	}

	// Find the World after the stable prefix.
	world := e.replay.stateAfter(prefix)
	if world == nil {
		countSkip(CheckpointSkipUnplaced)
		return nil, nil
	}

	// Name it for the checkpoint above the held one, then encode it.
	held, err := snap.GetCheckpoint(ctx)
	if err != nil {
		return nil, err
	}
	named, ok, err := e.nameWorld(ctx, world, len(prefix), held.GetHeight())
	if err != nil {
		return nil, err
	}
	if !ok {
		countSkip(CheckpointSkipUnplaced)
		return nil, nil
	}

	// Encode it, holding later checkpoints until this one is adopted.
	data, err := world.MarshalVT()
	if err != nil {
		return nil, err
	}
	if e.replay.pages != nil {
		e.stableBusy.Store(true)
	}
	return &stableCheckpoint{host: host, prefix: prefix, data: data, height: held.GetHeight(), named: named}, nil
}

// adoptStable signs and adopts ckpt under the host lock once the store holds
// its World, unless the held state moved past its stable point or its
// checkpoint. The host lock writes after any pending receipt, on the durable
// state.
//
// The state write commits ordered, releasing the host lock before the store
// flushes it, since World writes queue their operations under that lock. A
// crash before the flush loses only the checkpoint: the state before it holds
// the same operations, and the next stable point checkpoints again. Every
// path that sends the state off the machine waits for SOHost.WaitDurable
// first, so no peer sees a checkpoint a crash could undo.
func (e *soEngine) adoptStable(ctx context.Context, ckpt *stableCheckpoint) error {
	// Wait for the store to hold the World.
	if err := waitNamed(ctx, ckpt.named); err != nil {
		return err
	}

	// Adopt it with an ordered state write, counting a stale checkpoint as
	// skipped.
	ctx = kvtx.WithOrderedCommitContext(ctx)
	err := ckpt.host.GetSOHost().UpdateSOState(ctx, func(state *sobject.SOState) error {
		return e.adoptStableCheckpoint(ctx, ckpt.host, state, ckpt.prefix, ckpt.data, ckpt.height)
	})
	if errors.Is(err, errStableCheckpointStale) {
		countSkip(CheckpointSkipStale)
		return nil
	}
	if err == nil {
		writeFlows.checkpoints.Add(1)
	}
	return err
}

// adoptStableCheckpoint signs, as host, the checkpoint covering prefix with the
// World data after it, and adopts it into state. Returns
// errStableCheckpointStale when state no longer holds the checkpoint at height
// or prefix is no longer stable in it: the replay base hold names the World
// only until the checkpoint moves past height.
func (e *soEngine) adoptStableCheckpoint(
	ctx context.Context,
	host sobject.StateHost,
	state *sobject.SOState,
	prefix [][]byte,
	data []byte,
	height uint64,
) error {
	// Check the held checkpoint is unchanged and prefix is still stable.
	current, err := state.GetCheckpointInner()
	if err != nil {
		return err
	}
	if current.GetHeight() != height {
		return errStableCheckpointStale
	}
	soID := e.so.GetSharedObjectID()
	held, err := state.OperationSet(soID)
	if err != nil {
		return err
	}
	stable := held.StablePoint(state.GetConfig().TrimRoster())
	if len(stable) < len(prefix) || !slices.EqualFunc(stable[:len(prefix)], prefix, bytes.Equal) {
		return errStableCheckpointStale
	}

	// Encrypt the World with the current key epoch.
	dataEnc, err := e.encodeCheckpointData(ctx, host, state, data)
	if err != nil {
		return err
	}

	// Sign and adopt the checkpoint.
	checkpoint, err := state.BuildStableCheckpoint(soID, host.GetPrivKey(), prefix, dataEnc)
	if err != nil {
		return err
	}
	return state.AdoptCheckpoint(soID, checkpoint)
}

// encodeCheckpointData encrypts World data with the current key epoch of
// state.
func (e *soEngine) encodeCheckpointData(ctx context.Context, host sobject.StateHost, state *sobject.SOState, data []byte) ([]byte, error) {
	xfrm, err := e.participant(host, state).GetTransformer(ctx)
	if err != nil {
		return nil, err
	}
	return xfrm.EncodeBlock(data)
}

// participant returns the local participant's view of state.
func (e *soEngine) participant(host sobject.StateHost, state *sobject.SOState) *sobject.SOStateParticipantHandle {
	return sobject.NewSOStateParticipantHandle(e.c.le, e.c.sfs, e.so.GetSharedObjectID(), state, host.GetPrivKey(), e.so.GetPeerID()).
		WithGrantKeyring(host.GetSOHost().GetGrantKeyring())
}

// ProposeCheckpoint returns the checkpoint after the one state holds that
// covers its stable point, with the World the replay reached there, or nil
// while fewer than MinCheckpointOperations operations are stable or the replay
// has not placed them.
func (e *soEngine) ProposeCheckpoint(ctx context.Context, state *sobject.SOState) (*sobject.SOCheckpointInner, error) {
	// Only a host that can decrypt proposes, under the write lock.
	host, ok := e.so.(sobject.StateHost)
	if !ok {
		return nil, nil
	}
	unlockWriteMtx, err := e.c.writeMtx.Lock(ctx, "propose-checkpoint")
	if err != nil {
		return nil, err
	}
	unlock := sync.OnceFunc(unlockWriteMtx)
	defer unlock()

	// Find the stable prefix and the World after it.
	soID := e.so.GetSharedObjectID()
	set, err := state.OperationSet(soID)
	if err != nil {
		return nil, err
	}
	prefix := set.StablePoint(state.GetConfig().TrimRoster())
	if len(prefix) < sobject.MinCheckpointOperations {
		return nil, nil
	}
	world := e.replay.stateAfter(prefix)
	if world == nil {
		return nil, nil
	}

	// Name the World for the checkpoint above the held one.
	held, err := state.GetCheckpointInner()
	if err != nil {
		return nil, err
	}
	named, ok, err := e.nameWorld(ctx, world, len(prefix), held.GetHeight())
	if err != nil || !ok {
		return nil, err
	}

	// Encrypt the World into the checkpoint body.
	data, err := world.MarshalVT()
	if err != nil {
		return nil, err
	}
	dataEnc, err := e.encodeCheckpointData(ctx, host, state, data)
	if err != nil {
		return nil, err
	}
	inner, err := state.StableCheckpointInner(soID, prefix, dataEnc)
	if err != nil {
		return nil, err
	}

	// Propose it once the store holds its World, without the write lock.
	unlock()
	if err := waitNamed(ctx, named); err != nil {
		return nil, err
	}
	return inner, nil
}

// JudgeCheckpoint reports whether inner is the checkpoint after the one state
// holds that covers a stable prefix of its operations in the order the replay
// placed them, with the World the replay reached after them. The encryption
// differs between proposers, so the decrypted World is compared. ok is false
// while the replay has not placed the covered operations first or they are
// not yet stable.
func (e *soEngine) JudgeCheckpoint(ctx context.Context, state *sobject.SOState, inner *sobject.SOCheckpointInner) (bool, bool, error) {
	// Only a host that can decrypt judges, under the write lock.
	host, ok := e.so.(sobject.StateHost)
	if !ok {
		return false, false, nil
	}
	unlock, err := e.c.writeMtx.Lock(ctx, "judge-checkpoint")
	if err != nil {
		return false, false, err
	}
	defer unlock()

	// Find the covered operations in replay order and the World after them.
	world, prefix, ok := e.replay.coveredWorld(inner)
	if !ok || len(prefix) == 0 {
		return false, false, nil
	}

	// The covered operations must be stable here.
	soID := e.so.GetSharedObjectID()
	set, err := state.OperationSet(soID)
	if err != nil {
		return false, false, err
	}
	stable := set.StablePoint(state.GetConfig().TrimRoster())
	if len(stable) < len(prefix) || !slices.EqualFunc(stable[:len(prefix)], prefix, bytes.Equal) {
		return false, false, nil
	}

	// Compare the body this voter would build around the same state data.
	expected, err := state.StableCheckpointInner(soID, prefix, inner.GetStateData())
	if err != nil {
		return false, false, nil
	}
	if !expected.EqualVT(inner) {
		return false, true, nil
	}

	// Compare the World it carries.
	decoded, err := e.participant(host, state).DecodeCheckpoint(inner)
	if err != nil {
		return false, true, nil
	}
	got := &InnerState{}
	if err := got.UnmarshalVT(decoded.GetStateData()); err != nil {
		return false, true, nil
	}
	return got.EqualVT(world), true, nil
}

// _ is a type assertion
var _ sobject.CheckpointJudge = (*soEngine)(nil)
