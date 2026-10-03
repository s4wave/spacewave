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

// StablePoint returns the stable prefix of Order: the longest prefix whose
// operations every member of roster has built on. Each later operation of a
// roster member descends from every stable operation, so replay never places
// a new operation inside the stable prefix and a checkpoint may cover it. An
// empty roster makes the whole order stable.
func (s *SOOperationSet) StablePoint(roster []string) [][]byte {
	// Intersect what each roster member has built on.
	order := s.Order()
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

// NeedsAcknowledgment reports whether peerID should write an acknowledgment.
// It must be on roster, and every head must be placed, so the acknowledgment
// it would write is placed too. It then acknowledges when at least lag placed
// edits are not below its own latest operations, or when it has not built on
// an acknowledgment of checkpointer, which asks every member to answer at
// once. Other acknowledgments ask nothing, so members never answer each
// other's answers.
func (s *SOOperationSet) NeedsAcknowledgment(roster []string, checkpointer, peerID string, lag int) bool {
	// Only roster members hold back the stable point.
	if !slices.Contains(roster, peerID) {
		return false
	}

	// An acknowledgment naming an unplaced head would itself wait unplaced.
	order := s.Order()
	placed := make(map[string]struct{}, len(order))
	for _, h := range order {
		placed[string(h)] = struct{}{}
	}
	for _, h := range s.Heads() {
		if _, ok := placed[string(h)]; !ok && !s.below(string(h)) {
			return false
		}
	}

	// Answer the checkpointer, or count the placed edits peerID has not
	// built on.
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

// cover returns the frontier and author heads of a checkpoint covering the
// operations in prefix, which must be a prefix of Order. The frontier holds
// the heads of the covered operations and every covered operation a held
// operation above prefix names, so each held operation still finds its links.
func (s *SOOperationSet) cover(prefix [][]byte) ([][]byte, []*SOCheckpointAuthor) {
	// Index the covered operations.
	covered := make(map[string]struct{}, len(prefix))
	for _, h := range prefix {
		covered[string(h)] = struct{}{}
	}

	// Keep each covered or checkpoint hash a covered operation leaves unnamed,
	// or an uncovered operation names.
	namedBelow := make(map[string]struct{})
	namedAbove := make(map[string]struct{})
	for key, inner := range s.ops {
		named := namedAbove
		if _, ok := covered[key]; ok {
			named = namedBelow
		}
		named[string(inner.GetPrevOpHash())] = struct{}{}
		for _, parent := range inner.GetParentHashes() {
			named[string(parent)] = struct{}{}
		}
	}
	candidates := slices.Collect(maps.Keys(covered))
	candidates = slices.AppendSeq(candidates, maps.Keys(s.frontier))
	var frontier [][]byte
	for _, key := range candidates {
		_, inner := namedBelow[key]
		_, outer := namedAbove[key]
		if !inner || outer {
			frontier = append(frontier, []byte(key))
		}
	}

	// Raise each author to its highest covered operation.
	heads := maps.Clone(s.authors)
	if heads == nil {
		heads = make(map[string]*SOCheckpointAuthor)
	}
	for _, h := range prefix {
		inner := s.ops[string(h)]
		if head, ok := heads[inner.GetPeerId()]; !ok || inner.GetNonce() > head.GetNonce() {
			heads[inner.GetPeerId()] = &SOCheckpointAuthor{
				PeerId: inner.GetPeerId(),
				Nonce:  inner.GetNonce(),
				OpHash: slices.Clone(h),
			}
		}
	}
	authors := slices.SortedFunc(maps.Values(heads), func(a, b *SOCheckpointAuthor) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})
	return sortedOperationHashes(frontier, nil), authors
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
	set, err := s.OperationSet(sharedObjectID)
	if err != nil {
		return nil, err
	}
	frontier, authors := set.cover(prefix)
	return s.buildCheckpoint(sharedObjectID, privKey, frontier, authors, stateDataEnc)
}

// buildCheckpoint signs the checkpoint after the held one with the given
// frontier and author heads.
func (s *SOState) buildCheckpoint(
	sharedObjectID string,
	privKey crypto.PrivKey,
	frontier [][]byte,
	authors []*SOCheckpointAuthor,
	stateDataEnc []byte,
) (*SOCheckpoint, error) {
	// Continue the chain from the held checkpoint.
	prev := s.GetCheckpoint()
	prevInner, err := s.GetCheckpointInner()
	if err != nil {
		return nil, err
	}
	if prevInner == nil {
		return nil, errors.New("state has no checkpoint to follow")
	}

	return BuildSOCheckpoint(privKey, &SOCheckpointInner{
		SharedObjectId:     sharedObjectID,
		Height:             prevInner.GetHeight() + 1,
		PrevCheckpointHash: prev.Hash(),
		ConfigHash:         s.GetConfig().GetConfigChainHash(),
		Frontier:           frontier,
		StateData:          stateDataEnc,
		ReplayVersion:      SOReplayVersion,
		KeyEpoch:           s.CurrentKeyEpoch().GetEpoch(),
		Authors:            authors,
	})
}
