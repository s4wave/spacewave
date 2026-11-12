package sobject

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// bodyRetryMin and bodyRetryMax bound the wait before a BodyDriver applies
// again after an error. A state change wakes it sooner.
const (
	bodyRetryMin = 250 * time.Millisecond
	bodyRetryMax = 30 * time.Second
)

// errBodyRetry wakes the driver to step again without a state change.
var errBodyRetry = errors.New("body retry")

// bodyPosition is one operation the body applied above its checkpoint.
type bodyPosition struct {
	// hash identifies the operation.
	hash []byte
	// reason is why the body refused it, or empty when it applied.
	reason string
}

// BodyDriver replays the agreed order of a SharedObject's operations into its
// Body. For each state it installs the checkpoint the state holds when it
// moved, keeps the operations the body applied that the order still places
// first, and applies the rest in order. A device that follows the leader
// installs each checkpoint and applies nothing. An error from the body stops
// the step at that operation, and the next step starts there.
//
// The driver serializes every call into the body under mtx, which also
// guards its fields.
type BodyDriver struct {
	// le is the logger.
	le *logrus.Entry
	// so supplies the state and the local peer.
	so SharedObject
	// body receives the order.
	body Body
	// report receives what the health shows when it changes, or is nil.
	report func(*BodyReport)
	// checkpointer signs this device's checkpoints, or is nil for a device
	// that signs none.
	checkpointer *bodyCheckpointer
	// wake asks Execute to step again without a state change.
	wake chan error
	// voteWake has the group voter step again, or is nil. A vote's store
	// refusal calls it once due, as no state change may come to retry it.
	voteWake func()
	// now returns the current time.
	now func() time.Time
	// damageRetry is how long a damaged step or checkpoint store waits after
	// its latest refusal before it is tried again, storeDamageRetry outside
	// tests.
	damageRetry time.Duration

	// mtx serializes the body's calls and guards the fields below.
	mtx sync.Mutex
	// installed is the checkpoint the body holds, or nil before the first
	// Install.
	installed *SOCheckpointInner
	// set holds the operations of positions.
	set *SOOperationSet
	// positions are the operations the body applied above installed, in order.
	positions []bodyPosition
	// applied holds the operations above the checkpoint any replay on this
	// device applied, so a refusal of one is a revocation.
	applied map[string]struct{}
	// mismatch is the checkpoint mismatch the last Install found, or nil.
	mismatch *SOCheckpointMismatch
	// shown is the report last sent to report.
	shown *BodyReport
	// storing reports that a checkpoint's store and signature run in the
	// background, so a step signs no other.
	storing bool
	// stepRefusal and storeRefusal are the store refusals the latest steps
	// and checkpoint stores met in a row, including the stores of a vote's
	// proposal or judgment, or nil. Once one repeats, or a DamagedBody
	// reports damage, the report shows the body damaged. A repeated store
	// refusal stores a checkpoint again only once damageRetry passed since
	// it, and a repeated step refusal steps again on a new state or then;
	// the first attempt that stores clears it.
	stepRefusal, storeRefusal *StoreRefusal
	// voteTimer calls voteWake when a vote's damaged store comes due, or is
	// nil.
	voteTimer *time.Timer
}

// NewBodyDriver returns the driver of body on so. report receives what the
// health shows of the replay, and may be nil.
func NewBodyDriver(le *logrus.Entry, so SharedObject, body Body, report func(*BodyReport)) *BodyDriver {
	d := &BodyDriver{
		le:          le,
		so:          so,
		body:        body,
		report:      report,
		applied:     make(map[string]struct{}),
		wake:        make(chan error, 1),
		now:         time.Now,
		damageRetry: storeDamageRetry,
	}
	if b, ok := body.(DamagedBody); ok {
		b.WatchDamage(func() { d.wakeAfter(0) })
	}
	return d
}

// SetCheckpointer has the driver sign, through host as privKey, the
// checkpoints this device owes: as the checkpointer of an owner-controlled
// Space, a checkpoint over the stable prefix once MinCheckpointOperations
// operations are stable, or once one is when it also orders them. The body's
// Root holds the state each one carries. sfs builds the key epoch transforms
// that encrypt it. Call it before Execute.
func (d *BodyDriver) SetCheckpointer(host *SOHost, privKey crypto.PrivKey, sfs *block_transform.StepFactorySet) {
	d.checkpointer = &bodyCheckpointer{host: host, privKey: privKey, sfs: sfs}
}

