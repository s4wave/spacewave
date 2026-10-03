package sobject

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
)

// roundFixture is a group of four equal voters deciding at one height.
type roundFixture struct {
	t      *testing.T
	state  *SOState
	height uint64
	// keys maps each voter to its key.
	keys map[string]crypto.PrivKey
}

// newRoundFixture builds a group of four voters.
func newRoundFixture(t *testing.T) *roundFixture {
	// Hand control to four voters.
	peers := createMockPeers(t, 4)
	keys := mustPrivKeys(t, peers)
	state := newGroupState(t, peers, 4)
	height, err := state.controlHeight()
	if err != nil {
		t.Fatal(err)
	}

	// Keep each voter's key.
	f := &roundFixture{t: t, state: state, height: height, keys: make(map[string]crypto.PrivKey)}
	for i, p := range peers {
		f.keys[p.GetPeerID().String()] = keys[i]
	}
	return f
}

// voter returns the i-th voter in proposer order.
func (f *roundFixture) voter(i int) string {
	return f.state.GetConfig().Voters()[i]
}

// proposer returns the proposer of round.
func (f *roundFixture) proposer(round uint32) string {
	return roundProposer(f.state.GetConfig(), f.height, round)
}

// msg signs one message of the decision as voter.
func (f *roundFixture) msg(voter string, typ SOControlMessageType, round uint32, value []byte, validRound uint32) ControlMessage {
	// Name the decision and the step.
	inner := &SOControlMessageInner{
		SharedObjectId: mockSharedObjectID,
		Height:         f.height,
		ConfigHash:     f.state.GetConfig().GetConfigChainHash(),
		Type:           typ,
		Round:          round,
		ValidRound:     validRound,
	}

	// Name the value, which only a proposal carries.
	if len(value) != 0 {
		h, err := ControlValueHash(SODecisionKind_SO_DECISION_KIND_CHECKPOINT, value)
		if err != nil {
			f.t.Fatal(err)
		}
		inner.ValueHash = h
		if typ == SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL {
			inner.Kind = SODecisionKind_SO_DECISION_KIND_CHECKPOINT
			inner.Value = value
		}
	}

	// Sign it as voter.
	signed, err := BuildSOControlMessage(f.keys[voter], inner)
	if err != nil {
		f.t.Fatal(err)
	}
	verified, err := signed.Verify(mockSharedObjectID)
	if err != nil {
		f.t.Fatal(err)
	}
	return ControlMessage{Msg: signed, Inner: verified}
}

// step steps voter self in round over msgs, judging every value valid and
// proposing none, with no timeout expired.
func (f *roundFixture) step(self string, round uint32, msgs []ControlMessage) *roundOutput {
	return stepRound(&roundInput{
		self:    self,
		cfg:     f.state.GetConfig(),
		height:  f.height,
		msgs:    msgs,
		round:   round,
		valid:   func(SODecisionKind, []byte) controlValidity { return controlValid },
		value:   func() (SODecisionKind, []byte) { return 0, nil },
		expired: func(roundTimer) bool { return false },
	})
}

// checkpointValue returns a distinct checkpoint body.
func checkpointValue(t *testing.T, height uint64) []byte {
	return mustMarshalVT(t, &SOCheckpointInner{SharedObjectId: mockSharedObjectID, Height: height})
}

// TestStepRoundLock checks that a voter locked on one value prevotes nil for
// another, until a later polka for the other justifies leaving the lock.
func TestStepRoundLock(t *testing.T) {
	// The voter is the proposer of round 4.
	f := newRoundFixture(t)
	x, y := checkpointValue(t, 1), checkpointValue(t, 2)
	yHash, _ := ControlValueHash(SODecisionKind_SO_DECISION_KIND_CHECKPOINT, y)
	self := f.proposer(4)

	// The other three voters can form a polka.
	var others []string
	for i := range 4 {
		if v := f.voter(i); v != self {
			others = append(others, v)
		}
	}

	// The voter locked on x in round 1.
	msgs := []ControlMessage{
		f.msg(f.proposer(1), SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL, 1, x, 0),
		f.msg(self, SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, 1, x, 0),
		f.msg(self, SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT, 1, x, 0),
	}

	// A fresh proposal of y in round 3 gets a nil prevote.
	fresh := append(msgs, f.msg(f.proposer(3), SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL, 3, y, 0))
	out := f.step(self, 3, fresh)
	if out.send == nil || out.send.GetType() != SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE || len(out.send.GetValueHash()) != 0 {
		t.Fatalf("locked voter sent %v; want a nil prevote", out.send)
	}

	// A polka for y in round 2 justifies y proposed again in round 3.
	justified := append([]ControlMessage(nil), msgs...)
	justified = append(justified, f.msg(f.proposer(2), SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL, 2, y, 0))
	for _, v := range others {
		justified = append(justified, f.msg(v, SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, 2, y, 0))
	}
	justified = append(justified, f.msg(f.proposer(3), SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL, 3, y, 2))
	out = f.step(self, 3, justified)
	if out.send == nil || !bytes.Equal(out.send.GetValueHash(), yHash) {
		t.Fatalf("voter sent %v after a later polka; want a prevote for y", out.send)
	}

	// As proposer of round 4 it proposes the polka's value with its round.
	out = f.step(self, 4, justified[:len(justified)-1])
	if out.send == nil || out.send.GetType() != SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL ||
		!bytes.Equal(out.send.GetValue(), y) || out.send.GetValidRound() != 2 {
		t.Fatalf("proposer sent %v; want y with valid round 2", out.send)
	}
}

