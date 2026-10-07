package sobject_world_engine

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
)

// replayBaseRootName names the local root that holds the checkpoint's World,
// which every replay starts from.
const replayBaseRootName = "replay-base"

// ReplaySpanRootName names the local root that holds the chain of span blocks
// reaching the World after every replayed operation above the checkpoint and
// the payloads of those operations. A member replaying from the checkpoint reads each of them, while
// the head may no longer reach an object root that an operation created and a
// later operation replaced, and no World need reach a payload.
const ReplaySpanRootName = "replay-span"

// replayCursorStoreID is the local state store holding the saved replay.
const replayCursorStoreID = "world-replay"

// replayCursorKey is the key of the saved replay in its store.
var replayCursorKey = []byte("cursor")

// replayProgressInterval is how often a long replay logs its progress.
const replayProgressInterval = 30 * time.Second

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
	// payloads are the roots of the operation's payload.
	payloads []*block.BlockRef
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
	// payloads are the roots of the write's payload.
	payloads []*block.BlockRef
	// published is set when the write committed with the replay save pending
	// before it, and its World under the accepted World root.
	published bool
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
	// lookupOp resolves World operations to read their payload roots.
	lookupOp world.LookupOp
	// base is the World of the checkpoint, before the first operation.
	base *InnerState
	// positions are the replayed operations in order.
	positions []replayPosition
	// changed is set when base or positions differ from the saved replay.
	changed bool
	// deferred is set when the only change since the saved replay is the
	// adopted published fork. The accepted World root holds its World, so the
	// next publication carries the save.
	deferred bool
	// applied holds the operations a replay on this device has applied,
	// including those of the saved replay.
	applied map[string]struct{}
	// set is the operation set of the last replay.
	set *sobject.SOOperationSet
	// mismatch is the latest judged checkpoint whose World differs from this
	// replay's, or nil when the latest judged checkpoint agreed.
	mismatch *sobject.SOCheckpointMismatch
	// missing is the block the last sync stopped at because no connected peer
	// could serve it, or nil when that sync did not stop for that reason.
	missing *block.BlockRef
	// span is the saved span block, the head of a chain holding the Worlds and
	// payloads of the first spanLen positions, or nil when no saved chain
	// holds a prefix of positions.
	span *block.BlockRef
	// spanLen is the number of positions span holds.
	spanLen int
	// next and nextLen are the span the pending save names and the positions
	// it holds. saved makes them span and spanLen.
	next    *block.BlockRef
	nextLen int
}

// newReplayer constructs a replayer for the World of so.
func newReplayer(c *Controller, so sobject.SharedObject) *replayer {
	return &replayer{c: c, so: so, lookupOp: c.buildLookupWorldOp(c.le)}
}

// sync replays snap and returns the World after the last operation it can
// place, with the outcome of every placed operation in order. fork, when set,
// supplies the World after a local write in place of replaying it. When a block
// an operation needs is not available, replay stops at that operation and sync
// returns the World before it with an error block.IsNotAvailable reports; the
// next sync resumes at the same operation. Reads do not wait for a peer to
// connect, since the caller holds the writer lock: a block only an absent peer
// holds stops replay and is recorded in missing.
func (r *replayer) sync(ctx context.Context, snap sobject.SharedObjectStateSnapshot, fork *replayFork) (*InnerState, []replayOutcome, error) {
	// Read without waiting for a peer.
	ctx, peerWait := block.WithoutPeerWait(ctx)
	r.missing = nil

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
		r.base, r.positions = base, r.positionsAbove(base)
		r.changed, r.deferred = true, false
		r.span, r.spanLen = nil, 0
	}

	// Replay the operation set from the shared prefix.
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, nil, err
	}
	state, outcomes, err := r.replay(ctx, snap, set, fork)
	if block.IsNotAvailable(err) {
		r.missing = peerWait.GetRef()
	}
	return state, outcomes, err
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
// checkpoint covers, and those operations in replay order. It answers only
// when the last replay held every covered operation, placed them before every
// other operation, and still holds the World after them; a device missing a
// covered operation, or holding another that sorts among them, cannot judge
// the checkpoint.
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
			payloads: outcome.GetPayloads(),
		}
		r.positions[i].world = outcome.GetWorld()
		if outcome.GetReason() == "" || outcome.GetRevoked() {
			r.markApplied(outcome.GetHash())
		}
	}
	if n := len(r.positions); n != 0 {
		r.positions[n-1].state = cursor.GetHead()
	}
	r.changed, r.deferred = false, false
	return nil
}

