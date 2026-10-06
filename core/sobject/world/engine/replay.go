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

// replaySpanRootName names the local root that holds the World after every
// replayed operation above the checkpoint. A member replaying from the
// checkpoint reads each of them, while the head may no longer reach an object
// root that an operation created and a later operation replaced.
const replaySpanRootName = "replay-span"

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
	// world is the root block of the World after the operation. A restored
	// position keeps it.
	world *block.BlockRef
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
	// set is the operation set of the last replay.
	set *sobject.SOOperationSet
	// mismatch is the latest judged checkpoint whose World differs from this
	// replay's, or nil when the latest judged checkpoint agreed.
	mismatch *sobject.SOCheckpointMismatch
}

// newReplayer constructs a replayer for the World of so.
func newReplayer(c *Controller, so sobject.SharedObject) *replayer {
	return &replayer{c: c, so: so}
}

// sync replays snap and returns the World after the last operation it can
// place, with the outcome of every placed operation in order. fork, when set,
// supplies the World after a local write in place of replaying it. When a block
// an operation needs is not available, replay stops at that operation and sync
// returns the World before it with an error block.IsNotAvailable reports; the
// next sync resumes at the same operation.
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
		if err := holdWorldRoot(ctx, r.so, replayBaseRootName, base.GetHeadRef().GetRootRef()); err != nil {
			return nil, nil, err
		}
		if world, _, ok := r.coveredWorld(checkpoint); ok {
			r.mismatch = nil
			if !world.EqualVT(base) {
				r.mismatch = &sobject.SOCheckpointMismatch{Height: checkpoint.GetHeight()}
			}
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

// coveredWorld returns the World this replay reached after the operations
// checkpoint covers, and those operations in replay order. It answers only when the last replay held every covered
// operation, placed them before every other operation, and still holds the
// World after them; a device missing a covered operation, or holding another
// that sorts among them, cannot judge the checkpoint.
func (r *replayer) coveredWorld(checkpoint *sobject.SOCheckpointInner) (*InnerState, [][]byte, bool) {
	// Judge only after a replay of a held set.
	if r.set == nil || r.base == nil {
		return nil, nil, false
	}
	covers := sobject.NewSOOperationSet(r.so.GetSharedObjectID(), checkpoint)
	covered := func(h []byte) bool {
		inner := r.set.Get(h)
		return inner != nil && covers.Covers(inner.GetPeerId(), inner.GetNonce())
	}

	// The covered operations must be the first positions.
	n := 0
	for n < len(r.positions) && covered(r.positions[n].outcome.hash) {
		n++
	}
	for _, pos := range r.positions[n:] {
		if covered(pos.outcome.hash) {
			return nil, nil, false
		}
	}

	// Each covered author head must be among them, or below the last
	// checkpoint.
	prefix := make([][]byte, n)
	placed := make(map[string]struct{}, n)
	for i, pos := range r.positions[:n] {
		prefix[i] = pos.outcome.hash
		placed[string(pos.outcome.hash)] = struct{}{}
	}
	for _, author := range checkpoint.GetAuthors() {
		if _, ok := placed[string(author.GetOpHash())]; !ok && !r.set.Covers(author.GetPeerId(), author.GetNonce()) {
			return nil, nil, false
		}
	}

	// Return the World after them.
	if n == 0 {
		return r.base, prefix, true
	}
	world := r.positions[n-1].state
	return world, prefix, world != nil
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
		r.positions[i].world = outcome.GetWorld()
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
// caller first holds the span and the World after the replay, so a later load
// resumes from Worlds whose blocks are kept.
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
			World:    pos.world,
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

// holdSpan adds to hold the World after every position under
// replaySpanRootName when the replay changed, releasing the Worlds of
// positions a checkpoint now covers.
func (r *replayer) holdSpan(hold *rootHold) error {
	// Collect each World once, in replay order. A rejected operation leaves
	// the World of the position before it.
	if !r.changed {
		return nil
	}
	var worlds []*block.BlockRef
	for _, pos := range r.positions {
		if pos.world.GetEmpty() || (len(worlds) != 0 && worlds[len(worlds)-1].EqualsRef(pos.world)) {
			continue
		}
		worlds = append(worlds, pos.world)
	}
	if len(worlds) == 0 {
		hold.release(replaySpanRootName)
		return nil
	}

	// Hold the span block referencing them.
	data, err := (&ReplaySpan{Worlds: worlds}).MarshalVT()
	if err != nil {
		return err
	}
	return hold.refBlock(replaySpanRootName, data, worlds)
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
// previous replay. The World after that prefix may have been collected since,
// so when its root is missing, replay starts again from the base. Any other
// missing block belongs to the operation that needs it and stops replay there,
// with the positions before it kept. The blocks of every placed position are
// durable when replay returns.
func (r *replayer) replay(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	set *sobject.SOOperationSet,
	fork *replayFork,
) (state *InnerState, outcomes []replayOutcome, err error) {
	// Advance one World through the pass and fence its blocks at the end.
	w := newReplayWorld(r.c, r.so)
	defer func() {
		if cerr := w.close(ctx); cerr != nil {
			state, outcomes, err = nil, nil, cerr
		}
	}()

	// Keep the positions of the prefix the new order shares with the last one.
	// A restored prefix resumes only from its last position.
	r.set = set
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
	state = r.base
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
			inner := set.Get(h)
			next, why, conflict, opErr := r.replayOp(ctx, w, snap, inner, n+i, state)
			if block.IsNotAvailable(opErr) {
				if i == 0 && n != 0 && r.worldLost(ctx, state) {
					r.positions = nil
					return r.replay(ctx, snap, set, nil)
				}
				return state, nil, errors.Wrapf(opErr, "replay stopped at operation %d (nonce %d of %s)", n+i, inner.GetNonce(), inner.GetPeerId())
			}
			if opErr != nil {
				return nil, nil, opErr
			}
			if next != nil {
				state = next
			}
			outcome.reason, outcome.conflict = why, conflict
		}
		r.place(outcome, state)
	}

	// Report every outcome in order.
	outcomes = make([]replayOutcome, len(r.positions))
	for i, pos := range r.positions {
		outcomes[i] = pos.outcome
	}
	return state, outcomes, nil
}

// worldLost reports whether the root block of the World state is missing from
// the block store.
func (r *replayer) worldLost(ctx context.Context, state *InnerState) bool {
	// Bind the World root; only a missing block counts as lost.
	head := state.GetHeadRef()
	if head.GetEmpty() {
		return false
	}
	ws, err := r.c.buildBlkEngine(ctx, r.c.le, r.so, head, head.GetTransformConf())
	if err != nil {
		return errors.Is(err, block.ErrNotFound)
	}
	ws.Release()
	return false
}

// place appends the outcome of the next operation and the World after it, and
// records whether this device has applied the operation.
func (r *replayer) place(outcome replayOutcome, state *InnerState) {
	if outcome.reason == "" {
		r.markApplied(outcome.hash)
	} else {
		_, outcome.revoked = r.applied[string(outcome.hash)]
	}
	r.positions = append(r.positions, replayPosition{outcome: outcome, state: state, world: state.GetHeadRef().GetRootRef()})
}

// markApplied records that this device applied the operation with hash h.
func (r *replayer) markApplied(h []byte) {
	if r.applied == nil {
		r.applied = make(map[string]struct{})
	}
	r.applied[string(h)] = struct{}{}
}

// replayOp applies one operation to state on w as its author, under the config
// the operation names. It returns the next World, or nil and the reason the
// operation was not applied, with whether the World rejected it.
func (r *replayer) replayOp(
	ctx context.Context,
	w *replayWorld,
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
		w,
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
