package sobject

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// soSequenceHashDomain separates sequence position hashes from other SHA-256
// digests.
const soSequenceHashDomain = "spacewave/sharedobject/sequence/v1"

// SOSequenceSignatureContext is the signature context of every sequence
// position. The signed body binds the object, so the context needs no
// per-position data.
var SOSequenceSignatureContext = baseCryptoContext + "sequence_signature"

// HashSOSequenceInner returns the identity of a sequence position from its
// signed body.
func HashSOSequenceInner(inner []byte) []byte {
	// Prefix the length-framed body with the domain.
	preimage := make([]byte, 0, len(soSequenceHashDomain)+8+len(inner))
	preimage = append(preimage, soSequenceHashDomain...)
	preimage = binary.BigEndian.AppendUint64(preimage, uint64(len(inner)))
	preimage = append(preimage, inner...)
	digest := sha256.Sum256(preimage)
	return digest[:]
}

// Hash returns the identity of the position.
func (q *SOSequence) Hash() []byte {
	return HashSOSequenceInner(q.GetInner())
}

// BuildSOSequence signs, as the sequencer of privKey, the position after prev
// that places op. prev is nil or height 0 before the first position.
func BuildSOSequence(sharedObjectID string, privKey crypto.PrivKey, prev *SOSequenceHead, op *SOOperationPosition) (*SOSequence, error) {
	// Bind the body to the object, the previous position, the operation and
	// the sequencer.
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "sequencer peer ID")
	}
	inner := &SOSequenceInner{
		PeerId:         peerID.String(),
		SharedObjectId: sharedObjectID,
		Height:         prev.GetHeight() + 1,
		PrevHash:       prev.GetHash(),
		Op:             op,
	}
	if err := inner.Validate(); err != nil {
		return nil, err
	}
	innerData, err := inner.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal sequence inner")
	}

	// Sign exactly the encoded body.
	sig, err := peer.NewSignature(SOSequenceSignatureContext, privKey, hash.RecommendedHashType, innerData, true)
	if err != nil {
		return nil, errors.Wrap(err, "sign sequence position")
	}
	return &SOSequence{Inner: innerData, Signature: sig}, nil
}

// Validate checks the position body's structure.
func (i *SOSequenceInner) Validate() error {
	// The body names an object, its sequencer and a height.
	if i.GetSharedObjectId() == "" {
		return ErrEmptySharedObjectID
	}
	if _, err := parsePeerIDField(i.GetPeerId()); err != nil {
		return errors.Wrap(err, "sequence peer_id")
	}
	if i.GetHeight() == 0 {
		return errors.New("sequence height must be positive")
	}

	// The first position names nothing before it; every later one names its
	// predecessor.
	if i.GetHeight() == 1 && len(i.GetPrevHash()) != 0 {
		return errors.New("first sequence position must not name a previous position")
	}
	if i.GetHeight() > 1 && len(i.GetPrevHash()) != sha256.Size {
		return errors.New("sequence prev_hash must be a 32-byte hash")
	}

	// The position places one real operation.
	return errors.Wrap(validatePosition(i.GetOp()), "sequence op")
}

// Verify authenticates the position as signed for sharedObjectID, and returns
// its body and its signer. It does not check that the signer is the sequencer.
func (q *SOSequence) Verify(sharedObjectID string) (*SOSequenceInner, string, error) {
	// Decode the well-formed body bound to this object.
	if len(q.GetInner()) == 0 {
		return nil, "", ErrEmptyInnerData
	}
	if err := q.GetSignature().Validate(); err != nil {
		return nil, "", err
	}
	inner := &SOSequenceInner{}
	if err := inner.UnmarshalVT(q.GetInner()); err != nil {
		return nil, "", errors.Wrap(err, "unmarshal sequence inner")
	}
	if err := inner.Validate(); err != nil {
		return nil, "", errors.Wrap(err, "invalid sequence inner")
	}
	if inner.GetSharedObjectId() != sharedObjectID {
		return nil, "", errors.New("sequence position is bound to another shared object")
	}

	// Recover the signer from the included public key.
	sig := q.GetSignature()
	pubKey, err := sig.ParsePubKey()
	if err != nil {
		return nil, "", err
	}
	if pubKey == nil {
		return nil, "", peer.ErrEmptyPeerID
	}
	signer, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return nil, "", errors.Wrap(err, "invalid peer ID in signature")
	}

	// The signature covers exactly the encoded body, by the named sequencer.
	valid, err := sig.VerifyWithPublic(SOSequenceSignatureContext, pubKey, q.GetInner())
	if err != nil {
		return nil, "", errors.Wrap(err, "verify sequence signature")
	}
	if !valid {
		return nil, "", peer.ErrSignatureInvalid
	}
	if signer.String() != inner.GetPeerId() {
		return nil, "", errors.New("sequence signer does not match inner peer ID")
	}
	return inner, signer.String(), nil
}

