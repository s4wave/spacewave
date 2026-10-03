package sobject

import (
	"bytes"
	"slices"
)

// controlValidity is a voter's judgment of a proposed value.
type controlValidity int

const (
	// controlUnknown defers the judgment until the voter holds what it needs.
	controlUnknown controlValidity = iota
	// controlValid accepts the value.
	controlValid
	// controlInvalid rejects the value.
	controlInvalid
)

// roundTimer names one timeout of a round: the step it ends.
type roundTimer struct {
	// step is the step the timeout ends: PROPOSAL, PREVOTE or PRECOMMIT.
	step SOControlMessageType
	// round is the round of the timeout.
	round uint32
}

// roundInput is what one step of a group decision reads.
type roundInput struct {
	// self is the local voter.
	self string
	// cfg holds the voters and their weights.
	cfg *SharedObjectConfig
	// height is the decision's height.
	height uint64
	// msgs are the held messages of the decision.
	msgs []ControlMessage
	// round is the round the voter is in, at least one.
	round uint32
	// valid judges a proposed value of kind.
	valid func(kind SODecisionKind, value []byte) controlValidity
	// value returns the value the voter proposes, and its kind, when it holds
	// no value another round justified, or nil.
	value func() (SODecisionKind, []byte)
	// expired reports whether a timeout armed earlier has passed.
	expired func(roundTimer) bool
}

// roundOutput is the outcome of one step: at most one of round, send and
// decided, and the timeouts to arm.
type roundOutput struct {
	// round is a later round to move to, or zero.
	round uint32
	// send is the message to sign and send, or nil.
	send *SOControlMessageInner
	// decided is the decided value, or nil, and decidedKind its kind.
	decided     []byte
	decidedKind SODecisionKind
	// commit holds the precommits that decided it.
	commit []*SOControlMessage
	// timers are the timeouts the current step waits on.
	timers []roundTimer
}

// roundTally sums the weight of distinct voters behind each vote.
type roundTally struct {
	// cfg holds the weights.
	cfg *SharedObjectConfig
	// votes maps type, round and value hash to the voters behind it.
	votes map[roundVote]map[string]*SOControlMessage
	// any maps type and round to the voters with any vote.
	any map[roundVote]map[string]struct{}
	// rounds maps a round to the voters with any message in it.
	rounds map[uint32]map[string]struct{}
}

// roundVote names the votes of one type in one round, for one value hash.
type roundVote struct {
	typ   SOControlMessageType
	round uint32
	hash  string
}

// newRoundTally sums msgs.
func newRoundTally(cfg *SharedObjectConfig, msgs []ControlMessage) *roundTally {
	t := &roundTally{
		cfg:    cfg,
		votes:  make(map[roundVote]map[string]*SOControlMessage),
		any:    make(map[roundVote]map[string]struct{}),
		rounds: make(map[uint32]map[string]struct{}),
	}
	for _, m := range msgs {
		inner := m.Inner
		if inner.GetRound() == 0 {
			continue
		}
		addVoter(t.rounds, inner.GetRound(), inner.GetPeerId())
		if inner.GetType() != SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE &&
			inner.GetType() != SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT {
			continue
		}
		key := roundVote{typ: inner.GetType(), round: inner.GetRound(), hash: string(inner.GetValueHash())}
		if t.votes[key] == nil {
			t.votes[key] = make(map[string]*SOControlMessage)
		}
		t.votes[key][inner.GetPeerId()] = m.Msg
		addVoter(t.any, roundVote{typ: inner.GetType(), round: inner.GetRound()}, inner.GetPeerId())
	}
	return t
}

// addVoter adds peerID to the set at key.
func addVoter[K comparable](sets map[K]map[string]struct{}, key K, peerID string) {
	if sets[key] == nil {
		sets[key] = make(map[string]struct{})
	}
	sets[key][peerID] = struct{}{}
}

// weight sums the weight of voters.
func (t *roundTally) weight(voters []string) uint64 {
	var w uint64
	for _, v := range voters {
		w += t.cfg.VotingWeight(v)
	}
	return w
}

