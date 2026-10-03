package sobject_world_engine

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
)

// replayBaseRootName names the local root that holds the checkpoint's World,
// which every replay starts from.
const replayBaseRootName = "replay-base"

// replayCursorStoreID is the local state store holding the saved replay.
const replayCursorStoreID = "world-replay"

// replayCursorKey is the key of the saved replay in its store.
var replayCursorKey = []byte("cursor")

// replayOutcome is the deterministic outcome of one replayed operation.
type replayOutcome struct {
	// hash identifies the operation.
	hash []byte
	// reason is empty when the operation applied. Otherwise it says in plain
	// words why the operation was not applied.
	reason string
	// conflict is set when the World rejected the operation after the
	// operations replayed before it.
	conflict bool
	// revoked is set when the operation is not applied but an earlier replay
	// on this device applied it.
	revoked bool
}

// replayPosition is one replayed operation and the World after it.
type replayPosition struct {
	// outcome is the outcome of the operation.
	outcome replayOutcome
	// state is the World after the operation. A position restored from a
	// saved replay keeps it only for the last position.
	state *InnerState
}

// replayFork is a local write computed on the World after the first index
// positions of a replay from base. Replay adopts its World as the outcome of
// the write when the write follows exactly those positions.
type replayFork struct {
	// base is the replay base the fork started from.
	base *InnerState
	// index is the number of positions the fork followed.
	index int
	// hash identifies the written operation.
	hash []byte
	// state is the World after the write.
	state *InnerState
}

// replayer replays an operation set to a World in the set's deterministic
// order, starting from the World of the set's checkpoint. The outcome of every
// operation depends only on the operation set, never on the replaying device
// or the order operations arrived in. It keeps the World after every replayed
// position, so a later replay resumes after the longest prefix its order
// shares with the previous replay. A replay saved by an earlier replayer of
// the same World resumes the same way. The owner of the replayer serializes
// calls.
type replayer struct {
	// c processes World operations.
	c *Controller
	// so holds the World blocks.
	so sobject.SharedObject
	// base is the World of the checkpoint, before the first operation.
	base *InnerState
	// positions are the replayed operations in order.
	positions []replayPosition
	// changed is set when base or positions differ from the saved replay.
	changed bool
	// applied holds the operations a replay on this device has applied,
	// including those of the saved replay.
	applied map[string]struct{}
}

// newReplayer constructs a replayer for the World of so.
func newReplayer(c *Controller, so sobject.SharedObject) *replayer {
	return &replayer{c: c, so: so}
}

// sync replays snap and returns the World after the last operation it can
// place, with the outcome of every placed operation in order. fork, when set,
// supplies the World after a local write in place of replaying it.
func (r *replayer) sync(ctx context.Context, snap sobject.SharedObjectStateSnapshot, fork *replayFork) (*InnerState, []replayOutcome, error) {
	// Restart from the checkpoint's World when it changed.
	checkpoint, err := snap.GetCheckpoint(ctx)
	if err != nil {
		return nil, nil, err
	}
	base := &InnerState{}
	if err := base.UnmarshalVT(checkpoint.GetStateData()); err != nil {
		return nil, nil, errors.Wrap(err, "checkpoint World state")
	}
	if !base.EqualVT(r.base) {
		if err := r.c.retainWorldRoot(ctx, r.so, replayBaseRootName, base.GetHeadRef()); err != nil {
			return nil, nil, err
		}
		r.base, r.positions, r.changed = base, r.positionsAbove(base), true
	}

	// Replay the operation set from the shared prefix.
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, nil, err
	}
	return r.replay(ctx, snap, set, fork)
}

// positionsAbove returns the positions after the last one whose World is
// base. A checkpoint at a replayed position covers the positions up to it, and
// the order of the rest is unchanged, so their outcomes stand.
func (r *replayer) positionsAbove(base *InnerState) []replayPosition {
	for i, v := range slices.Backward(r.positions) {
		if state := v.state; state != nil && state.EqualVT(base) {
			return slices.Clone(r.positions[i+1:])
		}
	}
	return nil
}

