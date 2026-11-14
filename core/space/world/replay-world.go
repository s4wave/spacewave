package space_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/world"
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
	a.waitBackground()
	if err != nil {
		release()
		return nil, nil, err
	}
	return a, release, nil
}

// ErrUnresolvedOp is returned by ImportRoots when the type of an operation
// above the checkpoint does not resolve, such as a plugin's before the plugin
// registers, so its payloads cannot all be named.
var ErrUnresolvedOp = errors.New("an operation's type does not resolve")

// ImportRoots returns the roots a replay of snap reads, so a copy of them lets
// another device replay the state without this one: the World its checkpoint
// carries, if any, then the payloads of the operations above it, resolving
// operation types with lookupOp. It fails with ErrUnresolvedOp rather than
// leave out a payload.
func ImportRoots(ctx context.Context, snap sobject.SharedObjectStateSnapshot, lookupOp world.LookupOp) ([]*block.BlockRef, error) {
	roots, unresolved, err := snapRoots(ctx, snap, lookupOp)
	if err == nil && unresolved {
		err = ErrUnresolvedOp
	}
	return roots, err
}

// PublishedGraph returns one root reaching every block a replay of snap reads:
// the World its checkpoint carries and the payloads of the operations above
// it, resolving operation types with lookupOp. With payloads, the root is a new
// HeldRoots block, returned as entry for the caller to store; otherwise it is
// the World root and entry is nil. An operation this participant cannot decode
// or its config does not authorize applies nothing and has no payload.
func PublishedGraph(ctx context.Context, snap sobject.SharedObjectStateSnapshot, lookupOp world.LookupOp) (*block.PutBatchEntry, *block.BlockRef, error) {
	// Name the roots, then return the only one or a block naming them all.
	roots, _, err := snapRoots(ctx, snap, lookupOp)
	if err != nil {
		return nil, nil, err
	}
	switch len(roots) {
	case 0:
		return nil, nil, nil
	case 1:
		return nil, roots[0], nil
	}
	entry, err := refsEntry(&HeldRoots{Roots: roots}, roots)
	if err != nil {
		return nil, nil, err
	}
	return entry, entry.Ref, nil
}

// snapRoots returns the World of the checkpoint of snap, if any, then the
// payload roots of the operations above it. unresolved reports an operation
// whose type lookupOp did not resolve, whose payloads roots holds only in part.
func snapRoots(ctx context.Context, snap sobject.SharedObjectStateSnapshot, lookupOp world.LookupOp) (roots []*block.BlockRef, unresolved bool, err error) {
	// Decode the checkpoint's World.
	checkpoint, err := snap.GetCheckpoint(ctx)
	if err != nil {
		return nil, false, err
	}
	head := &InnerState{}
	if err := head.UnmarshalVT(checkpoint.GetStateData()); err != nil {
		return nil, false, errors.Wrap(err, "checkpoint World state")
	}
	if worldRoot := head.GetHeadRef().GetRootRef(); !worldRoot.GetEmpty() {
		roots = append(roots, worldRoot)
	}

	// Collect the payload roots of the operations above it.
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, h := range set.Order() {
		refs, known, err := opPayloads(ctx, snap, set, h, lookupOp)
		if err != nil {
			return nil, false, err
		}
		roots = append(roots, refs...)
		unresolved = unresolved || !known
	}
	return roots, unresolved, nil
}

// opPayloads returns the payload roots of operation h of set, read from snap,
// resolving operation types with lookupOp. An acknowledgment, an equivocated
// operation, one its config does not authorize, and one that does not decode
// apply nothing and have none. known is false when only some payloads could be
// read yet: lookupOp failed for the operation's type, which a plugin may
// register later, and refs holds the roots read without it, the object roots
// the operation set. err is a failure to read snap.
func opPayloads(ctx context.Context, snap sobject.SharedObjectStateSnapshot, set *sobject.SOOperationSet, h []byte, lookupOp world.LookupOp) (refs []*block.BlockRef, known bool, err error) {
	// Skip the operations that apply nothing.
	inner := set.Get(h)
	if inner.IsAcknowledgment() || set.Equivocated(h) {
		return nil, true, nil
	}
	_, opData, reason, err := sobject.PrepareReplayOp(ctx, snap, inner)
	if err != nil {
		return nil, false, err
	}
	op := &WorldOp{}
	if reason != "" || op.UnmarshalVT(opData) != nil || op.GetApplyTxOp().GetTx() == nil {
		return nil, true, nil
	}

	// Read the roots the transaction names.
	refs, err = op.GetApplyTxOp().GetTx().PayloadRefs(ctx, lookupOp)
	return refs, err == nil, nil
}