// save writes the replay to the local state of the World when it changed and
// no publication carries it. The caller first holds the span and the World
// after the replay, so a later load resumes from Worlds whose blocks are kept.
func (r *replayer) save(ctx context.Context) error {
	// Skip a saved or deferred replay.
	if !r.changed || r.deferred {
		return nil
	}
	cursor, err := r.encodeCursor()
	if err != nil {
		return err
	}

	// Write the cursor.
	store, release, err := r.so.AccessLocalStateStore(ctx, replayCursorStoreID, nil)
	if err != nil {
		return err
	}
	defer release()
	open := func(ctx context.Context) (kvtx.Tx, error) { return store.NewTransaction(ctx, true) }
	err = kvtx.RunTransaction(ctx, true, open, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, replayCursorKey, cursor)
	})
	if err != nil {
		return err
	}
	r.saved()
	return nil
}

// holdSpan adds to hold the World after every position and the payloads of
// every position under ReplaySpanRootName when save will write the replay,
// releasing those of positions a checkpoint now covers.
func (r *replayer) holdSpan(hold *rootHold) error {
	// Skip a saved or deferred replay.
	if !r.changed || r.deferred {
		return nil
	}
	entry, span, err := r.encodeSpan(nil)
	if err != nil {
		return err
	}

	// Hold the new span block, or release the name when no World or payload
	// follows the checkpoint. A store without root ownership holds nothing,
	// so the next save starts a new chain.
	switch {
	case entry != nil:
		if err := hold.refBlock(ReplaySpanRootName, entry.Data, entry.Refs); err != nil {
			return err
		}
	case span == nil:
		hold.release(ReplaySpanRootName)
	}
	if hold.skip {
		span = nil
	}
	r.pend(span)
	return nil
}

// pendingSave returns the unsaved replay as parts of a publication: the span
// block, the span root and the cursor head. The span also holds payloads, the
// payloads of the published operation, which replay has not placed yet. It
// returns no parts when the replay is saved and payloads is empty. Call saved
// once the publication commits.
func (r *replayer) pendingSave(publisher sobject.StatePublisher, payloads []*block.BlockRef) ([]*block.PutBatchEntry, []block.NamedRoot, *block.AtomicHeadUpdate, error) {
	// Skip a saved replay with nothing more to hold.
	if !r.changed && len(payloads) == 0 {
		return nil, nil, nil, nil
	}
	entry, span, err := r.encodeSpan(payloads)
	if err != nil {
		return nil, nil, nil, err
	}
	r.pend(span)

	// Name a new span block, or release the name when no World or payload
	// follows the checkpoint. The name already holds an unchanged span.
	var entries []*block.PutBatchEntry
	var roots []block.NamedRoot
	switch {
	case entry != nil:
		entries = []*block.PutBatchEntry{entry}
		roots = []block.NamedRoot{{Name: ReplaySpanRootName, Ref: span}}
	case span == nil:
		roots = []block.NamedRoot{{Name: ReplaySpanRootName}}
	}

	// Write the cursor only when it changed.
	var head *block.AtomicHeadUpdate
	if r.changed {
		cursor, err := r.encodeCursor()
		if err != nil {
			return nil, nil, nil, err
		}
		head = publisher.LocalStateHead(replayCursorStoreID, replayCursorKey, cursor)
	}
	return entries, roots, head, nil
}

// pend records span, holding every position, as the span the pending save
// names.
func (r *replayer) pend(span *block.BlockRef) {
	r.next, r.nextLen = span, 0
	if span != nil {
		r.nextLen = len(r.positions)
	}
}

// saved records that the replay is durable as it stands, with the span the
// pending save named.
func (r *replayer) saved() {
	r.changed, r.deferred = false, false
	r.span, r.spanLen = r.next, r.nextLen
	r.next, r.nextLen = nil, 0
}