// SetVoteWake has the driver call wake when a group vote's damaged store
// comes due, so the voter proposes or judges again on a quiet Space. Call it
// before Execute.
func (d *BodyDriver) SetVoteWake(wake func()) {
	d.voteWake = wake
}

// Execute steps the body to each state of so until ctx ends. After an error
// it steps again on the next state, or after a backoff when no state arrives.
func (d *BodyDriver) Execute(ctx context.Context) error {
	// Watch the state.
	ctr, rel, err := d.so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer rel()

	// Wake a failed step after a backoff, and a damaged write when its
	// refusal is due, so a cause healed outside replay clears the damage on a
	// quiet Space that sends no new state.
	wake := d.wake
	delay := bodyRetryMin
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	wakeAfter := func(after time.Duration) {
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(after, func() {
			select {
			case wake <- errBodyRetry:
			default:
			}
		})
	}
	retry := func() {
		wakeAfter(delay)
		delay = min(2*delay, bodyRetryMax)
	}

	// Step each new state, and the held one on a retry.
	var snap SharedObjectStateSnapshot
	for {
		next, err := ctr.WaitValueChange(ctx, snap, wake)
		if err != nil && !errors.Is(err, errBodyRetry) {
			return err
		}
		if next != nil {
			snap = next
		}
		if snap == nil {
			continue
		}
		err = d.Step(ctx, snap)
		if err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			d.le.WithError(err).Warn("replay stopped before the end of the order")
			if !errors.Is(err, block.ErrMissingChild) || !d.stepDamaged() {
				retry()
				continue
			}
		}

		// Wake when the earliest damaged write comes due. One the step left
		// due waits on the backoff instead: its retry is in flight, or is
		// the group voter's, so waking at once would spin.
		at := d.nextRetry()
		wait := at.Sub(d.now())
		if !at.IsZero() && wait <= 0 {
			retry()
			continue
		}
		if err == nil {
			delay = bodyRetryMin
		}
		if !at.IsZero() {
			wakeAfter(wait)
		}
	}
}

// stepDamaged reports whether the steps' store refusal repeated, so a step
// refused again waits for the next state or its retry instead of a backoff.
func (d *BodyDriver) stepDamaged() bool {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.stepRefusal.Damage() != nil
}

// nextRetry returns when the earliest damaged write is due, or the zero time.
func (d *BodyDriver) nextRetry() time.Time {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.retryAt()
}

// Damaged returns the store refusal replay could not heal, or nil.
func (d *BodyDriver) Damaged() *SOStoreDamage {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.damage()
}

// Step brings the body to the order of snap: it installs snap's checkpoint
// when it moved, then applies every operation above it the body has not
// applied in this order. It returns the first error, with the body at the
// operations before it.
func (d *BodyDriver) Step(ctx context.Context, snap SharedObjectStateSnapshot) error {
	// Hold the body for the step.
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.step(ctx, snap)
}

// step is Step with mtx held. It notes the step's store refusal, then
// publishes the report.
func (d *BodyDriver) step(ctx context.Context, snap SharedObjectStateSnapshot) (err error) {
	// Read the order.
	ev, err := ReadOrder(ctx, snap)
	if err != nil {
		return err
	}
	defer func() {
		d.stepRefusal = d.noteRefusal(d.stepRefusal, err)
		d.publishReport(ev)
	}()

	// Install the checkpoint when it moved.
	if !ev.Checkpoint.EqualVT(d.installed) {
		if err := d.install(ctx, ev); err != nil {
			return err
		}
	}

	// A device that follows the leader shows the leader's checkpoint alone.
	follows, err := d.follows(ctx, ev)
	if err != nil {
		return err
	}
	if follows {
		return d.follow(ctx, ev)
	}

	// Keep the applied operations the order still places first and the
	// config still admits, and apply the rest.
	keep := d.admittedPrefix(ev, SharedPrefix(d.hashes(), ev.Order))
	d.positions = d.positions[:keep]
	d.set = ev.Set
	for i := keep; i < len(ev.Order); i++ {
		if err := d.apply(ctx, ev, i); err != nil {
			return err
		}
	}

	// Write again the body's own refused writes once due.
	if b, ok := d.body.(DamagedBody); ok && b.Damaged() != nil {
		if err := b.Retry(ctx); err != nil {
			return err
		}
	}

	// Sign the checkpoint this device owes.
	return d.checkpoint(ctx, ev)
}

