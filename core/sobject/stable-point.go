package sobject

import (
	"maps"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// AcknowledgmentLag is the number of placed edits a roster member may leave
// unacknowledged before it writes an acknowledgment.
const AcknowledgmentLag = 64

// MinCheckpointOperations is the number of stable operations an owner waits
// for before it signs a checkpoint below them.
const MinCheckpointOperations = 32

// StablePoint returns the stable prefix of Order. While the sequence is open
// it is the sequenced prefix: the sequencer places every later operation after
// it. Otherwise it is the longest prefix whose operations every member of
// roster has built on. Each later operation of a roster member descends from
// every stable operation, so replay never places a new operation inside the
// stable prefix and a checkpoint may cover it. An empty roster makes the whole
// order stable.
func (s *SOOperationSet) StablePoint(roster []string) [][]byte {
	// Stop at the sequenced prefix while the sequence is open.
	placed := s.order()
	order := placed.order
	if s.sequence.open {
		return order[:placed.sequenced]
	}

	// Intersect what each roster member has built on.
	if len(roster) == 0 {
		return order
	}
	stable := s.builtOn(order, roster[0])
	for _, peerID := range roster[1:] {
		built := s.builtOn(order, peerID)
		maps.DeleteFunc(stable, func(key string, _ struct{}) bool {
			_, ok := built[key]
			return !ok
		})
	}

	// Take the longest prefix of the order inside that intersection.
	for i, h := range order {
		if _, ok := stable[string(h)]; !ok {
			return order[:i]
		}
	}
	return order
}

// NeedsAcknowledgment reports whether the writer peerID should write an
// acknowledgment. A sequencer makes operations stable without them. Otherwise
// every head must be placed, so the acknowledgment it would write is placed
// too. It then acknowledges when at least lag placed edits are not below its
// own latest operations, or when it has not built on an acknowledgment of
// checkpointer, which asks every member to answer at once. Other
// acknowledgments ask nothing, so members never answer each other's answers.
func (s *SOOperationSet) NeedsAcknowledgment(checkpointer, peerID string, lag int) bool {
	// An acknowledgment naming an unplaced head would itself wait unplaced.
	if s.sequence.sequencer != "" || len(s.UnplacedHeads()) != 0 {
		return false
	}

	// Answer the checkpointer, or count the placed edits peerID has not
	// built on.
	order := s.Order()
	built := s.builtOn(order, peerID)
	n := 0
	for _, h := range order {
		if _, ok := built[string(h)]; ok {
			continue
		}
		inner := s.ops[string(h)]
		switch {
		case !inner.IsAcknowledgment():
			n++
		case inner.GetPeerId() == checkpointer && peerID != checkpointer:
			return true
		}
	}
	return n >= lag
}

// UnplacedHeads returns the heads that replay has not placed and the
// checkpoint does not cover, in hash order. Each waits for an operation it
// names that this set does not hold.
func (s *SOOperationSet) UnplacedHeads() []*SOOperationPosition {
	// Keep the heads outside the order that the checkpoint does not cover.
	placed := make(map[string]struct{}, len(s.ops))
	for _, h := range s.Order() {
		placed[string(h)] = struct{}{}
	}
	var unplaced []*SOOperationPosition
	for _, head := range s.Heads() {
		if _, ok := placed[string(head.GetOpHash())]; !ok && !s.Covers(head.GetPeerId(), head.GetNonce()) {
			unplaced = append(unplaced, head)
		}
	}
	return unplaced
}

// BuiltOnLatest reports whether peerID has built on author's latest operation:
// a placed operation of peerID descends from it, or author has none above the
// checkpoint and peerID has a placed operation.
func (s *SOOperationSet) BuiltOnLatest(peerID, author string) bool {
	// A peer with no placed operation has built on nothing.
	built := s.builtOn(s.Order(), peerID)
	if len(built) == 0 {
		return false
	}

	// Look for author's latest operation above the checkpoint.
	nonce, head := s.AuthorHead(author)
	if nonce == 0 || s.Covers(author, nonce) {
		return true
	}
	_, ok := built[string(head)]
	return ok
}

// builtOn returns the placed operations every later operation of peerID
// descends from: the intersection, over peerID's placed operations that no
// other placed operation of peerID follows, of each one and its ancestors.
// It is empty when peerID has no placed operation.
func (s *SOOperationSet) builtOn(order [][]byte, peerID string) map[string]struct{} {
	// Find peerID's placed operations and the ones a later one follows.
	var own []string
	followed := make(map[string]struct{})
	for _, h := range order {
		inner := s.ops[string(h)]
		if inner.GetPeerId() == peerID {
			own = append(own, string(h))
			followed[string(inner.GetPrevOpHash())] = struct{}{}
		}
	}

	// Intersect the ancestry of each latest operation.
	var built map[string]struct{}
	for _, key := range own {
		if _, ok := followed[key]; ok {
			continue
		}
		ancestors := s.Ancestors([]byte(key))
		ancestors[key] = struct{}{}
		if built == nil {
			built = ancestors
			continue
		}
		maps.DeleteFunc(built, func(k string, _ struct{}) bool {
			_, ok := ancestors[k]
			return !ok
		})
	}
	if built == nil {
		built = make(map[string]struct{})
	}
	return built
}

// cover returns the author heads of a checkpoint covering the operations in
// covered: each author's highest covered operation, sorted by peer ID.
func (s *SOOperationSet) cover(covered [][]byte) []*SOOperationPosition {
	// Raise each author the checkpoint covers to its highest covered operation.
	heads := maps.Clone(s.authors)
	if heads == nil {
		heads = make(map[string]*SOOperationPosition)
	}
	for _, h := range covered {
		inner := s.ops[string(h)]
		if head, ok := heads[inner.GetPeerId()]; !ok || inner.GetNonce() > head.GetNonce() {
			heads[inner.GetPeerId()] = &SOOperationPosition{
				PeerId: inner.GetPeerId(),
				Nonce:  inner.GetNonce(),
				OpHash: slices.Clone(h),
			}
		}
	}
	return slices.SortedFunc(maps.Values(heads), func(a, b *SOOperationPosition) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})
}