// quorum reports whether more than two thirds of the weight cast typ for
// hash in round. An empty hash is a nil vote.
func (t *roundTally) quorum(typ SOControlMessageType, round uint32, hash []byte) bool {
	votes := t.votes[roundVote{typ: typ, round: round, hash: string(hash)}]
	voters := make([]string, 0, len(votes))
	for v := range votes {
		voters = append(voters, v)
	}
	return HasQuorum(t.weight(voters), t.cfg.TotalVotingWeight())
}

// quorumAny reports whether more than two thirds of the weight cast typ in
// round, for any value.
func (t *roundTally) quorumAny(typ SOControlMessageType, round uint32) bool {
	return HasQuorum(t.weightOf(t.any[roundVote{typ: typ, round: round}]), t.cfg.TotalVotingWeight())
}

// weightOf sums the weight of a voter set.
func (t *roundTally) weightOf(set map[string]struct{}) uint64 {
	var w uint64
	for v := range set {
		w += t.cfg.VotingWeight(v)
	}
	return w
}

// commit returns the precommits for hash in round, sorted by voter.
func (t *roundTally) commit(round uint32, hash []byte) []*SOControlMessage {
	// Sort the voters who precommitted hash.
	votes := t.votes[roundVote{typ: SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT, round: round, hash: string(hash)}]
	voters := make([]string, 0, len(votes))
	for v := range votes {
		voters = append(voters, v)
	}
	slices.Sort(voters)

	// Collect their precommits in that order.
	out := make([]*SOControlMessage, len(voters))
	for i, v := range voters {
		out[i] = votes[v]
	}
	return out
}

// roundProposer returns the voter that proposes in round at height.
func roundProposer(cfg *SharedObjectConfig, height uint64, round uint32) string {
	voters := cfg.Voters()
	if len(voters) == 0 {
		return ""
	}
	return voters[(height+uint64(round))%uint64(len(voters))]
}

