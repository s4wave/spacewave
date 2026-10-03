package sobject

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// errNothingNew skips the write of a delivery that added no message.
var errNothingNew = errors.New("nothing new")

// groupHost is one device's copy of a shared object, locked like a provider.
type groupHost struct {
	// host serves the state.
	host *SOHost
	// ctr holds the state.
	ctr *ccontainer.CContainer[*SOState]
	// mtx is the provider lock.
	mtx sync.Mutex
	// changesMtx guards changes.
	changesMtx sync.Mutex
	// changes are the config changes the device applied, which peers sync.
	changes []*SOConfigChange
}

// newGroupHost hosts state on one device.
func newGroupHost(ctx context.Context, state *SOState) *groupHost {
	g := &groupHost{ctr: ccontainer.NewCContainer(state)}
	g.host = NewSOHost(ctx,
		func(context.Context, string, func()) (ccontainer.Watchable[*SOState], func(), error) {
			return g.ctr, func() {}, nil
		},
		func(context.Context, string) (SOStateLock, error) {
			g.mtx.Lock()
			return NewSOStateLock(g.ctr.GetValue(), func(_ context.Context, state *SOState, changes ...*SOConfigChange) error {
				// Record the changes, as the config chain keeps them, and
				// publish the state.
				g.changesMtx.Lock()
				g.changes = append(g.changes, changes...)
				g.changesMtx.Unlock()
				g.ctr.SetValue(state)
				return nil
			}, g.mtx.Unlock), nil
		},
		mockSharedObjectID,
	)
	return g
}

// deliver adds the messages g accepts, writing only when one is new.
func (g *groupHost) deliver(ctx context.Context, msgs []*SOControlMessage) error {
	err := g.host.UpdateSOState(ctx, func(state *SOState) error {
		var added bool
		for _, msg := range msgs {
			ok, err := state.AddControlMessage(mockSharedObjectID, msg)
			added = added || err == nil && ok
		}
		if !added {
			return errNothingNew
		}
		return nil
	})
	if errors.Is(err, errNothingNew) {
		return nil
	}
	return err
}

// syncChanges applies the config changes of from that g lacks, with the key
// epochs from holds at the same config, as devices syncing the state do.
func (g *groupHost) syncChanges(ctx context.Context, from *groupHost) error {
	// Read the changes from holds.
	from.changesMtx.Lock()
	changes := slices.Clone(from.changes)
	from.changesMtx.Unlock()
	source := from.ctr.GetValue()

	// Apply each one g lacks.
	for _, entry := range changes {
		if entry.GetConfigSeqno() <= g.ctr.GetValue().GetConfig().GetConfigChainSeqno() {
			continue
		}
		err := g.host.ApplyConfigChange(ctx, entry, func(state *SOState) error {
			if bytes.Equal(state.GetConfig().GetConfigChainHash(), source.GetConfig().GetConfigChainHash()) {
				state.KeyEpochs = source.CloneVT().GetKeyEpochs()
			}
			return nil
		})
		if err != nil && !errors.Is(err, ErrConfigChainHeadMismatch) {
			return err
		}
	}
	return nil
}

// waitState waits until g's state satisfies cond.
func (g *groupHost) waitState(ctx context.Context, cond func(*SOState) bool) (*SOState, error) {
	return g.ctr.WaitValueWithValidator(ctx, func(state *SOState) (bool, error) {
		return state != nil && cond(state), nil
	}, nil)
}

// groupNet gossips the config changes and control messages of every host to
// every host, as devices syncing the state do.
type groupNet struct {
	// hosts are the devices.
	hosts []*groupHost
	// observe sees each new state of host i and returns messages to inject.
	observe func(i int, state *SOState) []*SOControlMessage
	// wg tracks the gossip goroutines.
	wg sync.WaitGroup
}

// start gossips until ctx ends.
func (n *groupNet) start(ctx context.Context, t *testing.T) {
	for i := range n.hosts {
		n.wg.Go(func() {
			if err := n.gossip(ctx, i); err != nil && ctx.Err() == nil {
				t.Error(err)
			}
		})
	}
}