// stateAfter returns the World after prefix when the replay placed prefix
// first and still holds that World, or nil.
func (r *replayer) stateAfter(prefix [][]byte) *InnerState {
	if len(prefix) == 0 || len(prefix) > len(r.positions) {
		return nil
	}
	for i, h := range prefix {
		if !bytes.Equal(r.positions[i].outcome.hash, h) {
			return nil
		}
	}
	return r.positions[len(prefix)-1].state
}

// load restores the replay saved in the local state of the World, if any.
// Only its last position keeps a World, so a later order that diverges before
// it replays from the base.
func (r *replayer) load(ctx context.Context) error {
	// Read the saved replay.
	store, release, err := r.so.AccessLocalStateStore(ctx, replayCursorStoreID, nil)
	if err != nil {
		return err
	}
	defer release()
	open := func(ctx context.Context) (kvtx.Tx, error) { return store.NewTransaction(ctx, false) }
	cursor := &ReplayCursor{}
	err = kvtx.RunTransaction(ctx, false, open, func(ctx context.Context, tx kvtx.Tx) error {
		data, found, err := tx.Get(ctx, replayCursorKey)
		if err != nil || !found {
			return err
		}
		return cursor.UnmarshalVT(data)
	})
	if err != nil {
		return err
	}
	if cursor.GetBase() == nil {
		return nil
	}

	// Restore its outcomes, with the World after the last one.
	r.base = cursor.GetBase()
	r.positions = make([]replayPosition, len(cursor.GetOutcomes()))
	for i, outcome := range cursor.GetOutcomes() {
		r.positions[i].outcome = replayOutcome{
			hash:     outcome.GetHash(),
			reason:   outcome.GetReason(),
			conflict: outcome.GetConflict(),
			revoked:  outcome.GetRevoked(),
		}
		if outcome.GetReason() == "" || outcome.GetRevoked() {
			r.markApplied(outcome.GetHash())
		}
	}
	if n := len(r.positions); n != 0 {
		r.positions[n-1].state = cursor.GetHead()
	}
	r.changed = false
	return nil
}

// save writes the replay to the local state of the World when it changed. The
// caller holds the World after the replay, so a later load resumes from a
// World whose blocks are kept.
func (r *replayer) save(ctx context.Context) error {
	// Skip an unchanged replay.
	if !r.changed {
		return nil
	}

	// Encode the base, the outcomes and the World after them.
	base, head, _ := r.head()
	cursor := &ReplayCursor{
		Base:     base,
		Head:     head,
		Outcomes: make([]*ReplayCursorOutcome, len(r.positions)),
	}
	for i, pos := range r.positions {
		cursor.Outcomes[i] = &ReplayCursorOutcome{
			Hash:     pos.outcome.hash,
			Reason:   pos.outcome.reason,
			Conflict: pos.outcome.conflict,
			Revoked:  pos.outcome.revoked,
		}
	}
	data, err := cursor.MarshalVT()
	if err != nil {
		return err
	}

	// Write it.
	store, release, err := r.so.AccessLocalStateStore(ctx, replayCursorStoreID, nil)
	if err != nil {
		return err
	}
	defer release()
	open := func(ctx context.Context) (kvtx.Tx, error) { return store.NewTransaction(ctx, true) }
	err = kvtx.RunTransaction(ctx, true, open, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, replayCursorKey, data)
	})
	if err != nil {
		return err
	}
	r.changed = false
	return nil
}

// head returns the replay base and the World after the last replayed position,
// with the number of positions.
func (r *replayer) head() (*InnerState, *InnerState, int) {
	if len(r.positions) == 0 {
		return r.base, r.base, 0
	}
	return r.base, r.positions[len(r.positions)-1].state, len(r.positions)
}

