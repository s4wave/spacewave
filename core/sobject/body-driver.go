package sobject

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/pkg/errors"
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
// The driver serializes every call into the body. Its fields are owned by the
// goroutine running Execute or Step.
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
}

// NewBodyDriver returns the driver of body on so. report receives what the
// health shows of the replay, and may be nil.
func NewBodyDriver(le *logrus.Entry, so SharedObject, body Body, report func(*BodyReport)) *BodyDriver {
	return &BodyDriver{
		le:      le,
		so:      so,
		body:    body,
		report:  report,
		applied: make(map[string]struct{}),
	}
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

// Execute steps the body to each state of so until ctx ends. After an error
// it steps again on the next state, or after a backoff when no state arrives.
func (d *BodyDriver) Execute(ctx context.Context) error {
	// Watch the state.
	ctr, rel, err := d.so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer rel()

	// Wake a failed step after a backoff.
	wake := make(chan error, 1)
	delay := bodyRetryMin
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	retry := func() {
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(delay, func() {
			select {
			case wake <- errBodyRetry:
			default:
			}
		})
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
		if err := d.Step(ctx, snap); err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			d.le.WithError(err).Warn("replay stopped before the end of the order")
			retry()
			continue
		}
		delay = bodyRetryMin
	}
}

// Step brings the body to the order of snap: it installs snap's checkpoint
// when it moved, then applies every operation above it the body has not
// applied in this order. It returns the first error, with the body at the
// operations before it.
func (d *BodyDriver) Step(ctx context.Context, snap SharedObjectStateSnapshot) error {
	// Read the order.
	ev, err := ReadOrder(ctx, snap)
	if err != nil {
		return err
	}
	defer d.publishReport(ev)

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

	// Keep the applied operations the order still places first, and apply
	// the rest.
	keep := SharedPrefix(d.hashes(), ev.Order)
	d.positions = d.positions[:keep]
	d.set = ev.Set
	for i := keep; i < len(ev.Order); i++ {
		if err := d.apply(ctx, ev, i); err != nil {
			return err
		}
	}

	// Sign the checkpoint this device owes.
	return d.checkpoint(ctx, ev)
}

// checkpoint signs and adopts the checkpoint over ev's stable prefix when this
// device owes it and the body applied the prefix. A state that moved past the
// prefix meanwhile skips it; the next step checkpoints again.
func (d *BodyDriver) checkpoint(ctx context.Context, ev *OrderEvent) error {
	// Only the checkpointer of an owner-controlled Space signs, and not while
	// handing over. Group control decides checkpoints by vote.
	c := d.checkpointer
	self := d.so.GetPeerID().String()
	if c == nil || ev.Config.IsGroupControl() || ev.Config.Checkpointer() != self {
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
	data, err := d.body.Root(ctx, ev.Stable)
	if err != nil || data == nil {
		return err
	}

	// Sign and adopt it on the held state.
	prefix := ev.Order[:ev.Stable]
	height := ev.Checkpoint.GetHeight()
	err = c.host.UpdateSOState(kvtx.WithOrderedCommitContext(ctx), func(state *SOState) error {
		return c.adopt(ctx, d.le, d.so, state, prefix, data, height)
	})
	if errors.Is(err, errCheckpointStale) {
		return nil
	}
	return err
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
	report := &BodyReport{CheckpointMismatch: d.mismatch}
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

// hashes returns the hashes of the applied positions, in order.
func (d *BodyDriver) hashes() [][]byte {
	out := make([][]byte, len(d.positions))
	for i, pos := range d.positions {
		out[i] = pos.hash
	}
	return out
}