// validateSequenceHead checks that head is unset, height 0 with no hash, or a
// real position. field names the head in errors.
func validateSequenceHead(field string, head *SOSequenceHead) error {
	if head.GetHeight() == 0 && len(head.GetHash()) != 0 {
		return errors.Errorf("%s at height 0 must not name a position", field)
	}
	if head.GetHeight() != 0 && len(head.GetHash()) != sha256.Size {
		return errors.Errorf("%s hash must be a 32-byte hash", field)
	}
	return nil
}

// Validate checks the sequencer's structure. An unset sequencer is Merge.
func (s *SOSequencer) Validate() error {
	if peerID := s.GetPeerId(); peerID != "" {
		if _, err := parsePeerIDField(peerID); err != nil {
			return errors.Wrap(err, "sequencer peer_id")
		}
	}
	return validateSequenceHead("sequencer start", s.GetStart())
}

// soPosition is one verified position of the resolved sequence.
type soPosition struct {
	// head is the position's height and identity.
	head *SOSequenceHead
	// op is the operation the position places.
	op *SOOperationPosition
}

// soSequence is the sequence above a checkpoint as one member resolves it.
type soSequence struct {
	// base is the last position the checkpoint covers, height 0 when none.
	base *SOSequenceHead
	// positions continue base without a gap or a fork.
	positions []soPosition
	// sequencer is the peer that signs later positions, empty under Merge.
	sequencer string
	// open is set while positions may still be added: a sequencer is
	// appointed, or the positions have not reached its start.
	open bool
}

// resolveSequence returns the sequence above base that records support under
// sequencer. Positions up to the sequencer's start are taken along the hash
// links down from start, whoever signed them; later ones only when the
// sequencer signed them. It stops at the first gap, at a position that does
// not name the one before it, and at a fork the sequencer signed. Records that
// fail to verify are ignored.
func resolveSequence(sharedObjectID string, base *SOSequenceHead, sequencer *SOSequencer, records []*SOSequence) soSequence {
	// Start above the base, open while a sequencer may still extend it.
	if base == nil {
		base = &SOSequenceHead{}
	}
	start := sequencer.GetStart()
	q := soSequence{
		base:      base,
		sequencer: sequencer.GetPeerId(),
		open:      sequencer.GetPeerId() != "" || start.GetHeight() > base.GetHeight(),
	}

	// Verify the records and index them by identity and by height.
	type verified struct {
		inner  *SOSequenceInner
		signer string
		hash   []byte
	}
	byHash := make(map[string]*verified, len(records))
	byHeight := make(map[uint64][]*verified, len(records))
	for _, record := range records {
		inner, signer, err := record.Verify(sharedObjectID)
		if err != nil || inner.GetHeight() <= base.GetHeight() {
			continue
		}
		v := &verified{inner: inner, signer: signer, hash: record.Hash()}
		byHash[string(v.hash)] = v
		byHeight[inner.GetHeight()] = append(byHeight[inner.GetHeight()], v)
	}

	// Follow the hash links down from start to the base.
	path := make(map[uint64]*verified)
	for h := start.GetHash(); len(h) != 0; {
		v := byHash[string(h)]
		if v == nil || v.inner.GetHeight() <= base.GetHeight() {
			break
		}
		path[v.inner.GetHeight()] = v
		h = v.inner.GetPrevHash()
	}

	// Walk up from the base, through the start path, then through the one
	// position the sequencer signed after each.
	prev := base
	for height := base.GetHeight() + 1; ; height++ {
		next := path[height]
		if height > start.GetHeight() {
			next = nil
			for _, v := range byHeight[height] {
				if v.signer != q.sequencer || q.sequencer == "" || !bytes.Equal(v.inner.GetPrevHash(), prev.GetHash()) {
					continue
				}
				if next != nil {
					return q
				}
				next = v
			}
		}
		if next == nil || !bytes.Equal(next.inner.GetPrevHash(), prev.GetHash()) {
			return q
		}
		prev = &SOSequenceHead{Height: height, Hash: next.hash}
		q.positions = append(q.positions, soPosition{head: prev, op: next.inner.GetOp()})
		if q.sequencer == "" && height >= start.GetHeight() {
			q.open = false
		}
	}
}
