package space_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// ReplayWorld replays the operation set of snap as the Space World engine
// engineID, and returns the World after it. The pages it computes land in a
// stage on so's block store, which holds them until release, so the caller
// copies the World before it releases.
func ReplayWorld(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	engineID string,
	opTypes OpTypes,
	snap sobject.SharedObjectStateSnapshot,
) (*InnerState, func(), error) {
	a, release, err := replayOnce(ctx, le, b, sfs, so, engineID, opTypes, snap, 0)
	if err != nil {
		return nil, nil, err
	}
	state, _ := a.replay.head()
	return state, release, nil
}

// replayOnce replays snap on a transient adapter whose pages stay in memory
// up to maxPending bytes and go to a stage on so's block store past it. The
// stage holds them until release. It waits for the trims its installs
// start.
func replayOnce(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	engineID string,
	opTypes OpTypes,
	snap sobject.SharedObjectStateSnapshot,
	maxPending int,
) (*Adapter, func(), error) {
	// Hold what the replay writes under a stage.
	staged, release, err := block.OpenStage(ctx, so.GetBlockStore())
	if err != nil {
		return nil, nil, err
	}

	// Replay through a transient adapter over it.
	a, err := NewAdapter(le, so, AdapterOptions{
		Bus:             b,
		StepFactorySet:  sfs,
		EngineID:        engineID,
		OpTypes:         opTypes,
		MaxPendingBytes: maxPending,
		transient:       true,
		pagesStore:      staged,
	})
	if err != nil {
		release()
		return nil, nil, err
	}
	defer a.replay.close()
	err = sobject.NewBodyDriver(le, so, a, nil).Step(ctx, snap)
	a.background.Wait()
	if err != nil {
		release()
		return nil, nil, err
	}
	return a, release, nil
}

// CheckpointWorld replays the operation set of state as the Space World engine
// engineID, and returns state advanced to the checkpoint over every held
// operation, signed as the owner of privKey, whose World is the replayed one,
// with the World. The returned state holds no operations, so a whole-state copy
// such as a transfer carries only the blocks of that World. The World's pages
// stay held until release.
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
) (*sobject.SOState, *InnerState, func(), error) {
	// Replay the state as its owner.
	soID := so.GetSharedObjectID()
	snap := sobject.NewSOStateParticipantHandle(le, sfs, soID, state, privKey, so.GetPeerID())
	if host, ok := so.(sobject.InviteHost); ok {
		soHost := host.GetSOHost()
		snap = snap.WithConfigHistory(soHost.ReadConfigEntry).WithOperationVerifier(soHost.GetOperationVerifier()).WithGrantKeyring(soHost.GetGrantKeyring())
	}
	replayed, release, err := ReplayWorld(ctx, le, b, sfs, so, engineID, opTypes, snap)
	if err != nil {
		return nil, nil, nil, err
	}
	held := false
	defer func() {
		if !held {
			release()
		}
	}()

	// Encrypt the World with the current key and sign the checkpoint.
	data, err := replayed.MarshalVT()
	if err != nil {
		return nil, nil, nil, err
	}
	xfrm, err := snap.GetTransformer(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	dataEnc, err := xfrm.EncodeBlock(data)
	if err != nil {
		return nil, nil, nil, err
	}
	checkpoint, err := state.BuildNextCheckpoint(soID, privKey, dataEnc)
	if err != nil {
		return nil, nil, nil, err
	}

	// Adopt it, which drops every covered operation.
	next := state.CloneVT()
	if err := next.AdoptCheckpoint(soID, checkpoint); err != nil {
		return nil, nil, nil, err
	}
	held = true
	return next, replayed, release, nil
}

// PublishedGraph returns one root reaching every block a replay of snap reads:
// the World its checkpoint carries and the payloads of the operations above
// it, resolving operation types with lookupOp. With payloads, the root is a new
// HeldRoots block, returned as entry for the caller to store; otherwise it is
// the World root and entry is nil. An operation this participant cannot decode
// or its config does not authorize applies nothing and has no payload.
func PublishedGraph(ctx context.Context, snap sobject.SharedObjectStateSnapshot, lookupOp world.LookupOp) (*block.PutBatchEntry, *block.BlockRef, error) {
	// Decode the checkpoint's World.
	checkpoint, err := snap.GetCheckpoint(ctx)
	if err != nil {
		return nil, nil, err
	}
	head := &InnerState{}
	if err := head.UnmarshalVT(checkpoint.GetStateData()); err != nil {
		return nil, nil, errors.Wrap(err, "checkpoint World state")
	}
	worldRoot := head.GetHeadRef().GetRootRef()

	// Collect the payload roots of the operations above it.
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, nil, err
	}
	var payloads []*block.BlockRef
	for _, h := range set.Order() {
		inner := set.Get(h)
		if inner.IsAcknowledgment() || set.Equivocated(h) {
			continue
		}
		_, opData, reason, err := sobject.PrepareReplayOp(ctx, snap, inner)
		if err != nil {
			return nil, nil, err
		}
		op := &WorldOp{}
		if reason != "" || op.UnmarshalVT(opData) != nil || op.GetApplyTxOp().GetTx() == nil {
			continue
		}
		refs, err := op.GetApplyTxOp().GetTx().PayloadRefs(ctx, lookupOp)
		if err != nil {
			continue
		}
		payloads = append(payloads, refs...)
	}
	if len(payloads) == 0 {
		return nil, worldRoot, nil
	}

	// Reference the World and the payloads from one block.
	roots := payloads
	if !worldRoot.GetEmpty() {
		roots = append([]*block.BlockRef{worldRoot}, payloads...)
	}
	entry, err := refsEntry(&HeldRoots{Roots: roots}, roots)
	if err != nil {
		return nil, nil, err
	}
	return entry, entry.Ref, nil
}