// stepRound takes one step of Tendermint (Buchman, Kwon and Milosevic, "The
// latest gossip on BFT consensus", 2018, Algorithm 1) for in. The voter's own
// messages are its persisted state: its lock is its highest non-nil
// precommit, and it never sends a second message of one type in one round.
// It acts only while a value or a round message exists, so an idle decision
// arms no timeouts.
func stepRound(in *roundInput) *roundOutput {
	// Sum the votes, and find this voter's messages and lock.
	t := newRoundTally(in.cfg, in.msgs)
	sent := make(map[roundVote]struct{})
	var lockRound uint32
	var lockHash []byte
	var active bool
	for _, m := range in.msgs {
		inner := m.Inner
		active = active || inner.GetRound() != 0
		if inner.GetPeerId() != in.self {
			continue
		}
		sent[roundVote{typ: inner.GetType(), round: inner.GetRound()}] = struct{}{}
		if inner.GetType() == SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT && len(inner.GetValueHash()) != 0 && inner.GetRound() >= lockRound {
			lockRound, lockHash = inner.GetRound(), inner.GetValueHash()
		}
	}
	has := func(typ SOControlMessageType, round uint32) bool {
		_, ok := sent[roundVote{typ: typ, round: round}]
		return ok
	}

	// A quorum of precommits for a proposed value decides it in any round.
	for _, m := range in.msgs {
		inner := m.Inner
		if inner.GetType() != SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL {
			continue
		}
		if t.quorum(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT, inner.GetRound(), inner.GetValueHash()) {
			return &roundOutput{
				decided:     inner.GetValue(),
				decidedKind: inner.GetKind(),
				commit:      t.commit(inner.GetRound(), inner.GetValueHash()),
			}
		}
	}

	// Stay idle until a value or a round message exists.
	var fresh []byte
	var freshKind SODecisionKind
	if !active {
		if freshKind, fresh = in.value(); fresh == nil {
			return &roundOutput{}
		}
	}

	// Skip to a later round that more than a third of the weight reached.
	r := in.round
	var skip uint32
	for round, voters := range t.rounds {
		if round > r && round > skip && exceedsFaultyWeight(t.weightOf(voters), in.cfg.TotalVotingWeight()) {
			skip = round
		}
	}
	if skip != 0 {
		return &roundOutput{round: skip}
	}

	// A round whose precommit timeout passed ends.
	precommitTimer := roundTimer{step: SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT, round: r}
	if in.expired(precommitTimer) {
		return &roundOutput{round: r + 1}
	}
	out := &roundOutput{}
	if t.quorumAny(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT, r) {
		out.timers = append(out.timers, precommitTimer)
	}

	// The round's proposal: the smallest value hash its proposer sent.
	proposer := roundProposer(in.cfg, in.height, r)
	proposal := roundProposal(in.msgs, proposer, r)

	// The proposer proposes the value a polka justified, or a new one.
	vote := func(typ SOControlMessageType, hash []byte) *SOControlMessageInner {
		return &SOControlMessageInner{Type: typ, Round: r, ValueHash: hash}
	}
	if proposer == in.self && !has(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL, r) && !has(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, r) {
		kind, value, validRound := roundValidValue(in, t, r)
		if value == nil {
			if fresh == nil {
				freshKind, fresh = in.value()
			}
			kind, value = freshKind, fresh
		}
		if value != nil {
			send := vote(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL, nil)
			send.Kind, send.Value, send.ValidRound = kind, value, validRound
			out.send = send
			return out
		}
	}

	// Prevote the proposal unless locked on another value, or nil when it is
	// invalid or its timeout passed.
	if !has(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, r) {
		proposeTimer := roundTimer{step: SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL, round: r}
		if proposal != nil {
			h := proposal.GetValueHash()
			vr := proposal.GetValidRound()
			justified := vr == 0 || t.quorum(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, vr, h)
			if justified {
				switch in.valid(proposal.GetKind(), proposal.GetValue()) {
				case controlValid:
					if lockRound == 0 || lockRound <= vr && vr != 0 || bytes.Equal(lockHash, h) {
						out.send = vote(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, h)
					} else {
						out.send = vote(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, nil)
					}
					return out
				case controlInvalid:
					out.send = vote(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, nil)
					return out
				}
			}
		}
		if in.expired(proposeTimer) {
			out.send = vote(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, nil)
			return out
		}
		out.timers = append(out.timers, proposeTimer)
		return out
	}

	// Precommit a proposal with a polka, which locks it, or nil on a nil
	// polka or when the prevote timeout passed.
	if !has(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT, r) {
		if proposal != nil && t.quorum(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, r, proposal.GetValueHash()) &&
			in.valid(proposal.GetKind(), proposal.GetValue()) == controlValid {
			out.send = vote(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT, proposal.GetValueHash())
			return out
		}
		prevoteTimer := roundTimer{step: SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, round: r}
		if t.quorum(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, r, nil) || in.expired(prevoteTimer) {
			out.send = vote(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT, nil)
			return out
		}
		if t.quorumAny(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, r) {
			out.timers = append(out.timers, prevoteTimer)
		}
	}
	return out
}

// roundProposal returns the proposal proposer sent in round, choosing the
// smallest value hash when it sent several, or nil.
func roundProposal(msgs []ControlMessage, proposer string, round uint32) *SOControlMessageInner {
	var out *SOControlMessageInner
	for _, m := range msgs {
		inner := m.Inner
		if inner.GetType() != SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL ||
			inner.GetRound() != round || inner.GetPeerId() != proposer {
			continue
		}
		if out == nil || bytes.Compare(inner.GetValueHash(), out.GetValueHash()) < 0 {
			out = inner
		}
	}
	return out
}

// roundValidValue returns the kind and valid value of the latest round before
// r whose proposal won a polka, and that round, or nil.
func roundValidValue(in *roundInput, t *roundTally, r uint32) (SODecisionKind, []byte, uint32) {
	// Keep the latest justified proposal of an earlier round.
	var kind SODecisionKind
	var value []byte
	var validRound uint32
	for _, m := range in.msgs {
		inner := m.Inner
		vr := inner.GetRound()
		if inner.GetType() != SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL || vr >= r || vr <= validRound {
			continue
		}
		if inner.GetPeerId() != roundProposer(in.cfg, in.height, vr) ||
			!t.quorum(SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE, vr, inner.GetValueHash()) ||
			in.valid(inner.GetKind(), inner.GetValue()) != controlValid {
			continue
		}
		kind, value, validRound = inner.GetKind(), inner.GetValue(), vr
	}
	return kind, value, validRound
}
