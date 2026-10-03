package sobject

import (
	"bytes"
	"cmp"
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// NewSOStateBlock constructs a new SOState block.
func NewSOStateBlock() block.Block {
	return &SOState{}
}

// UnmarshalSOState unmarshals a SOState from a block cursor.
func UnmarshalSOState(ctx context.Context, bcs *block.Cursor) (*SOState, error) {
	return block.UnmarshalBlock[*SOState](ctx, bcs, NewSOStateBlock)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (s *SOState) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (s *SOState) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// Validate checks the configuration, checkpoint, key epochs, operations,
// sequence positions and control messages.
// It authenticates every signature but does not check that a checkpoint or
// grant signer still holds authority: that is checked when they are adopted,
// so a signer's later departure leaves them valid.
func (s *SOState) Validate(sharedObjectID string) error {
	// The configuration must be structurally valid.
	if err := s.GetConfig().Validate(); err != nil {
		return errors.Wrap(err, "invalid config")
	}
	readers := make(map[string]bool, len(s.GetConfig().GetParticipants()))
	for _, participant := range s.GetConfig().GetParticipants() {
		readers[participant.GetPeerId()] = CanReadState(participant.GetRole())
	}

	// A held checkpoint is signed for this object.
	var checkpoint *SOCheckpointInner
	if s.GetCheckpoint() != nil {
		var err error
		checkpoint, _, err = s.GetCheckpoint().Verify(sharedObjectID)
		if err != nil {
			return errors.Wrap(err, "checkpoint")
		}
	}

	// Key epochs ascend, and each grants one key to each distinct reader.
	for i, epoch := range s.GetKeyEpochs() {
		if i > 0 && s.GetKeyEpochs()[i-1].GetEpoch() >= epoch.GetEpoch() {
			return errors.New("key epochs must be strictly sorted by epoch")
		}
		seen := make(map[string]struct{}, len(epoch.GetGrants()))
		for j, grant := range epoch.GetGrants() {
			if err := grant.Validate(); err != nil {
				return errors.Wrapf(err, "key_epochs[%d].grants[%d]", i, j)
			}
			if _, err := grant.Verify(sharedObjectID); err != nil {
				return errors.Wrapf(err, "key_epochs[%d].grants[%d]", i, j)
			}
			peerID := grant.GetPeerId()
			if _, ok := seen[peerID]; ok {
				return errors.Errorf("key_epochs[%d].grants[%d]: duplicate peer id: %s", i, j, peerID)
			}
			seen[peerID] = struct{}{}
			if !readers[peerID] {
				return errors.Errorf("key_epochs[%d]: peer %s has grant but no read access in participants", i, peerID)
			}
		}
	}

	// Operations are signed, above the checkpoint, and sorted by hash.
	if len(s.GetOps()) > MaxOperations {
		return errors.Wrap(ErrMaxCountExceeded, "operations")
	}
	set := NewSOOperationSet(sharedObjectID, checkpoint)
	var prev []byte
	for i, op := range s.GetOps() {
		added, err := set.Add(op)
		if err != nil {
			return errors.Wrapf(err, "ops[%d]", i)
		}
		if !added {
			return errors.Errorf("ops[%d]: duplicate or covered by the checkpoint", i)
		}
		h := op.Hash()
		if i > 0 && bytes.Compare(prev, h) >= 0 {
			return errors.New("ops must be strictly sorted by hash")
		}
		prev = h
	}

	// Positions are signed for this object, above the checkpoint, and sorted
	// by height, then by hash.
	if len(s.GetSequence()) > MaxOperations {
		return errors.Wrap(ErrMaxCountExceeded, "sequence")
	}
	var prevHeight uint64
	for i, record := range s.GetSequence() {
		inner, _, err := record.Verify(sharedObjectID)
		if err != nil {
			return errors.Wrapf(err, "sequence[%d]", i)
		}
		if inner.GetHeight() <= checkpoint.GetSequence().GetHeight() {
			return errors.Errorf("sequence[%d]: covered by the checkpoint", i)
		}
		h := record.Hash()
		if i > 0 && (inner.GetHeight() < prevHeight || inner.GetHeight() == prevHeight && bytes.Compare(prev, h) >= 0) {
			return errors.New("sequence must be strictly sorted by height, then by hash")
		}
		prevHeight, prev = inner.GetHeight(), h
	}

	// Control messages are signed for this object and sorted by hash.
	return s.validateControlMessages(sharedObjectID)
}

// GetCheckpointInner returns the held checkpoint's body, or nil before the
// first checkpoint.
func (s *SOState) GetCheckpointInner() (*SOCheckpointInner, error) {
	if s.GetCheckpoint() == nil {
		return nil, nil
	}
	return s.GetCheckpoint().UnmarshalInner()
}

// OperationSet returns the held operations as a verified set above the
// held checkpoint.
func (s *SOState) OperationSet(sharedObjectID string) (*SOOperationSet, error) {
	// Anchor the set at the checkpoint.
	checkpoint, err := s.GetCheckpointInner()
	if err != nil {
		return nil, err
	}

	// Verify every held operation into the set, and resolve the sequence.
	set := NewSOOperationSet(sharedObjectID, checkpoint)
	for i, op := range s.GetOps() {
		if _, err := set.Add(op); err != nil {
			return nil, errors.Wrapf(err, "ops[%d]", i)
		}
	}
	set.setSequence(s.GetConfig().GetSequencer(), s.GetSequence())
	return set, nil
}

// CurrentKeyEpoch returns the latest key epoch, or nil before the first.
func (s *SOState) CurrentKeyEpoch() *SOKeyEpoch {
	epochs := s.GetKeyEpochs()
	if len(epochs) == 0 {
		return nil
	}
	return epochs[len(epochs)-1]
}

// FindGrant returns the grant of peerID in the epoch, or nil.
func (e *SOKeyEpoch) FindGrant(peerID string) *SOGrant {
	for _, grant := range e.GetGrants() {
		if grant.GetPeerId() == peerID {
			return grant
		}
	}
	return nil
}

// GetKeyEpoch returns the key epoch with the given number, or nil.
func (s *SOState) GetKeyEpoch(epoch uint64) *SOKeyEpoch {
	i, ok := slices.BinarySearchFunc(s.GetKeyEpochs(), epoch, func(e *SOKeyEpoch, n uint64) int {
		return cmp.Compare(e.GetEpoch(), n)
	})
	if !ok {
		return nil
	}
	return s.GetKeyEpochs()[i]
}

// SetKeyEpoch adds epoch, or replaces the held epoch with its number,
// keeping the epochs sorted.
func (s *SOState) SetKeyEpoch(epoch *SOKeyEpoch) {
	i, ok := slices.BinarySearchFunc(s.KeyEpochs, epoch.GetEpoch(), func(e *SOKeyEpoch, n uint64) int {
		return cmp.Compare(e.GetEpoch(), n)
	})
	if ok {
		s.KeyEpochs[i] = epoch
		return
	}
	s.KeyEpochs = slices.Insert(s.KeyEpochs, i, epoch)
}

// NextOperationLink returns where peerID's next operation goes: one past the
// head of its chain, naming every other head of the operation DAG, under the
// current config and key epoch.
func (s *SOState) NextOperationLink(sharedObjectID, peerID string) (*SOOperationLink, error) {
	// Extend the author's own head.
	set, err := s.OperationSet(sharedObjectID)
	if err != nil {
		return nil, err
	}
	nonce, prev := set.AuthorHead(peerID)
	link := &SOOperationLink{
		Nonce:      nonce + 1,
		PrevOpHash: prev,
		ConfigHash: s.GetConfig().GetConfigChainHash(),
		KeyEpoch:   s.CurrentKeyEpoch().GetEpoch(),
	}

	// Name every other head.
	for _, head := range set.Heads() {
		if !bytes.Equal(head.GetOpHash(), prev) {
			link.Parents = append(link.Parents, head)
		}
	}
	return link, nil
}

// GetOperation returns peerID's held operation with localID, or nil.
func (s *SOState) GetOperation(peerID, localID string) (*SOOperation, error) {
	for _, op := range s.GetOps() {
		inner, err := op.UnmarshalInner()
		if err != nil {
			return nil, err
		}
		if inner.GetPeerId() == peerID && inner.GetLocalId() == localID {
			return op, nil
		}
	}
	return nil, nil
}

// AddOperation verifies op and adds it to the set. It reports false when the
// state already holds op or the checkpoint covers it. Authorization is decided
// by replay under the config op names.
func (s *SOState) AddOperation(sharedObjectID string, op *SOOperation) (bool, error) {
	// Admit an authentic operation above the checkpoint.
	inner, err := op.Verify(sharedObjectID)
	if err != nil {
		return false, err
	}
	checkpoint, err := s.GetCheckpointInner()
	if err != nil {
		return false, err
	}
	if NewSOOperationSet(sharedObjectID, checkpoint).Covers(inner.GetPeerId(), inner.GetNonce()) {
		return false, nil
	}

	// Insert it in hash order unless held. Hashing may suspend under GoScript,
	// which slices callbacks do not allow, so search with a plain loop.
	h := op.Hash()
	i, j := 0, len(s.Ops)
	for i < j {
		m := int(uint(i+j) >> 1)
		c := bytes.Compare(s.Ops[m].Hash(), h)
		if c == 0 {
			return false, nil
		}
		if c < 0 {
			i = m + 1
		} else {
			j = m
		}
	}
	if len(s.Ops) >= MaxOperations {
		return false, errors.Wrap(ErrMaxCountExceeded, "operations")
	}
	s.Ops = slices.Insert(s.Ops, i, op)
	return true, nil
}

// AddSequence verifies a sequence position and adds it. It reports false when
// the state holds it, the checkpoint covers it, or it is above the
// sequencer's start and the sequencer did not sign it. Whether it extends the
// sequence is decided when the sequence is resolved.
func (s *SOState) AddSequence(sharedObjectID string, record *SOSequence) (bool, error) {
	// Admit a position above the checkpoint that the start path or the
	// sequencer can account for.
	inner, signer, err := record.Verify(sharedObjectID)
	if err != nil {
		return false, err
	}
	checkpoint, err := s.GetCheckpointInner()
	if err != nil {
		return false, err
	}
	sequencer := s.GetConfig().GetSequencer()
	if inner.GetHeight() <= checkpoint.GetSequence().GetHeight() ||
		inner.GetHeight() > sequencer.GetStart().GetHeight() && signer != sequencer.GetPeerId() {
		return false, nil
	}

	// Insert it in height, then hash order unless held. Hashing may suspend
	// under GoScript, so search with a plain loop.
	h := record.Hash()
	i := 0
	for ; i < len(s.Sequence); i++ {
		held, err := s.Sequence[i].UnmarshalHeight()
		if err != nil {
			return false, err
		}
		if held > inner.GetHeight() {
			break
		}
		if held < inner.GetHeight() {
			continue
		}
		c := bytes.Compare(s.Sequence[i].Hash(), h)
		if c == 0 {
			return false, nil
		}
		if c > 0 {
			break
		}
	}
	if len(s.Sequence) >= MaxOperations {
		return false, errors.Wrap(ErrMaxCountExceeded, "sequence")
	}
	s.Sequence = slices.Insert(s.Sequence, i, record)
	return true, nil
}

// SequenceOperations signs, as the sequencer of privKey, a position for each
// placed operation after the sequence. It signs nothing unless privKey is the
// config's sequencer, every resolved position up to the sequencer's start is
// placed, and the sequencer has signed nothing above them, so it extends the
// one sequence every member resolves and never forks it. It returns the new
// positions.
func (s *SOState) SequenceOperations(sharedObjectID string, privKey crypto.PrivKey) ([]*SOSequence, error) {
	// Only the appointed sequencer extends a fully placed sequence.
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	sequencer := s.GetConfig().GetSequencer()
	if sequencer.GetPeerId() != peerID.String() {
		return nil, nil
	}
	set, err := s.OperationSet(sharedObjectID)
	if err != nil {
		return nil, err
	}
	placed := set.order()
	if !placed.complete || placed.tail.GetHeight() < sequencer.GetStart().GetHeight() {
		return nil, nil
	}

	// A position it already signed above the tail would fork the sequence.
	for _, record := range s.GetSequence() {
		inner, signer, err := record.Verify(sharedObjectID)
		if err == nil && signer == sequencer.GetPeerId() && inner.GetHeight() > placed.tail.GetHeight() {
			return nil, nil
		}
	}

	// Place the rest of the order after the tail.
	var added []*SOSequence
	prev := placed.tail
	for _, h := range placed.order[placed.sequenced:] {
		inner := set.Get(h)
		op := &SOOperationPosition{PeerId: inner.GetPeerId(), Nonce: inner.GetNonce(), OpHash: h}
		record, err := BuildSOSequence(sharedObjectID, privKey, prev, op)
		if err != nil {
			return nil, err
		}
		if _, err := s.AddSequence(sharedObjectID, record); err != nil {
			return nil, err
		}
		added = append(added, record)
		prev = &SOSequenceHead{Height: prev.GetHeight() + 1, Hash: record.Hash()}
	}
	return added, nil
}

// UnmarshalHeight decodes the position's height.
func (q *SOSequence) UnmarshalHeight() (uint64, error) {
	inner := &SOSequenceInner{}
	if err := inner.UnmarshalVT(q.GetInner()); err != nil {
		return 0, errors.Wrap(err, "unmarshal sequence inner")
	}
	return inner.GetHeight(), nil
}

// AdoptCheckpoint replaces the held checkpoint with next when an owner under
// the held config signed it and it descends from the held checkpoint. The held
// checkpoint, or one below its height, is ignored without an authority check:
// a lagging peer's state imports cleanly even after its signer left. The
// operations next covers leave the set.
func (s *SOState) AdoptCheckpoint(sharedObjectID string, next *SOCheckpoint) error {
	// Keep the held checkpoint unless next is its successor. A skipped height
	// cannot be linked and is taken on the owner's signature alone.
	inner, _, err := next.Verify(sharedObjectID)
	if err != nil {
		return err
	}
	if held := s.GetCheckpoint(); held != nil {
		heldInner, err := held.UnmarshalInner()
		if err != nil {
			return err
		}
		switch {
		case inner.GetHeight() < heldInner.GetHeight():
			return nil
		case inner.GetHeight() == heldInner.GetHeight():
			if !bytes.Equal(held.Hash(), next.Hash()) {
				return errors.New("checkpoint conflicts with the held checkpoint")
			}
			// Take the signatures of a remaining owner when the held signer left.
			if _, err := held.ValidateAuthority(sharedObjectID, s.GetConfig()); err != nil {
				if _, err := next.ValidateAuthority(sharedObjectID, s.GetConfig()); err == nil {
					s.Checkpoint = next.CloneVT()
				}
			}
			return nil
		case inner.GetHeight() == heldInner.GetHeight()+1 && !bytes.Equal(inner.GetPrevCheckpointHash(), held.Hash()):
			return errors.New("checkpoint does not follow the held checkpoint")
		}
	}

	// The held config authorizes the new checkpoint.
	if _, err := next.ValidateAuthority(sharedObjectID, s.GetConfig()); err != nil {
		return err
	}

	// Drop the operations the new checkpoint covers. Verification may suspend
	// under GoScript, so filter with a plain loop instead of slices.DeleteFunc.
	set := NewSOOperationSet(sharedObjectID, inner)
	s.Checkpoint = next.CloneVT()
	kept := s.Ops[:0]
	for _, op := range s.Ops {
		if added, err := set.Add(op); err == nil && added {
			kept = append(kept, op)
		}
	}
	clear(s.Ops[len(kept):])
	s.Ops = kept

	// Drop the positions it covers, and the ones the sequencer can no longer
	// account for.
	sequencer := s.GetConfig().GetSequencer()
	keptSeq := s.Sequence[:0]
	for _, record := range s.Sequence {
		seqInner, signer, err := record.Verify(sharedObjectID)
		if err != nil || seqInner.GetHeight() <= inner.GetSequence().GetHeight() {
			continue
		}
		if seqInner.GetHeight() > sequencer.GetStart().GetHeight() && signer != sequencer.GetPeerId() {
			continue
		}
		keptSeq = append(keptSeq, record)
	}
	clear(s.Sequence[len(keptSeq):])
	s.Sequence = keptSeq

	// Close the decision of this checkpoint.
	s.pruneControlMessages()
	return nil
}

// MergeKeyEpochs drops the held grants of peers that can no longer read, then
// adds the grants in epochs that the state lacks, checking each against the
// held config. A held grant for a recipient is kept unless its signer has left
// and epochs carries a replacement.
func (s *SOState) MergeKeyEpochs(sharedObjectID string, epochs []*SOKeyEpoch) error {
	// Drop the grants of removed readers.
	cfg := s.GetConfig()
	for _, epoch := range s.GetKeyEpochs() {
		epoch.Grants = slices.DeleteFunc(epoch.Grants, func(grant *SOGrant) bool {
			return !slices.ContainsFunc(cfg.GetParticipants(), func(p *SOParticipantConfig) bool {
				return p.GetPeerId() == grant.GetPeerId() && CanReadState(p.GetRole())
			})
		})
	}

	for _, epoch := range epochs {
		// Start from the held epoch, or a new one.
		merged := s.GetKeyEpoch(epoch.GetEpoch()).CloneVT()
		if merged == nil {
			merged = &SOKeyEpoch{Epoch: epoch.GetEpoch()}
		}

		// Add or replace each authorized grant.
		changed := false
		for _, grant := range epoch.GetGrants() {
			i := slices.IndexFunc(merged.GetGrants(), func(g *SOGrant) bool {
				return g.GetPeerId() == grant.GetPeerId()
			})
			if i != -1 && merged.GetGrants()[i].ValidateSignature(sharedObjectID, cfg) == nil {
				continue
			}
			if err := grant.ValidateSignature(sharedObjectID, cfg); err != nil {
				if i != -1 {
					continue
				}
				return errors.Wrapf(err, "key epoch %d grant", epoch.GetEpoch())
			}
			if i != -1 {
				merged.Grants[i] = grant.CloneVT()
			} else {
				merged.Grants = append(merged.Grants, grant.CloneVT())
			}
			changed = true
		}
		if changed {
			s.SetKeyEpoch(merged)
		}
	}
	return nil
}

// ValidateAuthority checks that the checkpoint and every grant were signed
// with authority under the held config. Validate checks only that the
// signatures are authentic.
func (s *SOState) ValidateAuthority(sharedObjectID string) error {
	cfg := s.GetConfig()
	if checkpoint := s.GetCheckpoint(); checkpoint != nil {
		if _, err := checkpoint.ValidateAuthority(sharedObjectID, cfg); err != nil {
			return err
		}
	}
	for _, epoch := range s.GetKeyEpochs() {
		for _, grant := range epoch.GetGrants() {
			if err := grant.ValidateSignature(sharedObjectID, cfg); err != nil {
				return errors.Wrapf(err, "key epoch %d grant", epoch.GetEpoch())
			}
		}
	}
	return nil
}