// gossip sends each new state of host i to every host.
func (n *groupNet) gossip(ctx context.Context, i int) error {
	var state *SOState
	for {
		next, err := n.hosts[i].ctr.WaitValueChange(ctx, state, nil)
		if err != nil {
			return err
		}
		state = next
		msgs := state.GetControlMessages()
		if n.observe != nil {
			msgs = append(slices.Clone(msgs), n.observe(i, state)...)
		}
		for _, h := range n.hosts {
			if err := h.syncChanges(ctx, n.hosts[i]); err != nil {
				return err
			}
			if err := h.deliver(ctx, msgs); err != nil {
				return err
			}
		}
	}
}

// groupVoter runs one honest voter's driver and restarts it on request.
type groupVoter struct {
	// host is the voter's device.
	host *groupHost
	// priv signs as the voter.
	priv crypto.PrivKey
	// mtx guards the fields below.
	mtx sync.Mutex
	// cancel and done stop and join the running driver.
	cancel context.CancelFunc
	done   chan struct{}
	// restarts counts the restarts of a running driver.
	restarts int
}

// start runs a new driver for the voter.
func (v *groupVoter) start(ctx context.Context, t *testing.T) {
	v.mtx.Lock()
	defer v.mtx.Unlock()
	v.startLocked(ctx, t)
}

// startLocked runs a new driver while v.mtx is held.
func (v *groupVoter) startLocked(ctx context.Context, t *testing.T) {
	// Build a driver with short rounds.
	c, err := NewControl(logrus.NewEntry(logrus.New()), v.host.host, v.priv, nil)
	if err != nil {
		t.Error(err)
		return
	}
	c.SetTimeout(func(round uint32) time.Duration {
		return 50*time.Millisecond + time.Duration(round)*50*time.Millisecond
	})

	// Run it until stopped.
	ctx, v.cancel = context.WithCancel(ctx)
	v.done = make(chan struct{})
	go func() {
		defer close(v.done)
		_ = c.Execute(ctx)
	}()
}

// stop stops the voter's driver and waits for it to exit.
func (v *groupVoter) stop() {
	v.mtx.Lock()
	defer v.mtx.Unlock()
	v.stopLocked()
}

// stopLocked stops the driver while v.mtx is held.
func (v *groupVoter) stopLocked() {
	if v.cancel != nil {
		v.cancel()
		<-v.done
		v.cancel = nil
	}
}

// restartOnce restarts the running driver the first time it is called while
// one runs.
func (v *groupVoter) restartOnce(ctx context.Context, t *testing.T) {
	// Restart only a running driver, once.
	v.mtx.Lock()
	defer v.mtx.Unlock()
	if v.cancel == nil || v.restarts != 0 || ctx.Err() != nil {
		return
	}

	// Stop it and start a fresh one.
	v.restarts++
	v.stopLocked()
	v.startLocked(ctx, t)
}

// equivocator plays a dishonest voter: it proposes every agreed change in its
// rounds and prevotes and precommits every proposal it sees.
type equivocator struct {
	// priv signs as the voter.
	priv crypto.PrivKey
	// self is the voter's peer ID.
	self string
	// mtx guards the fields below.
	mtx sync.Mutex
	// sent names the messages it signed.
	sent map[string]struct{}
	// msgs are the messages it signed.
	msgs []*SOControlMessage
}