// checkpoint starts the checkpoint over ev's stable prefix when this device
// owes it, the body applied the prefix and no checkpoint is storing: it takes
// the body's state there, then stores, signs and adopts it in the background,
// so the driver keeps applying and deciding operations meanwhile. The next
// step installs the checkpoint once it is adopted. A state that moved past
// the prefix meanwhile skips it, and the driver steps again to checkpoint
// anew.
func (d *BodyDriver) checkpoint(ctx context.Context, ev *OrderEvent) error {
	// Only the checkpointer of an owner-controlled Space signs, and not while
	// handing over or while another checkpoint stores. Group control decides
	// checkpoints by vote.
	c := d.checkpointer
	self := d.so.GetPeerID().String()
	if c == nil || d.storing || !d.storeRefusal.Due(d.now()) || ev.Config.IsGroupControl() || ev.Config.Checkpointer() != self {
		return nil
	}
	own, err := ev.Snapshot.GetOwnership(ctx)
	if err != nil || own.Departing(self) {
		return err
	}

	// Wait for enough stable operations the body applied. The leader
	// checkpoints every ordered batch, since the others install its
	// checkpoints instead of replaying.
	minStable := MinCheckpointOperations
	if ev.Config.Leader() == self {
		minStable = 1
	}
	if ev.Stable < minStable || len(d.positions) < ev.Stable {
		return nil
	}
	data, store, err := d.body.Root(ctx, ev.Stable)
	if err != nil || data == nil {
		return err
	}

	// Store, sign and adopt it in the background.
	d.storing = true
	prefix := ev.Order[:ev.Stable]
	height := ev.Checkpoint.GetHeight()
	go d.storeCheckpoint(ctx, c, store, prefix, data, height)
	return nil
}

// storeCheckpoint stores the body's state for the checkpoint over prefix,
// then signs and adopts it on the held state, and wakes the driver to step
// again. It runs beside the driver's apply loop.
func (d *BodyDriver) storeCheckpoint(ctx context.Context, c *bodyCheckpointer, store func(context.Context) error, prefix [][]byte, data []byte, height uint64) {
	// Store the state, then sign and adopt the checkpoint.
	err := store(ctx)
	stored := err
	if err == nil {
		err = c.host.UpdateSOState(kvtx.WithOrderedCommitContext(ctx), func(state *SOState) error {
			return c.adopt(ctx, d.le, d.so, state, prefix, data, height)
		})
	}
	if err != nil && !errors.Is(err, errCheckpointStale) && ctx.Err() == nil {
		d.le.WithError(err).Warn("checkpoint stopped before it was adopted")
	}

	// Note a refused store, and let the next step checkpoint again, or,
	// once the refusal repeated, the step Execute wakes when it is due.
	d.mtx.Lock()
	d.storing = false
	if ctx.Err() == nil {
		d.storeRefusal = d.noteRefusal(d.storeRefusal, stored)
	}
	d.mtx.Unlock()
	d.wakeAfter(0)
}

// wakeAfter wakes the driver to step again after delay, unless a wake is
// already pending.
func (d *BodyDriver) wakeAfter(delay time.Duration) {
	wake := func() {
		select {
		case d.wake <- errBodyRetry:
		default:
		}
	}
	if delay <= 0 {
		wake()
		return
	}
	time.AfterFunc(delay, wake)
}

// ProposeCheckpoint returns the checkpoint after the one state holds over its
// stable prefix, with the body's state there, or nil while fewer than
// MinCheckpointOperations operations are stable, the body has not applied
// them in that order, or the body is damaged. The driver of a group voter
// answers for it. A store refusal counts toward damage as a signed
// checkpoint's does, so a voter stops proposing over a refused store.
func (d *BodyDriver) ProposeCheckpoint(ctx context.Context, state *SOState) (*SOCheckpointInner, error) {
	// Take the body's state after the stable prefix, then store it outside
	// the body's lock and encrypt it into the checkpoint body.
	set, prefix, data, store, err := d.proposal(ctx, state)
	if err != nil || store == nil {
		return nil, err
	}
	if err := d.storeVote(ctx, store); err != nil {
		return nil, err
	}
	dataEnc, err := d.checkpointer.encrypt(ctx, d.le, d.so, state, data)
	if err != nil {
		return nil, err
	}
	return state.stableCheckpointInner(d.so.GetSharedObjectID(), set, prefix, dataEnc)
}

