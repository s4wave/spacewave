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
	if host, ok := so.(sobject.InviteHost); ok {
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
// replay, so nothing replays again.
func (e *soEngine) checkpointStable(ctx context.Context, snap sobject.SharedObjectStateSnapshot, set *sobject.SOOperationSet) error {
	// Only the checkpointer signs, and only from a host it can sign through.
	host, ok := e.so.(sobject.InviteHost)
	if !ok {
		return nil
	}
	cfg, err := snap.GetConfig(ctx)
	if err != nil {
		return err
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
	host sobject.InviteHost,
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
	handle := sobject.NewSOStateParticipantHandle(e.c.le, e.c.sfs, soID, state, host.GetPrivKey(), e.so.GetPeerID())
	xfrm, err := handle.GetTransformer(ctx)
	if err != nil {
		return err
	}
	dataEnc, err := xfrm.EncodeBlock(data)
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
