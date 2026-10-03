package sobject

import (
	"bytes"
	"slices"

	"github.com/pkg/errors"
)

// maxControlRounds bounds the rounds of one voter's messages a state keeps in
// the open decision. Older rounds are dropped first, except the voter's lock.
const maxControlRounds = 32

// maxControlAgreements bounds the changes one voter agrees to under a config.
const maxControlAgreements = 4

// maxControlEvidence bounds the messages one voter has for one step of one
// round. A second message is evidence that the voter equivocated.
const maxControlEvidence = 2

// MaxControlMessages bounds the control messages a state holds.
const MaxControlMessages = MaxParticipants * (maxControlRounds*3*maxControlEvidence + maxControlAgreements) * 2

// ControlMessage is a verified control message with its body.
type ControlMessage struct {
	// Msg is the signed message.
	Msg *SOControlMessage
	// Inner is its verified body.
	Inner *SOControlMessageInner
}

// controlHeight returns the height of the open decision: one more than the
// held checkpoint's.
func (s *SOState) controlHeight() (uint64, error) {
	inner, err := s.GetCheckpointInner()
	if err != nil {
		return 0, err
	}
	if inner == nil {
		return 0, errors.New("state has no checkpoint to follow")
	}
	return inner.GetHeight() + 1, nil
}

// controlOpen reports whether inner belongs to the open decision under the
// held config and checkpoint, or agrees to a change of the held config.
func (s *SOState) controlOpen(inner *SOControlMessageInner) bool {
	if !s.GetConfig().IsGroupControl() || !bytes.Equal(inner.GetConfigHash(), s.GetConfig().GetConfigChainHash()) {
		return false
	}
	if inner.GetType() == SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_AGREE {
		return true
	}
	height, err := s.controlHeight()
	return err == nil && inner.GetHeight() == height
}

// body decodes a held message, which was verified when it was added.
func (m *SOControlMessage) body() (*SOControlMessageInner, error) {
	inner := &SOControlMessageInner{}
	if err := inner.UnmarshalVT(m.GetInner()); err != nil {
		return nil, errors.Wrap(err, "unmarshal control message inner")
	}
	return inner, nil
}

// OpenControlMessages returns the held messages of the open decision and the
// agreements to changes of the held config.
func (s *SOState) OpenControlMessages() ([]ControlMessage, error) {
	var out []ControlMessage
	for _, msg := range s.GetControlMessages() {
		inner, err := msg.body()
		if err != nil {
			return nil, err
		}
		if s.controlOpen(inner) {
			out = append(out, ControlMessage{Msg: msg, Inner: inner})
		}
	}
	return out, nil
}

// AddControlMessage adds a voter's message to the open decision. It reports
// false for a message the state holds, belongs to a closed decision, or
// exceeds the evidence kept of its signer. Under group control only a voter of
// the held config signs messages.
func (s *SOState) AddControlMessage(sharedObjectID string, msg *SOControlMessage) (bool, error) {
	// Find its place, and skip a held message, which was verified when added.
	h := msg.Hash()
	i, found := slices.BinarySearchFunc(s.GetControlMessages(), h, func(m *SOControlMessage, h []byte) int {
		return bytes.Compare(m.Hash(), h)
	})
	if found {
		return false, nil
	}

	// Authenticate the voter.
	inner, err := msg.Verify(sharedObjectID)
	if err != nil {
		return false, err
	}
	if s.GetConfig().VotingWeight(inner.GetPeerId()) == 0 {
		return false, errors.Errorf("%s does not vote under the held config", inner.GetPeerId())
	}
	if !s.controlOpen(inner) {
		return false, nil
	}

	// Keep a bounded record of the signer's messages.
	var agreements, slot int
	for _, held := range s.GetControlMessages() {
		other, err := held.body()
		if err != nil {
			return false, err
		}
		if other.GetPeerId() != inner.GetPeerId() || other.GetHeight() != inner.GetHeight() {
			continue
		}
		if other.GetType() == SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_AGREE {
			agreements++
		}
		if other.GetType() == inner.GetType() && other.GetRound() == inner.GetRound() {
			slot++
		}
	}
	if inner.GetType() == SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_AGREE {
		if agreements >= maxControlAgreements {
			return false, nil
		}
	} else if slot >= maxControlEvidence {
		return false, nil
	}
	s.ControlMessages = slices.Insert(s.ControlMessages, i, msg.CloneVT())
	return true, s.trimControlRounds(inner)
}

// trimControlRounds drops the oldest rounds of the signer of inner beyond
// maxControlRounds in the open decision, keeping the signer's highest non-nil
// precommit, which is its lock.
func (s *SOState) trimControlRounds(inner *SOControlMessageInner) error {
	// Collect the signer's rounds and lock.
	var rounds []uint32
	var lock []byte
	var lockRound uint32
	type entry struct {
		inner *SOControlMessageInner
		hash  []byte
	}
	var mine []entry
	for _, msg := range s.GetControlMessages() {
		other, err := msg.body()
		if err != nil {
			return err
		}
		if other.GetPeerId() != inner.GetPeerId() || other.GetHeight() != inner.GetHeight() || other.GetRound() == 0 {
			continue
		}
		mine = append(mine, entry{inner: other, hash: msg.Hash()})
		if !slices.Contains(rounds, other.GetRound()) {
			rounds = append(rounds, other.GetRound())
		}
		if other.GetType() == SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT && len(other.GetValueHash()) != 0 && other.GetRound() >= lockRound {
			lock, lockRound = msg.Hash(), other.GetRound()
		}
	}
	if len(rounds) <= maxControlRounds {
		return nil
	}

	// Drop the messages of the oldest rounds.
	slices.Sort(rounds)
	floor := rounds[len(rounds)-maxControlRounds]
	drop := make(map[string]struct{})
	for _, e := range mine {
		if e.inner.GetRound() < floor && !bytes.Equal(e.hash, lock) {
			drop[string(e.hash)] = struct{}{}
		}
	}
	s.ControlMessages = slices.DeleteFunc(s.ControlMessages, func(m *SOControlMessage) bool {
		_, ok := drop[string(m.Hash())]
		return ok
	})
	return nil
}

// MergeControlMessages adds the messages of a peer's state that belong to
// an open decision, skipping the rest.
func (s *SOState) MergeControlMessages(sharedObjectID string, msgs []*SOControlMessage) {
	for _, msg := range msgs {
		_, _ = s.AddControlMessage(sharedObjectID, msg)
	}
}

// pruneControlMessages drops the messages of decisions the held config and
// checkpoint closed.
func (s *SOState) pruneControlMessages() {
	s.ControlMessages = slices.DeleteFunc(s.ControlMessages, func(msg *SOControlMessage) bool {
		inner, err := msg.body()
		return err != nil || !s.controlOpen(inner)
	})
}

// validateControlMessages checks that the control messages are signed for
// this object, bounded, and sorted by hash.
func (s *SOState) validateControlMessages(sharedObjectID string) error {
	if len(s.GetControlMessages()) > MaxControlMessages {
		return errors.Wrap(ErrMaxCountExceeded, "control messages")
	}
	var prev []byte
	for i, msg := range s.GetControlMessages() {
		if _, err := msg.Verify(sharedObjectID); err != nil {
			return errors.Wrapf(err, "control_messages[%d]", i)
		}
		h := msg.Hash()
		if i > 0 && bytes.Compare(prev, h) >= 0 {
			return errors.New("control messages must be strictly sorted by hash")
		}
		prev = h
	}
	return nil
}
