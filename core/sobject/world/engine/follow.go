package sobject_world_engine

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// reach returns the World this device installs for snap, with the outcome of
// every operation it replayed: the leader's checkpoint when follow is set and
// it follows the leader, and otherwise the replay with fork.
func (e *soEngine) reach(ctx context.Context, snap sobject.SharedObjectStateSnapshot, fork *replayFork, follow bool) (*InnerState, []replayOutcome, error) {
	if follow {
		follows, err := e.follows(ctx, snap)
		if err != nil {
			return nil, nil, err
		}
		if follows {
			state, err := e.replay.follow(ctx, snap)
			return state, nil, err
		}
	}
	return e.replay.sync(ctx, snap, fork)
}

// follows reports whether this device follows the leader of snap: it installs
// each checkpoint the leader publishes instead of replaying the operations
// above it. An owner replays, since it may checkpoint or judge checkpoints.
// So does a device with its own edits above the checkpoint, so it shows them
// until a checkpoint covers them.
func (e *soEngine) follows(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (bool, error) {
	// Follow only a leader, and only as a participant that does not own. An
	// owner handing over follows: it no longer checkpoints or judges.
	cfg, err := snap.GetConfig(ctx)
	if err != nil {
		return false, err
	}
	if cfg.Leader() == "" {
		return false, nil
	}
	own, err := e.ownershipOf(ctx, snap)
	if err != nil {
		return false, err
	}
	self := e.so.GetPeerID().String()
	if own.Owns(self) {
		return false, nil
	}

	// Replay while an edit of this device is above the checkpoint.
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return false, err
	}
	for _, h := range set.Order() {
		if inner := set.Get(h); inner.GetPeerId() == self && !inner.IsAcknowledgment() {
			return false, nil
		}
	}
	return true, nil
}

// follow makes the World of the checkpoint of snap the World and returns it,
// replaying nothing above it. The replay drops its positions, so a later sync
// replays from the checkpoint.
func (r *replayer) follow(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (*InnerState, error) {
	// Read without waiting for a peer, as sync does.
	ctx, peerWait := block.WithoutPeerWait(ctx)
	r.gap = replayGap{}

	// Start from the checkpoint and drop the positions above it.
	if err := r.rebase(ctx, snap); err != nil {
		return nil, r.stopAt(peerWait, err)
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, err
	}
	r.set = set
	if len(r.positions) != 0 {
		r.positions = nil
		r.changed, r.deferred = true, false
		r.span, r.spanLen = nil, 0
	}
	return r.base, nil
}

// PublishedGraph returns the root of every block a replay of snap reads: the
// World the checkpoint of snap publishes and the payloads of the operations
// above it, resolving operation types with lookupOp. The Worlds after the
// operations are derived from these. When an operation carries payloads, the
// root is a new span block referencing the World and the payloads, returned
// as entry for the caller to store; otherwise it is the World root and entry
// is nil. An operation this participant cannot decrypt or that its config does
// not authorize applies nothing and has no payload.
func PublishedGraph(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	lookupOp world.LookupOp,
) (entry *block.PutBatchEntry, root *block.BlockRef, err error) {
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
		if reason != "" {
			continue
		}
		refs, err := opPayloadRefs(ctx, lookupOp, opData)
		if err != nil {
			continue
		}
		payloads = append(payloads, refs...)
	}
	if len(payloads) == 0 {
		return nil, worldRoot, nil
	}

	// Reference the World and the payloads from one span block.
	var worlds []*block.BlockRef
	if !worldRoot.GetEmpty() {
		worlds = append(worlds, worldRoot)
	}
	data, err := (&ReplaySpan{Worlds: worlds, Payloads: payloads}).MarshalVT()
	if err != nil {
		return nil, nil, err
	}
	root, err = block.BuildBlockRef(data, nil)
	if err != nil {
		return nil, nil, err
	}
	entry = &block.PutBatchEntry{Ref: root, Data: data, Refs: slices.Concat(worlds, payloads), RefsKnown: true}
	return entry, root, nil
}