// replay replays set after the longest prefix its order shares with the
// previous replay. A World of that prefix may have been collected since, so a
// missing block while resuming replays again from the base.
func (r *replayer) replay(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	set *sobject.SOOperationSet,
	fork *replayFork,
) (*InnerState, []replayOutcome, error) {
	// Keep the positions of the prefix the new order shares with the last one.
	// A restored prefix resumes only from its last position.
	order := set.Order()
	n := 0
	for n < len(order) && n < len(r.positions) && bytes.Equal(order[n], r.positions[n].outcome.hash) {
		n++
	}
	if n != 0 && r.positions[n-1].state == nil {
		n = 0
	}
	if n != len(r.positions) || n != len(order) {
		r.changed = true
	}
	r.positions = r.positions[:n]

	// Replay the rest of the order from the World after the shared prefix.
	state := r.base
	if n != 0 {
		state = r.positions[n-1].state
	}
	for i, h := range order[n:] {
		// Adopt a local write that follows exactly the positions it forked
		// from.
		if i == 0 && fork != nil && fork.base == r.base && fork.index == n && bytes.Equal(fork.hash, h) {
			state = fork.state
			r.place(replayOutcome{hash: h}, state)
			continue
		}

		// Apply every other operation as every member does.
		outcome := replayOutcome{hash: h}
		if set.Equivocated(h) {
			outcome.reason = sobject.ReasonEquivocated
		} else {
			next, why, conflict, err := r.replayOp(ctx, snap, set.Get(h), n+i, state)
			if errors.Is(err, block.ErrNotFound) && n != 0 {
				r.positions = nil
				return r.replay(ctx, snap, set, nil)
			}
			if err != nil {
				return nil, nil, err
			}
			if next != nil {
				state = next
			}
			outcome.reason, outcome.conflict = why, conflict
		}
		r.place(outcome, state)
	}

	// Report every outcome in order.
	outcomes := make([]replayOutcome, len(r.positions))
	for i, pos := range r.positions {
		outcomes[i] = pos.outcome
	}
	return state, outcomes, nil
}

// place appends the outcome of the next operation and the World after it, and
// records whether this device has applied the operation.
func (r *replayer) place(outcome replayOutcome, state *InnerState) {
	if outcome.reason == "" {
		r.markApplied(outcome.hash)
	} else {
		_, outcome.revoked = r.applied[string(outcome.hash)]
	}
	r.positions = append(r.positions, replayPosition{outcome: outcome, state: state})
}

// markApplied records that this device applied the operation with hash h.
func (r *replayer) markApplied(h []byte) {
	if r.applied == nil {
		r.applied = make(map[string]struct{})
	}
	r.applied[string(h)] = struct{}{}
}

// replayOp applies one operation to state as its author, under the config the
// operation names. It returns the next World, or nil and the reason the
// operation was not applied, with whether the World rejected it.
func (r *replayer) replayOp(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	inner *sobject.SOOperationInner,
	idx int,
	state *InnerState,
) (*InnerState, string, bool, error) {
	// An acknowledgment applies nothing.
	if inner.IsAcknowledgment() {
		return nil, "", false, nil
	}

	// Authorize and decode the operation as every member does.
	writer, opData, reason, err := sobject.PrepareReplayOp(ctx, snap, inner)
	if err != nil || reason != "" {
		return nil, reason, false, err
	}
	author, err := inner.ParsePeerID()
	if err != nil {
		return nil, "", false, err
	}

	// Apply it as the author's person.
	person := writer.GetEntityId()
	if person == "" {
		person = author.String()
	}
	next, res, err := r.c.processOp(
		world.WithOperationPerson(ctx, person),
		r.c.le,
		r.so,
		opData,
		inner.GetLocalId(),
		author,
		inner.GetNonce(),
		idx,
		state,
	)
	if err != nil {
		return nil, "", false, err
	}
	if !res.GetSuccess() {
		return nil, res.GetErrorDetails().GetErrorMsg(), true, nil
	}
	return next, "", false, nil
}

// outcomeReason returns the reason the operation with hash h was not applied,
// and whether replay placed it.
func outcomeReason(outcomes []replayOutcome, h []byte) (string, bool) {
	for _, outcome := range outcomes {
		if bytes.Equal(outcome.hash, h) {
			return outcome.reason, true
		}
	}
	return "", false
}