// proposal returns the verified set and stable prefix of state, with the
// body's state after the prefix and the store that holds it, or a nil store
// when the driver proposes nothing.
func (d *BodyDriver) proposal(ctx context.Context, state *SOState) (*SOOperationSet, [][]byte, []byte, func(context.Context) error, error) {
	// Hold the body; a driver without a signer, or a damaged one, proposes
	// nothing.
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if d.checkpointer == nil || !d.storeRefusal.Due(d.now()) {
		return nil, nil, nil, nil, nil
	}

	// Find the stable prefix the body applied above the held checkpoint, and
	// take the body's state after it.
	set, prefix, ok, err := d.stablePrefix(state, MinCheckpointOperations)
	if err != nil || !ok {
		return nil, nil, nil, nil, err
	}
	data, store, err := d.body.Root(ctx, len(prefix))
	if err != nil || data == nil {
		return nil, nil, nil, nil, err
	}
	return set, prefix, data, store, nil
}

// storeVote runs the store of a vote's proposal or judgment without the
// body's lock, as the signer's checkpoint store runs beside the apply loop,
// then notes its refusal. When that changes the damage, it wakes the driver,
// whose step publishes the health.
func (d *BodyDriver) storeVote(ctx context.Context, store func(context.Context) error) error {
	// Store the vote's checkpoint outside the lock.
	err := store(ctx)
	if ctx.Err() != nil {
		return err
	}

	// Note its refusal, and wake the voter once a damaged store is due.
	d.mtx.Lock()
	before := d.storeRefusal.Damage()
	d.storeRefusal = d.noteRefusal(d.storeRefusal, err)
	changed := !before.EqualVT(d.storeRefusal.Damage())
	d.armVoteWake()
	d.mtx.Unlock()

	// Wake the driver to publish the health when the damage changed.
	if changed {
		d.wakeAfter(0)
	}
	return err
}

// armVoteWake arms voteTimer to wake the voter when the vote's damaged store
// is due, or stops it while none is damaged. The caller holds mtx.
func (d *BodyDriver) armVoteWake() {
	if d.voteTimer != nil {
		d.voteTimer.Stop()
		d.voteTimer = nil
	}
	at := d.storeRefusal.RetryAt()
	if d.voteWake == nil || at.IsZero() {
		return
	}
	d.voteTimer = time.AfterFunc(max(0, at.Sub(d.now())), d.voteWake)
}

// JudgeCheckpoint reports whether inner is the checkpoint after the one state
// holds over a stable prefix the body applied first, carrying the body's state
// after it. ok is false while the body cannot tell: it did not apply the
// covered operations first, or they are not stable.
func (d *BodyDriver) JudgeCheckpoint(ctx context.Context, state *SOState, inner *SOCheckpointInner) (bool, bool, error) {
	// Take the body's state after the covered operations, then store it
	// outside the body's lock and compare it with the one inner carries.
	data, want, store, decided, err := d.judgment(ctx, state, inner)
	if err != nil || store == nil {
		return false, decided, err
	}
	if err := d.storeVote(ctx, store); err != nil {
		return false, false, err
	}
	return bytes.Equal(data, want), true, nil
}

// judgment returns the body's state after the operations inner covers, the
// state inner carries and the store that holds the body's, or a nil store
// with whether the voter decided against inner.
func (d *BodyDriver) judgment(ctx context.Context, state *SOState, inner *SOCheckpointInner) ([]byte, []byte, func(context.Context) error, bool, error) {
	// Hold the body, and find the covered operations among the applied ones.
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if d.set == nil || d.checkpointer == nil {
		return nil, nil, nil, false, nil
	}
	n := d.set.CoveredPrefix(d.hashes(), inner)
	if n <= 0 {
		return nil, nil, nil, false, nil
	}

	// They must be the stable prefix of state.
	set, prefix, ok, err := d.stablePrefix(state, n)
	if err != nil || !ok || len(prefix) < n {
		return nil, nil, nil, false, err
	}
	prefix = prefix[:n]

	// Compare the body this voter would build around the same state data.
	soID := d.so.GetSharedObjectID()
	expected, err := state.stableCheckpointInner(soID, set, prefix, inner.GetStateData())
	if err != nil || !expected.EqualVT(inner) {
		return nil, nil, nil, err == nil, nil
	}

	// Compare the state it carries with the body's.
	decoded, err := d.checkpointer.decode(d.le, d.so, state, inner)
	if err != nil {
		return nil, nil, nil, true, nil
	}

	// Take the body's state unless a refusal still waits out its retry.
	if !d.storeRefusal.Due(d.now()) {
		return nil, nil, nil, false, nil
	}
	data, store, err := d.body.Root(ctx, n)
	if err != nil || data == nil {
		return nil, nil, nil, false, err
	}
	return data, decoded.GetStateData(), store, false, nil
}