// observe signs the dishonest messages state calls for.
func (e *equivocator) observe(t *testing.T, state *SOState) []*SOControlMessage {
	// Act only under group control.
	e.mtx.Lock()
	defer e.mtx.Unlock()
	cfg := state.GetConfig()
	if !cfg.IsGroupControl() {
		return e.msgs
	}

	// Find the open decision and its messages.
	height, err := state.controlHeight()
	if err != nil {
		t.Error(err)
		return e.msgs
	}
	msgs, err := state.OpenControlMessages()
	if err != nil {
		t.Error(err)
		return e.msgs
	}
	sign := func(key string, inner *SOControlMessageInner) {
		// Sign each message once.
		if _, ok := e.sent[key]; ok {
			return
		}
		e.sent[key] = struct{}{}

		// Name the decision, sign and keep it.
		inner.SharedObjectId = mockSharedObjectID
		inner.Height = height
		inner.ConfigHash = cfg.GetConfigChainHash()
		msg, err := BuildSOControlMessage(e.priv, inner)
		if err != nil {
			t.Error(err)
			return
		}
		e.msgs = append(e.msgs, msg)
	}

	// Propose every agreed change in each round it proposes.
	agreements, err := controlAgreements(cfg, msgs)
	if err != nil {
		t.Error(err)
		return e.msgs
	}
	for _, m := range msgs {
		r := m.Inner.GetRound()
		if r == 0 || roundProposer(cfg, height, r) != e.self {
			continue
		}
		for _, a := range agreements {
			value, err := state.sealControlRecord(a.value)
			if err != nil {
				t.Error(err)
				continue
			}
			h, err := ControlValueHash(SODecisionKind_SO_DECISION_KIND_CONFIG, value)
			if err != nil {
				t.Error(err)
				continue
			}
			sign("proposal"+string(cfg.GetConfigChainHash())+string(rune(r))+string(h), &SOControlMessageInner{
				Type:      SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL,
				Kind:      SODecisionKind_SO_DECISION_KIND_CONFIG,
				Round:     r,
				Value:     value,
				ValueHash: h,
			})
		}
	}

	// Vote for every proposal.
	for _, m := range msgs {
		if m.Inner.GetType() != SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL {
			continue
		}
		for _, typ := range []SOControlMessageType{
			SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE,
			SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT,
		} {
			key := typ.String() + string(cfg.GetConfigChainHash()) + string(rune(m.Inner.GetRound())) + string(m.Inner.GetValueHash())
			sign(key, &SOControlMessageInner{
				Type:      typ,
				Round:     m.Inner.GetRound(),
				ValueHash: m.Inner.GetValueHash(),
			})
		}
	}
	return e.msgs
}

// newGroupState returns a state under group control whose voters are the
// first voters peers, all writers, with the rest readers.
func newGroupState(t *testing.T, peers []peer.Peer, voters int) *SOState {
	// Make the first peer the owner and the first voters writers.
	roles := make([]SOParticipantRole, len(peers))
	for i := range roles {
		roles[i] = SOParticipantRole_SOParticipantRole_READER
		if i < voters {
			roles[i] = SOParticipantRole_SOParticipantRole_WRITER
		}
	}
	roles[0] = SOParticipantRole_SOParticipantRole_OWNER

	// The owner hands control to the group.
	state, _ := newTestSOState(t, peers, roles...)
	g := newGroupHost(t.Context(), state)
	if err := SetSOControl(t.Context(), g.host, SOControl_SO_CONTROL_GROUP, mustPrivKeys(t, peers[:1])[0]); err != nil {
		t.Fatal(err)
	}

	// Every writer votes.
	state = g.ctr.GetValue()
	if got := len(state.GetConfig().Voters()); got != voters {
		t.Fatalf("group has %d voters; want %d", got, voters)
	}
	return state
}

// countAgreements returns the agreement messages state holds.
func countAgreements(state *SOState) int {
	var n int
	for _, msg := range state.GetControlMessages() {
		inner, err := msg.body()
		if err == nil && inner.GetType() == SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_AGREE {
			n++
		}
	}
	return n
}

