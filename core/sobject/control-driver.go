package sobject

import (
	"bytes"
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// CheckpointJudge proposes and judges the checkpoints a group decides. The
// World engine implements it from its replay.
type CheckpointJudge interface {
	// ProposeCheckpoint returns the body of the checkpoint the local voter
	// proposes after the held one, or nil while too few operations are
	// stable or the replay has not reached them.
	ProposeCheckpoint(ctx context.Context, state *SOState) (*SOCheckpointInner, error)
	// JudgeCheckpoint reports whether inner is the checkpoint after the held
	// one that the local replay reached: the same covered operations, sequence
	// position and World. ok is false while the replay cannot tell.
	JudgeCheckpoint(ctx context.Context, state *SOState, inner *SOCheckpointInner) (valid, ok bool, err error)
}

// errControlWake wakes the driver to step again without a state change.
var errControlWake = errors.New("control wake")

// errControlStale reports that the state moved past the step a message was
// built for.
var errControlStale = errors.New("control step is stale")

// Control takes part, as one voter, in the decisions a group makes about a
// shared object: each decides its next control record or its next
// checkpoint. The voter's messages live in the shared object state, so a
// restart resumes from them with its lock. Only the round it waits in and its
// timeouts are in memory.
type Control struct {
	// le is the logger.
	le *logrus.Entry
	// host holds the state.
	host *SOHost
	// privKey signs as the voter.
	privKey crypto.PrivKey
	// self is the voter's peer ID.
	self string
	// judge proposes and judges checkpoints, or nil to take no part in them.
	judge CheckpointJudge
	// timeout is the wait of each step in a round.
	timeout func(round uint32) time.Duration
	// wake carries errControlWake to the driver.
	wake chan error
	// decision is the in-memory progress of the open decision, or nil.
	decision *controlDecision
}

// controlDecision is the in-memory progress of the open decision.
type controlDecision struct {
	// height and configHash name the decision.
	height     uint64
	configHash []byte
	// round is the round the voter is in.
	round uint32
	// deadlines holds when each armed timeout expires.
	deadlines map[roundTimer]time.Time
	// timers wake the driver at the deadlines.
	timers []*time.Timer
}

// NewControl builds the driver of privKey's votes on host's shared object.
// judge may be nil.
func NewControl(le *logrus.Entry, host *SOHost, privKey crypto.PrivKey, judge CheckpointJudge) (*Control, error) {
	self, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	return &Control{
		le:      le,
		host:    host,
		privKey: privKey,
		self:    self.String(),
		judge:   judge,
		timeout: defaultControlTimeout,
		wake:    make(chan error, 1),
	}, nil
}

// defaultControlTimeout waits two seconds in the first round and one more
// in each later round, so slow voters catch up.
func defaultControlTimeout(round uint32) time.Duration {
	return 2*time.Second + time.Duration(round)*time.Second
}

// SetTimeout replaces the wait of each step. Call it before Execute.
func (c *Control) SetTimeout(timeout func(round uint32) time.Duration) {
	c.timeout = timeout
}

// Wake has the driver step again, as when what the judge answers changed.
func (c *Control) Wake() {
	select {
	case c.wake <- errControlWake:
	default:
	}
}

// Execute steps the decisions each time the state changes or a timeout
// expires, until ctx ends.
func (c *Control) Execute(ctx context.Context) error {
	// Watch the state.
	ctr, rel, err := c.host.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer rel()
	defer c.reset()

	// Step each new state, and the held one on a wake.
	var state *SOState
	for {
		next, err := ctr.WaitValueChange(ctx, state, c.wake)
		if err != nil && !errors.Is(err, errControlWake) {
			return err
		}
		if next != nil {
			state = next
		}
		if state == nil {
			continue
		}
		if err := c.step(ctx, state); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.le.WithError(err).Warn("group decision step failed")
		}
	}
}