// stablePrefix returns state's operations as a verified set and their stable
// prefix when it has at least minLen operations, the body applied them first
// above state's checkpoint, and that checkpoint is the installed one. The set
// comes through the checkpointer's verifier, which d has.
func (d *BodyDriver) stablePrefix(state *SOState, minLen int) (*SOOperationSet, [][]byte, bool, error) {
	// The state's checkpoint must be the installed one.
	held, err := state.GetCheckpointInner()
	if err != nil || d.installed == nil || held.GetHeight() != d.installed.GetHeight() {
		return nil, nil, false, err
	}

	// Find the stable prefix, and require the body applied it.
	set, err := d.checkpointer.verified(state)
	if err != nil {
		return nil, nil, false, err
	}
	prefix := set.StablePoint(state.GetConfig().TrimRoster())
	if len(prefix) < minLen || len(d.positions) < len(prefix) {
		return nil, nil, false, nil
	}
	return set, prefix, slices.EqualFunc(d.hashes()[:len(prefix)], prefix, bytes.Equal), nil
}

// install resets the body to ev's checkpoint, keeping the positions after the
// ones the checkpoint covers when the body reached the same state there.
func (d *BodyDriver) install(ctx context.Context, ev *OrderEvent) error {
	// Count the applied operations the checkpoint covers.
	covered := -1
	if d.installed != nil && d.set != nil {
		covered = d.set.CoveredPrefix(d.hashes(), ev.Checkpoint)
	}

	// Install it.
	kept, err := d.body.Install(ctx, ev.Checkpoint, covered)
	if err != nil {
		return err
	}

	// Report a body that reached another state, and keep what it kept.
	d.mismatch = nil
	if covered >= 0 && !kept {
		d.le.WithField("checkpoint-height", ev.Checkpoint.GetHeight()).Warn("checkpoint state differs from the replayed state")
		d.mismatch = &SOCheckpointMismatch{Height: ev.Checkpoint.GetHeight()}
	}
	positions := d.positions
	d.positions = nil
	if kept {
		d.positions = slices.Clone(positions[covered:])
	}
	d.installed = ev.Checkpoint
	return nil
}

// follow drops the operations the body applied above the checkpoint, so it
// shows the checkpoint the leader published.
func (d *BodyDriver) follow(ctx context.Context, ev *OrderEvent) error {
	// Nothing to drop once following.
	if len(d.positions) == 0 {
		return nil
	}

	// Reset the body to the checkpoint.
	if _, err := d.body.Install(ctx, ev.Checkpoint, -1); err != nil {
		return err
	}
	d.positions = nil
	return nil
}

// apply applies the operation at index i of ev's order.
func (d *BodyDriver) apply(ctx context.Context, ev *OrderEvent, i int) error {
	// Prepare the operation as every member does.
	h := ev.Order[i]
	inner := ev.Set.Get(h)
	op := &OrderedOperation{Hash: h, Inner: inner}
	switch {
	case ev.Set.Equivocated(h):
		op.Refused = ReasonEquivocated
	case inner.IsAcknowledgment():
	default:
		author, data, reason, err := PrepareReplayOp(ctx, ev.Snapshot, inner)
		if err != nil {
			return err
		}
		op.Author, op.Data, op.Refused = author, data, reason
	}

	// Apply it and record its outcome.
	reason, err := d.body.Apply(ctx, i, op)
	if err != nil {
		return errors.Wrapf(err, "apply operation %d above checkpoint %d", i, ev.Checkpoint.GetHeight())
	}
	d.positions = append(d.positions, bodyPosition{hash: h, reason: reason})
	if reason == "" {
		d.applied[string(h)] = struct{}{}
	}
	return nil
}

