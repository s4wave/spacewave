package sobject_world_engine

import (
	"bytes"
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CheckpointWorld replays the operation set of state as the World engine
// engineID, resolving operations with lookupOp, and returns state advanced to
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
	lookupOp world.LookupOp,
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
	replayed, err := ReplayWorld(ctx, le, b, sfs, so, engineID, lookupOp, snap)
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
// operations as the World engine engineID does: with lookupOp first, then
// through the bus. It must resolve every operation the engine does, or replay
// rejects operations the engine applied.
func NewWorldReplay(
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	engineID string,
	lookupOp world.LookupOp,
) (*WorldReplay, error) {
	// Replay through a controller of the engine.
	c, err := NewController(le, b, &Config{EngineId: engineID}, sfs)
	if err != nil {
		return nil, err
	}
	c.SetStaticLookupOp(lookupOp)
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
	lookupOp world.LookupOp,
	snap sobject.SharedObjectStateSnapshot,
) (*InnerState, error) {
	replay, err := NewWorldReplay(le, b, sfs, so, engineID, lookupOp)
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
	if e.stableNext.Swap(ckpt) != nil {
		countCheckpoint(outcomeSuperseded)
	}
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
		if err := e.adoptStable(e.lifetime, ckpt); err != nil && e.lifetime.Err() == nil {
			countCheckpoint(outcomeFailed)
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
		skipCheckpoint(skipNotHost)
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
		skipCheckpoint(skipGroupControl)
		return nil, nil
	}
	self := e.so.GetPeerID().String()
	if cfg.Checkpointer() != self {
		skipCheckpoint(skipNotCheckpointer)
		return nil, nil
	}

	// An owner handing over never signs again; its successor checkpoints once
	// it commits the departure.
	own, err := e.ownershipOf(ctx, snap)
	if err != nil {
		return nil, err
	}
	if own.Departing(self) {
		skipCheckpoint(skipDeparting)
		return nil, nil
	}

	// Skip while an admission waits for its receipt: the checkpoint writes
	// the state directly, and the update after the last receipt settles
	// checkpoints again.
	if e.pending() {
		skipCheckpoint(skipReceiptPending)
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
		skipCheckpoint(skipFewStable)
		return nil, nil
	}

	// Encode the World after the stable prefix.
	world := e.replay.stateAfter(prefix)
	if world == nil {
		skipCheckpoint(skipNoWorld)
		return nil, nil
	}
	data, err := world.MarshalVT()
	if err != nil {
		return nil, err
	}
	return &stableCheckpoint{host: host, prefix: prefix, data: data}, nil
}

// adoptStable signs and adopts ckpt under the host lock, unless the held state
// moved past its stable point. The host lock writes after any pending receipt,
// on the durable state.
func (e *soEngine) adoptStable(ctx context.Context, ckpt *stableCheckpoint) error {
	err := ckpt.host.GetSOHost().UpdateSOState(ctx, func(state *sobject.SOState) error {
		return e.adoptStableCheckpoint(ctx, ckpt.host, state, ckpt.prefix, ckpt.data)
	})
	switch {
	case errors.Is(err, errStableCheckpointStale):
		countCheckpoint(outcomeStale)
		return nil
	case err == nil:
		countCheckpoint(outcomeWritten)
	}
	return err
}

// adoptStableCheckpoint signs, as host, the checkpoint covering prefix with the
// World data after it, and adopts it into state. Returns
// errStableCheckpointStale when prefix is no longer stable in state.
func (e *soEngine) adoptStableCheckpoint(
	ctx context.Context,
	host sobject.StateHost,
	state *sobject.SOState,
	prefix [][]byte,
	data []byte,
) error {
	// Check prefix is still stable.
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
	unlock, err := e.c.writeMtx.Lock(ctx, "propose-checkpoint")
	if err != nil {
		return nil, err
	}
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

	// Encrypt the World into the checkpoint body.
	data, err := world.MarshalVT()
	if err != nil {
		return nil, err
	}
	dataEnc, err := e.encodeCheckpointData(ctx, host, state, data)
	if err != nil {
		return nil, err
	}
	return state.StableCheckpointInner(soID, prefix, dataEnc)
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