// TestGroupControlSplitDishonestRestart checks that four voters change who is
// in the shared object when the honest voters split between two changes, one
// voter equivocates, and the voter both changes need restarts while locked:
// every honest voter applies the same one change.
func TestGroupControlSplitDishonestRestart(t *testing.T) {
	// A, B, C and D vote; R reads; N is not yet a member.
	peers := createMockPeers(t, 6)
	keys := mustPrivKeys(t, peers)
	ids := make([]string, len(peers))
	for i, p := range peers {
		ids[i] = p.GetPeerID().String()
	}
	state := newGroupState(t, peers[:5], 4)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	hosts := []*groupHost{newGroupHost(ctx, state), newGroupHost(ctx, state), newGroupHost(ctx, state)}
	seqno := state.GetConfig().GetConfigChainSeqno()

	// D equivocates; B restarts after its first lock.
	voters := make([]*groupVoter, len(hosts))
	for i, h := range hosts {
		voters[i] = &groupVoter{host: h, priv: keys[i]}
	}
	d := &equivocator{priv: keys[3], self: ids[3], sent: make(map[string]struct{})}
	net := &groupNet{hosts: hosts}
	net.observe = func(i int, state *SOState) []*SOControlMessage {
		if i == 1 && holdsLock(state, ids[1]) {
			voters[1].restartOnce(ctx, t)
		}
		return d.observe(t, state)
	}
	net.start(ctx, t)
	t.Cleanup(func() {
		cancel()
		for _, v := range voters {
			v.stop()
		}
		net.wg.Wait()
	})

	// A, B and D agree to admit N; B, C and D agree to remove R.
	admit := func(h *groupHost, key crypto.PrivKey) {
		_, err := AddSOParticipant(ctx, h.host, mockSharedObjectID, key, "", ids[5], peers[5].GetPubKey(), SOParticipantRole_SOParticipantRole_WRITER, "", "")
		if !errors.Is(err, ErrAwaitingGroup) {
			t.Fatalf("admit: %v", err)
		}
	}
	remove := func(h *groupHost, key crypto.PrivKey) {
		removed, err := RemoveSOParticipants(ctx, h.host, ids[4:5], key, nil)
		if !errors.Is(err, ErrAwaitingGroup) || len(removed) != 1 {
			t.Fatalf("remove: %v %v", removed, err)
		}
	}

	// Each voter agrees on its own device; D agrees through A's and C's.
	admit(hosts[0], keys[0])
	admit(hosts[1], keys[1])
	admit(hosts[0], keys[3])
	remove(hosts[1], keys[1])
	remove(hosts[2], keys[2])
	remove(hosts[2], keys[3])

	// Every device holds the six agreements.
	for _, h := range hosts {
		if _, err := h.waitState(ctx, func(s *SOState) bool { return countAgreements(s) == 6 }); err != nil {
			t.Fatal(err)
		}
	}

	// The voters decide one of them.
	for _, v := range voters {
		v.start(ctx, t)
	}

	// Every device applies the same change.
	var heads []*SOState
	for _, h := range hosts {
		s, err := h.waitState(ctx, func(s *SOState) bool { return s.GetConfig().GetConfigChainSeqno() > seqno })
		if err != nil {
			logControlMessages(t, hosts)
			t.Fatal(err)
		}
		heads = append(heads, s)
	}

	// They agree on the chain head.
	for i, s := range heads[1:] {
		if !bytes.Equal(s.GetConfig().GetConfigChainHash(), heads[0].GetConfig().GetConfigChainHash()) {
			t.Fatalf("voter %d applied another change", i+1)
		}
	}

	// The head applies exactly one of the two changes.
	cfg := heads[0].GetConfig()
	admitted := cfg.VotingWeight(ids[5]) == 1
	removed := !slices.ContainsFunc(cfg.GetParticipants(), func(p *SOParticipantConfig) bool { return p.GetPeerId() == ids[4] })
	if admitted == removed {
		t.Fatalf("admitted %v, removed %v; want exactly one change", admitted, removed)
	}
	if cfg.GetConfigChainSeqno() != seqno+1 {
		t.Fatalf("config seqno %d; want %d", cfg.GetConfigChainSeqno(), seqno+1)
	}

	// Note when B decided before it could restart.
	voters[1].mtx.Lock()
	defer voters[1].mtx.Unlock()
	if voters[1].restarts == 0 {
		t.Log("B decided before it held a lock to restart on")
	}
}