// follows reports whether this device follows the leader of ev: it installs
// each checkpoint the leader publishes instead of replaying the operations
// above it. An owner replays, since it may checkpoint or judge checkpoints. So
// does a device with its own edits above the checkpoint, so it shows them
// until a checkpoint covers them.
func (d *BodyDriver) follows(ctx context.Context, ev *OrderEvent) (bool, error) {
	// Follow only a leader, and only as a participant that does not own. An
	// owner handing over follows: it no longer checkpoints or judges.
	if ev.Config.Leader() == "" {
		return false, nil
	}
	own, err := ev.Snapshot.GetOwnership(ctx)
	if err != nil {
		return false, err
	}
	self := d.so.GetPeerID().String()
	if own.Owns(self) {
		return false, nil
	}

	// Replay while an edit of this device is above the checkpoint.
	for _, h := range ev.Order {
		if inner := ev.Set.Get(h); inner.GetPeerId() == self && !inner.IsAcknowledgment() {
			return false, nil
		}
	}
	return true, nil
}

// publishReport sends what the health shows of the replay of ev when it
// changed, and forgets the applied operations ev's checkpoint covers.
func (d *BodyDriver) publishReport(ev *OrderEvent) {
	// Forget the applied operations no longer above the checkpoint.
	for key := range d.applied {
		if ev.Set.Get([]byte(key)) == nil {
			delete(d.applied, key)
		}
	}
	if d.report == nil {
		return
	}

	// Collect this device's revoked edits.
	self := d.so.GetPeerID().String()
	report := &BodyReport{CheckpointMismatch: d.mismatch, Damage: d.damage()}
	if ev.Set.Unordered() > 0 {
		report.Unordered = uint32(ev.Set.Unordered()) //nolint:gosec // the operations held in memory.
	}
	for i, pos := range d.positions {
		if pos.reason == "" || ev.Set.Get(pos.hash).GetPeerId() != self {
			continue
		}
		if _, ok := d.applied[string(pos.hash)]; !ok {
			continue
		}
		report.RejectedEdits = append(report.RejectedEdits, &SORejectedEdit{
			OpHash:        pos.hash,
			Reason:        pos.reason,
			LostToPeerIds: d.concurrentAuthors(ev.Set, d.positions[:i], pos.hash, self),
		})
	}

	// Send it when it changed.
	if report.equal(d.shown) {
		return
	}
	d.shown = report
	d.report(report)
}

// concurrentAuthors returns the sorted authors, other than self, of the
// applied operations in earlier that the operation with hash h does not
// descend from.
func (d *BodyDriver) concurrentAuthors(set *SOOperationSet, earlier []bodyPosition, h []byte, self string) []string {
	// Collect the authors of the applied operations h does not descend from.
	ancestors := set.Ancestors(h)
	authors := make(map[string]struct{})
	for _, pos := range earlier {
		if pos.reason != "" {
			continue
		}
		if _, ok := ancestors[string(pos.hash)]; ok {
			continue
		}
		if author := set.Get(pos.hash).GetPeerId(); author != self {
			authors[author] = struct{}{}
		}
	}

	// Sort them so every member names them alike.
	if len(authors) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(authors))
}

// admittedPrefix returns how many of the first n positions keep their
// outcome under ev's config. A removal that pinned an author below
// operations already applied, such as a cut through a chosen nonce, rejects
// them now, so the body applies again from the first one and reaches the
// World a member replaying from the checkpoint reaches. A refused position
// applied nothing and stays. The check runs only while the config pins
// removed authors: one binary search per position above the checkpoint.
func (d *BodyDriver) admittedPrefix(ev *OrderEvent, n int) int {
	if len(ev.Config.GetRemovedAuthors()) == 0 {
		return n
	}
	for i, pos := range d.positions[:n] {
		if pos.reason != "" {
			continue
		}
		inner := ev.Set.Get(pos.hash)
		if inner != nil && !ev.Config.AdmitsOperation(inner.GetPeerId(), inner.GetNonce()) {
			return i
		}
	}
	return n
}

// hashes returns the hashes of the applied positions, in order.
func (d *BodyDriver) hashes() [][]byte {
	out := make([][]byte, len(d.positions))
	for i, pos := range d.positions {
		out[i] = pos.hash
	}
	return out
}

// _ is a type assertion
var _ checkpointJudge = (*BodyDriver)(nil)