// step advances the open decision of state until it waits.
func (c *Control) step(ctx context.Context, state *SOState) error {
	// Only a voter under group control takes part.
	cfg := state.GetConfig()
	if !cfg.IsGroupControl() || cfg.VotingWeight(c.self) == 0 {
		c.reset()
		return nil
	}

	// Find the decision and its messages, and resume the voter's round.
	height, err := state.controlHeight()
	if err != nil {
		return err
	}
	d := c.open(height, cfg.GetConfigChainHash())
	msgs, err := state.OpenControlMessages()
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if m.Inner.GetPeerId() == c.self && m.Inner.GetRound() > d.round {
			d.round = m.Inner.GetRound()
		}
	}
	d.round = max(d.round, 1)

	// Step until the voter waits in its round.
	j := &controlJudgment{c: c, ctx: ctx, state: state, msgs: msgs, valid: make(map[controlValueKey]controlValidity)}
	for {
		out := stepRound(&roundInput{
			self:    c.self,
			cfg:     cfg,
			height:  height,
			msgs:    msgs,
			round:   d.round,
			valid:   j.judge,
			value:   j.value,
			expired: d.expired,
		})
		if j.err != nil {
			return j.err
		}
		c.arm(d, out.timers)

		// Act on the outcome. A write wakes the driver with the next state.
		switch {
		case out.round != 0:
			d.round = out.round
			continue
		case out.send != nil:
			return c.send(ctx, height, cfg.GetConfigChainHash(), out.send)
		case out.decided != nil:
			return c.decide(ctx, out.decidedKind, out.decided, out.commit)
		}
		return nil
	}
}

// open returns the progress of the open decision, starting over when the
// decision changed.
func (c *Control) open(height uint64, configHash []byte) *controlDecision {
	if d := c.decision; d != nil && d.height == height && bytes.Equal(d.configHash, configHash) {
		return d
	}
	c.reset()
	c.decision = &controlDecision{height: height, configHash: configHash, deadlines: make(map[roundTimer]time.Time)}
	return c.decision
}

// arm starts each timeout not yet armed.
func (c *Control) arm(d *controlDecision, timers []roundTimer) {
	for _, t := range timers {
		if _, ok := d.deadlines[t]; ok {
			continue
		}
		wait := c.timeout(t.round)
		d.deadlines[t] = time.Now().Add(wait)
		d.timers = append(d.timers, time.AfterFunc(wait, c.Wake))
	}
}

// expired reports whether the timeout t has passed.
func (d *controlDecision) expired(t roundTimer) bool {
	deadline, ok := d.deadlines[t]
	return ok && !time.Now().Before(deadline)
}

// reset stops the timers of the open decision and forgets it.
func (c *Control) reset() {
	if c.decision == nil {
		return
	}
	for _, t := range c.decision.timers {
		t.Stop()
	}
	c.decision = nil
}

// send signs msg for the decision and adds it to the state, unless the state
// moved on or already holds the voter's message of that step.
func (c *Control) send(ctx context.Context, height uint64, configHash []byte, msg *SOControlMessageInner) error {
	// Name the decision and sign.
	inner := msg.CloneVT()
	inner.SharedObjectId = c.host.GetSharedObjectID()
	inner.Height = height
	inner.ConfigHash = configHash
	if len(inner.GetValue()) != 0 {
		h, err := ControlValueHash(inner.GetKind(), inner.GetValue())
		if err != nil {
			return err
		}
		inner.ValueHash = h
	}
	signed, err := BuildSOControlMessage(c.privKey, inner)
	if err != nil {
		return err
	}

	// Add it under the host lock to the decision it was built for.
	err = c.host.UpdateSOState(ctx, func(state *SOState) error {
		// Skip a decision that closed since the step.
		if !state.controlOpen(inner) {
			return errControlStale
		}

		// Skip a step the voter already sent.
		msgs, err := state.OpenControlMessages()
		if err != nil {
			return err
		}
		for _, m := range msgs {
			if m.Inner.GetPeerId() == c.self && m.Inner.GetType() == inner.GetType() && m.Inner.GetRound() == inner.GetRound() {
				return errControlStale
			}
		}

		// Add the message.
		added, err := state.AddControlMessage(c.host.GetSharedObjectID(), signed)
		if err != nil {
			return err
		}
		if !added {
			return errControlStale
		}
		return nil
	})
	if errors.Is(err, errControlStale) {
		return nil
	}
	return err
}

