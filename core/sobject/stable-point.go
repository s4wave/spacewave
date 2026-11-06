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

// RosterDropBytes is the encoded size of the operations above the checkpoint
// at which the checkpointer drops the roster members that lag behind it. It is
// a quarter of MaxSyncStateSize, which leaves room for the checkpoint and the
// edits that arrive while the checkpoint is built.
const RosterDropBytes = MaxSyncStateSize / 4

// Size returns the encoded size in bytes of the operations the set holds above
// the checkpoint.
func (s *SOOperationSet) Size() int {
	var size int
	for _, inner := range s.ops {
		size += inner.SizeVT()
	}
	return size
}

// StablePoint returns the stable prefix of Order. While the sequence is open
// it is the sequenced prefix: the sequencer places every later operation after
// it. Otherwise it is the longest prefix whose operations every member of
// roster has built on. Each later operation of a roster member descends from
// every stable operation, so replay never places a new operation inside the
// stable prefix and a checkpoint may cover it. An empty roster makes the whole
// order stable. StableCheckpoint.tla models the stable point and the
// checkpointer that adopts a checkpoint over it. Callers must not modify the
// result, which shares the set's order.
//
// Cost: one pass over the order and the links of each operation, with a word
// of bits per 64 latest operations of the roster, after the order the set
// computes once.
func (s *SOOperationSet) StablePoint(roster []string) [][]byte {
	// Stop at the sequenced prefix while the sequence is open.
	placed := s.order()
	order := placed.order
	if s.sequence.open {
		return order[:placed.sequenced:placed.sequenced]
	}
	if len(roster) == 0 {
		return order
	}
	n := s.stableLen(order, roster)
	return order[:n:n]
}

// stableLen returns the length of the longest prefix of order whose
// operations every member of roster has built on: each is an ancestor of, or
// is, every latest placed operation of each member, as builtOn defines them.
// It walks order once from the end, giving each operation the set of latest
// operations that descend from it, as bits: an operation's bits are its own and
// those of every placed operation linking to it, and order places every link
// before the operation that names it. A member with no placed operation has
// built on nothing.
func (s *SOOperationSet) stableLen(order [][]byte, roster []string) int {
	// Find each member's placed operations and the ones a later one of the
	// same member follows.
	members := make(map[string]struct{}, len(roster))
	for _, peerID := range roster {
		members[peerID] = struct{}{}
	}
	index := make(map[string]int, len(order))
	followed := make(map[string]struct{})
	placedBy := make(map[string]int, len(members))
	for i, h := range order {
		index[string(h)] = i
		inner := s.ops[string(h)]
		if _, ok := members[inner.GetPeerId()]; ok {
			placedBy[inner.GetPeerId()]++
			followed[string(inner.GetPrevOpHash())] = struct{}{}
		}
	}
	if len(placedBy) != len(members) {
		return 0
	}

	// Give each latest operation of a member its own bit.
	bit := make(map[int]int)
	for i, h := range order {
		_, own := members[s.ops[string(h)].GetPeerId()]
		if _, later := followed[string(h)]; own && !later {
			bit[i] = len(bit)
		}
	}
	words := (len(bit) + 63) / 64
	masks := make([]uint64, len(order)*words)

	// Carry each operation's bits to the operations it links to, from the
	// last placed one back.
	for i := len(order) - 1; i >= 0; i-- {
		mask := masks[i*words : (i+1)*words]
		if b, ok := bit[i]; ok {
			mask[b/64] |= 1 << (b % 64)
		}
		for _, link := range links(s.ops[string(order[i])]) {
			if s.Covers(link.GetPeerId(), link.GetNonce()) {
				continue
			}
			if j, ok := index[string(link.GetOpHash())]; ok {
				for w, m := range mask {
					masks[j*words+w] |= m
				}
			}
		}
	}

	// The stable prefix ends at the first operation some latest operation
	// does not descend from.
	for i := range order {
		mask := masks[i*words : (i+1)*words]
		for w, m := range mask {
			full := ^uint64(0)
			if w == words-1 && len(bit)%64 != 0 {
				full = 1<<(len(bit)%64) - 1
			}
			if m != full {
				return i
			}
		}
	}
	return len(order)
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
	return s.stableCheckpointInner(sharedObjectID, set, prefix, stateDataEnc)
}

// stableCheckpointInner is StableCheckpointInner over set, the verified
// operations s holds, so a caller with a verified set checks no signature
// again.
func (s *SOState) stableCheckpointInner(sharedObjectID string, set *SOOperationSet, prefix [][]byte, stateDataEnc []byte) (*SOCheckpointInner, error) {
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