// encodeSpan returns the span block holding the World after every position
// and the payloads of every position followed by extra, with the span to name.
// The block holds only the positions the saved span does not, and refers to
// the saved span as Prev, so the chain holds every position. It returns no
// block when the saved span holds everything, and no span when no World or
// payload follows the checkpoint.
func (r *replayer) encodeSpan(extra []*block.BlockRef) (*block.PutBatchEntry, *block.BlockRef, error) {
	// Collect each World once, in replay order, after the saved span. A
	// rejected operation leaves the World of the position before it.
	prev, from := r.span, r.spanLen
	var worlds, payloads []*block.BlockRef
	for _, pos := range r.positions[from:] {
		payloads = append(payloads, pos.outcome.payloads...)
		if pos.world.GetEmpty() || (len(worlds) != 0 && worlds[len(worlds)-1].EqualsRef(pos.world)) {
			continue
		}
		worlds = append(worlds, pos.world)
	}
	payloads = append(payloads, extra...)
	if len(worlds) == 0 && len(payloads) == 0 {
		return nil, prev, nil
	}

	// Reference them and the saved span from one new block.
	data, err := (&ReplaySpan{Worlds: worlds, Payloads: payloads, Prev: prev}).MarshalVT()
	if err != nil {
		return nil, nil, err
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		return nil, nil, err
	}
	refs := slices.Concat(worlds, payloads)
	if prev != nil {
		refs = append(refs, prev)
	}
	return &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs}, ref, nil
}

// encodeCursor returns the encoded base, outcomes and World after them.
func (r *replayer) encodeCursor() ([]byte, error) {
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
			Payloads: pos.outcome.payloads,
		}
	}
	return cursor.MarshalVT()
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
		// Defer the save of a published fork appended to a saved replay.
		adopted := fork != nil && fork.published && fork.base == r.base &&
			fork.index == n && n == len(r.positions) && len(order) == n+1 &&
			bytes.Equal(fork.hash, order[n])
		r.deferred = adopted && (!r.changed || r.deferred)
		r.changed = true
	}
	r.positions = r.positions[:n]
	if n < r.spanLen {
		r.span, r.spanLen = nil, 0
	}

	// Replay the rest of the order from the World after the shared prefix.
	state = r.base
	if n != 0 {
		state = r.positions[n-1].state
	}
	lastProgress := time.Now()
	for i, h := range order[n:] {
		// Log the progress of a long replay.
		if time.Since(lastProgress) >= replayProgressInterval {
			lastProgress = time.Now()
			r.c.le.Infof("replay placed %d of %d operations", n+i, len(order))
		}

		// Adopt a local write that follows exactly the positions it forked
		// from.
		if i == 0 && fork != nil && fork.base == r.base && fork.index == n && bytes.Equal(fork.hash, h) {
			state = fork.state
			r.place(replayOutcome{hash: h, payloads: fork.payloads}, state)
			continue
		}

		// Apply every other operation as every member does.
		outcome := replayOutcome{hash: h}
		if set.Equivocated(h) {
			outcome.reason = sobject.ReasonEquivocated
		} else {
			inner := set.Get(h)
			next, opOutcome, opErr := r.replayOp(ctx, w, snap, inner, n+i, state)
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
			opOutcome.hash = h
			outcome = opOutcome
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
// the operation names. It returns the next World, or nil when the operation
// was not applied, and the operation's outcome without its hash.
func (r *replayer) replayOp(
	ctx context.Context,
	w *replayWorld,
	snap sobject.SharedObjectStateSnapshot,
	inner *sobject.SOOperationInner,
	idx int,
	state *InnerState,
) (*InnerState, replayOutcome, error) {
	// An acknowledgment applies nothing.
	var outcome replayOutcome
	if inner.IsAcknowledgment() {
		return nil, outcome, nil
	}

	// Authorize and decode the operation as every member does.
	writer, opData, reason, err := sobject.PrepareReplayOp(ctx, snap, inner)
	if err != nil || reason != "" {
		outcome.reason = reason
		return nil, outcome, err
	}
	author, err := inner.ParsePeerID()
	if err != nil {
		return nil, outcome, err
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
		return nil, outcome, err
	}

	// Keep the payload roots of a rejected operation too, since a later order
	// may apply it. An operation that does not decode applied nothing and
	// has none.
	outcome.payloads, err = opPayloadRefs(ctx, r.lookupOp, opData)
	if err != nil && res.GetSuccess() {
		return nil, outcome, err
	}
	if !res.GetSuccess() {
		outcome.reason, outcome.conflict = res.GetErrorDetails().GetErrorMsg(), true
		return nil, outcome, nil
	}
	return next, outcome, nil
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