// BuildStableCheckpoint signs, as the owner of privKey, the checkpoint after
// the held one that covers prefix, a stable prefix of the held operations.
// stateDataEnc is the state after prefix, encrypted with the key of the
// current key epoch.
func (s *SOState) BuildStableCheckpoint(
	sharedObjectID string,
	privKey crypto.PrivKey,
	prefix [][]byte,
	stateDataEnc []byte,
) (*SOCheckpoint, error) {
	inner, err := s.StableCheckpointInner(sharedObjectID, prefix, stateDataEnc)
	if err != nil {
		return nil, err
	}
	return BuildSOCheckpoint(privKey, inner)
}

// StableCheckpointInner returns the body of the checkpoint after the held one
// that covers prefix, a prefix of the held operations, with the state data
// stateDataEnc. A group decides it unsigned.
func (s *SOState) StableCheckpointInner(sharedObjectID string, prefix [][]byte, stateDataEnc []byte) (*SOCheckpointInner, error) {
	set, err := s.OperationSet(sharedObjectID)
	if err != nil {
		return nil, err
	}
	for _, h := range prefix {
		if set.Get(h) == nil {
			return nil, errors.New("checkpoint covers an operation the state does not hold")
		}
	}
	return s.checkpointInner(sharedObjectID, set.cover(prefix), set.SequenceHead(prefix), stateDataEnc)
}

// buildCheckpoint signs the checkpoint after the held one with the given
// author heads and sequence position.
func (s *SOState) buildCheckpoint(
	sharedObjectID string,
	privKey crypto.PrivKey,
	authors []*SOOperationPosition,
	sequence *SOSequenceHead,
	stateDataEnc []byte,
) (*SOCheckpoint, error) {
	inner, err := s.checkpointInner(sharedObjectID, authors, sequence, stateDataEnc)
	if err != nil {
		return nil, err
	}
	return BuildSOCheckpoint(privKey, inner)
}

// checkpointInner returns the body of the checkpoint after the held one with
// the given author heads and sequence position.
func (s *SOState) checkpointInner(
	sharedObjectID string,
	authors []*SOOperationPosition,
	sequence *SOSequenceHead,
	stateDataEnc []byte,
) (*SOCheckpointInner, error) {
	// Continue the chain from the held checkpoint.
	prev := s.GetCheckpoint()
	prevInner, err := s.GetCheckpointInner()
	if err != nil {
		return nil, err
	}
	if prevInner == nil {
		return nil, errors.New("state has no checkpoint to follow")
	}

	return &SOCheckpointInner{
		SharedObjectId:     sharedObjectID,
		Height:             prevInner.GetHeight() + 1,
		PrevCheckpointHash: prev.Hash(),
		ConfigHash:         s.GetConfig().GetConfigChainHash(),
		StateData:          stateDataEnc,
		ReplayVersion:      SOReplayVersion,
		KeyEpoch:           s.CurrentKeyEpoch().GetEpoch(),
		Authors:            authors,
		Sequence:           sequenceHeadOrNil(sequence),
	}, nil
}

// sequenceHeadOrNil returns head, or nil at height 0, so a checkpoint below
// every position encodes no position.
func sequenceHeadOrNil(head *SOSequenceHead) *SOSequenceHead {
	if head.GetHeight() == 0 {
		return nil
	}
	return head.CloneVT()
}
