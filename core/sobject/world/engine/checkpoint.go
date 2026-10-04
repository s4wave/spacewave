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
		snap = snap.WithConfigHistory(host.GetSOHost().ReadConfigEntry)
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

// ReplayWorld replays the operation set of snap from its checkpoint's World
// and returns the World after the last operation it can place. so supplies the
// blocks the operations reference and receives the blocks replay writes.
// Replay resolves operations as the World engine engineID does: with lookupOp
// first, then through the bus. It must resolve every operation the engine
// does, or replay rejects operations the engine applied.
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
	// Replay through a controller of the engine.
	c, err := NewController(le, b, &Config{EngineId: engineID}, sfs)
	if err != nil {
		return nil, err
	}
	c.SetStaticLookupOp(lookupOp)
	replayed, _, err := newReplayer(c, so).sync(ctx, snap, nil)
	return replayed, err
}

// errStableCheckpointStale reports that the held state moved past the stable
// point a checkpoint was prepared for.
var errStableCheckpointStale = errors.New("held state moved past the stable point")

// checkpointStable signs and adopts a checkpoint at the stable point when the
// local peer is the checkpointer and at least MinCheckpointOperations
// operations are stable. The World after the stable prefix comes from the
// replay, so nothing replays again. Under group control the voters decide the
// checkpoint instead, so the replay only wakes the local voter.
func (e *soEngine) checkpointStable(ctx context.Context, snap sobject.SharedObjectStateSnapshot, set *sobject.SOOperationSet) error {
	// Only the checkpointer signs, and only from a host it can sign through.
	host, ok := e.so.(sobject.StateHost)
	if !ok {
		return nil
	}
	cfg, err := snap.GetConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.IsGroupControl() {
		if e.control != nil {
			e.control.Wake()
		}
		return nil
	}
	self := e.so.GetPeerID().String()
	if cfg.Checkpointer() != self {
		return nil
	}

	// Wait for enough stable operations whose World the replay holds.
	roster := cfg.TrimRoster()
	prefix := set.StablePoint(roster)
	if len(prefix) < sobject.MinCheckpointOperations {
		return nil
	}
	world := e.replay.stateAfter(prefix)
	if world == nil {
		return nil
	}
	data, err := world.MarshalVT()
	if err != nil {
		return err
	}

	// Sign and adopt it under the host lock, unless the held state moved.
	err = host.GetSOHost().UpdateSOState(ctx, func(state *sobject.SOState) error {
		return e.adoptStableCheckpoint(ctx, host, state, prefix, data)
	})
	if errors.Is(err, errStableCheckpointStale) {
		return nil
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
	return sobject.NewSOStateParticipantHandle(e.c.le, e.c.sfs, e.so.GetSharedObjectID(), state, host.GetPrivKey(), e.so.GetPeerID())
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
	unlock, err := e.c.writeMtx.Lock(ctx)
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
	unlock, err := e.c.writeMtx.Lock(ctx)
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