// decide applies a decided value with the precommits that decided it.
func (c *Control) decide(ctx context.Context, kind SODecisionKind, value []byte, commit []*SOControlMessage) error {
	switch kind {
	case SODecisionKind_SO_DECISION_KIND_CONFIG:
		// Apply the record and hand the key to the members it admits.
		entry := &SOConfigChange{}
		if err := entry.UnmarshalVT(value); err != nil {
			return err
		}
		entry.Commit = commit
		err := c.host.ApplyConfigChange(ctx, entry, func(state *SOState) error {
			return reconcileGroupGrants(c.host.GetSharedObjectID(), state, c.privKey, c.self)
		})
		if errors.Is(err, ErrConfigChainHeadMismatch) {
			return nil
		}
		if err == nil {
			c.le.WithField("config-seqno", entry.GetConfigSeqno()).Debug("group decided a control record")
		}
		return err
	case SODecisionKind_SO_DECISION_KIND_CHECKPOINT:
		// Adopt the checkpoint the group decided.
		checkpoint := &SOCheckpoint{Inner: value, Commit: commit}
		return c.host.UpdateSOState(ctx, func(state *SOState) error {
			return state.AdoptCheckpoint(c.host.GetSharedObjectID(), checkpoint)
		})
	default:
		return errors.Errorf("unknown decision kind %v", kind)
	}
}

// controlJudgment judges and proposes the values of one state's decision.
type controlJudgment struct {
	c     *Control
	ctx   context.Context
	state *SOState
	msgs  []ControlMessage
	// valid caches the judgment of each value.
	valid map[controlValueKey]controlValidity
	// proposed, proposedKind and hasProposed cache the proposed value.
	proposed     []byte
	proposedKind SODecisionKind
	hasProposed  bool
	// err is the first error the judge or proposer hit.
	err error
}

// controlValueKey names a value by its kind and encoding.
type controlValueKey struct {
	kind  SODecisionKind
	value string
}

// judge returns the voter's judgment of value, judging each value once.
func (j *controlJudgment) judge(kind SODecisionKind, value []byte) controlValidity {
	// Reuse an earlier judgment.
	key := controlValueKey{kind: kind, value: string(value)}
	if v, ok := j.valid[key]; ok {
		return v
	}

	// Judge the value and keep the first error.
	v, err := j.judgeValue(kind, value)
	if err != nil && j.err == nil {
		j.err = err
	}
	j.valid[key] = v
	return v
}

// judgeValue judges value of kind.
func (j *controlJudgment) judgeValue(kind SODecisionKind, value []byte) (controlValidity, error) {
	switch kind {
	case SODecisionKind_SO_DECISION_KIND_CONFIG:
		entry := &SOConfigChange{}
		if err := entry.UnmarshalVT(value); err != nil {
			return controlInvalid, nil
		}
		return j.state.judgeControlRecord(j.c.host.GetSharedObjectID(), j.c.self, j.msgs, entry), nil
	case SODecisionKind_SO_DECISION_KIND_CHECKPOINT:
		if j.c.judge == nil {
			return controlUnknown, nil
		}
		inner := &SOCheckpointInner{}
		if err := inner.UnmarshalVT(value); err != nil {
			return controlInvalid, nil
		}
		valid, ok, err := j.c.judge.JudgeCheckpoint(j.ctx, j.state, inner)
		switch {
		case err != nil:
			return controlUnknown, err
		case !ok:
			return controlUnknown, nil
		case valid:
			return controlValid, nil
		default:
			return controlInvalid, nil
		}
	default:
		return controlInvalid, nil
	}
}

// value returns the value the voter proposes, computing it once.
func (j *controlJudgment) value() (SODecisionKind, []byte) {
	if !j.hasProposed {
		j.hasProposed = true
		var err error
		j.proposedKind, j.proposed, err = j.proposeValue()
		if err != nil && j.err == nil {
			j.err = err
		}
	}
	return j.proposedKind, j.proposed
}

// proposeValue returns the sealed control record the voter and a quorum
// agreed to, else the checkpoint the judge proposes, else nil. A change of
// members goes first, since checkpoints follow it.
func (j *controlJudgment) proposeValue() (SODecisionKind, []byte, error) {
	// Propose an agreed control record first.
	record, err := j.state.agreedControlRecord(j.c.host.GetSharedObjectID(), j.c.self, j.msgs)
	if err != nil || record != nil {
		return SODecisionKind_SO_DECISION_KIND_CONFIG, record, err
	}

	// Otherwise propose the checkpoint the judge reached.
	if j.c.judge == nil {
		return 0, nil, nil
	}
	inner, err := j.c.judge.ProposeCheckpoint(j.ctx, j.state)
	if err != nil || inner == nil {
		return 0, nil, err
	}
	value, err := inner.MarshalVT()
	return SODecisionKind_SO_DECISION_KIND_CHECKPOINT, value, err
}
