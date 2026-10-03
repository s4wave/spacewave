package sobject_world_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
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