// TestStepRoundSkipAndDecide checks that a voter joins a later round more
// than a third of the weight reached, and decides a value more than two
// thirds precommitted in any round, with those precommits as the commit.
func TestStepRoundSkipAndDecide(t *testing.T) {
	// The voter is the first.
	f := newRoundFixture(t)
	x := checkpointValue(t, 1)
	self := f.voter(0)

	// One voter of four in round 5 is not enough to skip; two are.
	late := []ControlMessage{f.msg(f.voter(1), SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, 5, nil, 0)}
	if out := f.step(self, 1, late); out.round != 0 {
		t.Fatalf("skipped to round %d on one voter", out.round)
	}
	late = append(late, f.msg(f.voter(2), SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, 5, nil, 0))
	if out := f.step(self, 1, late); out.round != 5 {
		t.Fatalf("moved to round %d; want 5", out.round)
	}

	// Three precommits of four decide x proposed in round 2.
	msgs := []ControlMessage{f.msg(f.proposer(2), SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL, 2, x, 0)}
	for i := 1; i < 4; i++ {
		msgs = append(msgs, f.msg(f.voter(i), SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT, 2, x, 0))
	}
	out := f.step(self, 1, msgs)
	if !bytes.Equal(out.decided, x) || out.decidedKind != SODecisionKind_SO_DECISION_KIND_CHECKPOINT || len(out.commit) != 3 {
		t.Fatalf("decided %x of kind %v with %d precommits; want x with 3", out.decided, out.decidedKind, len(out.commit))
	}
	h, _ := ControlValueHash(SODecisionKind_SO_DECISION_KIND_CHECKPOINT, x)
	if err := VerifyCommit(mockSharedObjectID, f.state.GetConfig(), f.height, h, out.commit); err != nil {
		t.Fatalf("commit does not verify: %v", err)
	}

	// With nothing to decide and no messages, the voter idles.
	if out := f.step(self, 1, nil); out.send != nil || out.round != 0 || len(out.timers) != 0 {
		t.Fatalf("idle voter acted: %+v", out)
	}
}

// TestAddControlMessageBounds checks that a state keeps two conflicting
// messages of a voter's step as evidence and no more, rejects a non-voter, and
// skips a message of another decision.
func TestAddControlMessageBounds(t *testing.T) {
	// The fourth voter equivocates.
	f := newRoundFixture(t)
	d := f.voter(3)

	// Two conflicting prevotes are evidence; a third is dropped.
	for i, want := range []bool{true, true, false} {
		msg := f.msg(d, SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, 1, checkpointValue(t, uint64(i+1)), 0)
		added, err := f.state.AddControlMessage(mockSharedObjectID, msg.Msg)
		if err != nil {
			t.Fatal(err)
		}
		if added != want {
			t.Fatalf("prevote %d added %v; want %v", i, added, want)
		}
	}

	// A message of another height is skipped.
	other := f.msg(d, SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, 1, nil, 0)
	inner := other.Inner.CloneVT()
	inner.Height++
	stale, err := BuildSOControlMessage(f.keys[d], inner)
	if err != nil {
		t.Fatal(err)
	}
	if added, err := f.state.AddControlMessage(mockSharedObjectID, stale); err != nil || added {
		t.Fatalf("stale message added %v: %v", added, err)
	}

	// A non-voter cannot add messages.
	outsider := mustPrivKeys(t, createMockPeers(t, 1))[0]
	foreign, err := BuildSOControlMessage(outsider, other.Inner.CloneVT())
	if err == nil {
		_, err = f.state.AddControlMessage(mockSharedObjectID, foreign)
	}
	if err == nil {
		t.Fatal("a non-voter added a message")
	}
	if err := f.state.Validate(mockSharedObjectID); err != nil {
		t.Fatal(err)
	}
}