// logControlMessages logs the control messages each host holds.
func logControlMessages(t *testing.T, hosts []*groupHost) {
	for i, h := range hosts {
		for _, msg := range h.ctr.GetValue().GetControlMessages() {
			inner, err := msg.body()
			if err != nil {
				t.Error(err)
				continue
			}
			t.Logf("host %d: %s %v round %d valid round %d %x", i, inner.GetPeerId(), inner.GetType(), inner.GetRound(), inner.GetValidRound(), inner.GetValueHash())
		}
	}
}

// holdsLock reports whether state holds a non-nil precommit of voter.
func holdsLock(state *SOState, voter string) bool {
	for _, msg := range state.GetControlMessages() {
		inner, err := msg.body()
		if err == nil && inner.GetPeerId() == voter && inner.GetType() == SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT && len(inner.GetValueHash()) != 0 {
			return true
		}
	}
	return false
}

// TestGroupControlThreeVotersWaitForAll checks that with three voters a
// change two agree to waits, and applies once the third agrees. The new
// member's key comes from the voters, not from its own signature.
func TestGroupControlThreeVotersWaitForAll(t *testing.T) {
	// A, B and C vote; N is not yet a member.
	peers := createMockPeers(t, 4)
	keys := mustPrivKeys(t, peers)
	newID := peers[3].GetPeerID().String()
	state := newGroupState(t, peers[:3], 3)
	seqno := state.GetConfig().GetConfigChainSeqno()

	// Each voter has a device on one network.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	hosts := []*groupHost{newGroupHost(ctx, state), newGroupHost(ctx, state), newGroupHost(ctx, state)}
	net := &groupNet{hosts: hosts}
	net.start(ctx, t)
	voters := make([]*groupVoter, len(hosts))
	for i, h := range hosts {
		voters[i] = &groupVoter{host: h, priv: keys[i]}
	}
	t.Cleanup(func() {
		cancel()
		for _, v := range voters {
			v.stop()
		}
		net.wg.Wait()
	})

	// Each voter agrees to admit N on its own device.
	admit := func(i int) {
		_, err := AddSOParticipant(ctx, hosts[i].host, mockSharedObjectID, keys[i], "", newID, peers[3].GetPubKey(), SOParticipantRole_SOParticipantRole_WRITER, "", "")
		if !errors.Is(err, ErrAwaitingGroup) {
			t.Fatalf("admit: %v", err)
		}
	}

	// With two of three agreeing, no voter proposes.
	admit(0)
	admit(1)
	for i, h := range hosts {
		s, err := h.waitState(ctx, func(s *SOState) bool { return countAgreements(s) == 2 })
		if err != nil {
			t.Fatal(err)
		}
		c, err := NewControl(logrus.NewEntry(logrus.New()), h.host, keys[i], nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.step(ctx, s); err != nil {
			t.Fatal(err)
		}
		c.reset()
		if got := len(h.ctr.GetValue().GetControlMessages()); got != 2 {
			t.Fatalf("voter %d holds %d messages after a step; want the 2 agreements", i, got)
		}
	}

	// Once C agrees, every voter applies the change and N can read.
	for _, v := range voters {
		v.start(ctx, t)
	}
	admit(2)
	for i, h := range hosts {
		s, err := h.waitState(ctx, func(s *SOState) bool { return s.GetConfig().GetConfigChainSeqno() > seqno })
		if err != nil {
			t.Fatal(err)
		}
		if s.GetConfig().VotingWeight(newID) != 1 {
			t.Fatalf("voter %d did not admit N as a voter", i)
		}
		if s.CurrentKeyEpoch().FindGrant(newID) == nil {
			t.Fatalf("voter %d did not grant N the key", i)
		}
		if err := s.Validate(mockSharedObjectID); err != nil {
			t.Fatalf("voter %d state: %v", i, err)
		}
	}
}
