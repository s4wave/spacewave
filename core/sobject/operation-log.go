package sobject

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// SOOperationProtocolVersion is the operation format this build writes and accepts.
const SOOperationProtocolVersion = 1

// MaxSOOperationParents bounds the causal parents named by one operation.
const MaxSOOperationParents = 256

// soOperationHashDomain separates operation hashes from other SHA-256 digests.
const soOperationHashDomain = "spacewave/sharedobject/operation/v1"

// SOOperationSignatureContext is the signature context of every operation.
// The signed body binds the object, so the context needs no per-operation data.
var SOOperationSignatureContext = baseCryptoContext + "participant_operation_signature"

// SOOperationLink places a new operation in its author's chain and in the
// operation DAG.
type SOOperationLink struct {
	// Nonce is the author sequence of the new operation.
	Nonce uint64
	// PrevOpHash is the hash of the author's previous operation, empty when Nonce is 1.
	PrevOpHash []byte
	// ParentHashes are the other heads the author knows.
	ParentHashes [][]byte
	// ConfigHash is the config chain hash the operation is written under.
	ConfigHash []byte
}

// HashSOOperationInner returns the identity of an operation from its signed body.
func HashSOOperationInner(inner []byte) []byte {
	// Prefix the length-framed body with the domain.
	preimage := make([]byte, 0, len(soOperationHashDomain)+8+len(inner))
	preimage = append(preimage, soOperationHashDomain...)
	preimage = binary.BigEndian.AppendUint64(preimage, uint64(len(inner)))
	preimage = append(preimage, inner...)
	digest := sha256.Sum256(preimage)
	return digest[:]
}

// Hash returns the identity of the operation.
func (op *SOOperation) Hash() []byte {
	return HashSOOperationInner(op.GetInner())
}

// BuildSOOperation signs a new operation by the author of privKey.
// opDataEnc should be opData encoded with the root state transform.
func BuildSOOperation(
	sharedObjectID string,
	privKey crypto.PrivKey,
	opDataEnc []byte,
	link *SOOperationLink,
	opLocalID string,
) (*SOOperation, error) {
	// Identify the author from the signing key.
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get peer ID from private key")
	}

	// Bind the body to the object, the author chain, the DAG and the config.
	inner := &SOOperationInner{
		PeerId:          peerID.String(),
		LocalId:         opLocalID,
		Nonce:           link.Nonce,
		OpData:          opDataEnc,
		SharedObjectId:  sharedObjectID,
		ProtocolVersion: SOOperationProtocolVersion,
		PrevOpHash:      link.PrevOpHash,
		ParentHashes:    sortedOperationHashes(link.ParentHashes, link.PrevOpHash),
		ConfigHash:      link.ConfigHash,
	}
	if err := inner.Validate(); err != nil {
		return nil, err
	}
	innerData, err := inner.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal operation inner data")
	}

	// Sign exactly the encoded body.
	sig, err := peer.NewSignature(SOOperationSignatureContext, privKey, hash.RecommendedHashType, innerData, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to sign operation")
	}
	op := &SOOperation{Inner: innerData, Signature: sig}
	if err := op.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid operation")
	}
	return op, nil
}

// sortedOperationHashes returns the distinct hashes in byte order without exclude.
func sortedOperationHashes(hashes [][]byte, exclude []byte) [][]byte {
	out := make([][]byte, 0, len(hashes))
	for _, h := range hashes {
		if !bytes.Equal(h, exclude) {
			out = append(out, h)
		}
	}
	slices.SortFunc(out, bytes.Compare)
	return slices.CompactFunc(out, bytes.Equal)
}

// validateLinks checks the operation's chain, parent and config references.
func (i *SOOperationInner) validateLinks() error {
	// The body names this protocol and an object.
	if i.GetProtocolVersion() != SOOperationProtocolVersion {
		return errors.Errorf("unsupported operation protocol version %d", i.GetProtocolVersion())
	}
	if i.GetSharedObjectId() == "" {
		return errors.New("operation shared_object_id is empty")
	}

	// The author's first operation has no predecessor; every later one names it.
	prev := i.GetPrevOpHash()
	if i.GetNonce() == 1 && len(prev) != 0 {
		return errors.New("first operation of an author must not name a previous operation")
	}
	if i.GetNonce() > 1 && len(prev) != sha256.Size {
		return errors.New("operation prev_op_hash must be a 32-byte hash")
	}

	// Parents are distinct hashes in byte order and never repeat prev.
	parents := i.GetParentHashes()
	if len(parents) > MaxSOOperationParents {
		return errors.Wrap(ErrMaxCountExceeded, "operation parent_hashes")
	}
	for j, parent := range parents {
		if len(parent) != sha256.Size {
			return errors.Errorf("parent_hashes[%d] must be a 32-byte hash", j)
		}
		if j > 0 && bytes.Compare(parents[j-1], parent) >= 0 {
			return errors.New("operation parent_hashes must be strictly sorted")
		}
		if bytes.Equal(parent, prev) {
			return errors.New("operation parent_hashes must not repeat prev_op_hash")
		}
	}

	// The operation names the config it was written under.
	if len(i.GetConfigHash()) != sha256.Size {
		return errors.New("operation config_hash must be a 32-byte hash")
	}
	return nil
}

// Verify authenticates the operation as written for sharedObjectID by the
// author it names, and returns its body. It does not check authorization.
func (op *SOOperation) Verify(sharedObjectID string) (*SOOperationInner, error) {
	// Decode the well-formed body.
	if err := op.Validate(); err != nil {
		return nil, err
	}
	inner, err := op.UnmarshalInner()
	if err != nil {
		return nil, err
	}

	// An operation signed for another object is not valid here.
	if inner.GetSharedObjectId() != sharedObjectID {
		return nil, errors.New("operation is bound to another shared object")
	}

	// The signer is the author.
	sig := op.GetSignature()
	pubKey, err := sig.ParsePubKey()
	if err != nil {
		return nil, err
	}
	if pubKey == nil {
		return nil, errors.New("operation signature has no public key")
	}
	signerID, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return nil, errors.Wrap(err, "invalid peer ID in signature")
	}
	if signerID.String() != inner.GetPeerId() {
		return nil, errors.New("signer peer ID does not match inner peer ID")
	}

	// The signature covers exactly the encoded body.
	valid, err := sig.VerifyWithPublic(SOOperationSignatureContext, pubKey, op.GetInner())
	if err != nil {
		return nil, errors.Wrap(err, "failed to verify signature")
	}
	if !valid {
		return nil, peer.ErrSignatureInvalid
	}
	return inner, nil
}

// ValidateSignature authenticates the operation and checks that its author
// may write operations under participants.
func (op *SOOperation) ValidateSignature(sharedObjectID string, participants []*SOParticipantConfig) error {
	inner, err := op.Verify(sharedObjectID)
	if err != nil {
		return err
	}
	for _, p := range participants {
		if p.GetPeerId() == inner.GetPeerId() && CanWriteOps(p.GetRole()) {
			return nil
		}
	}
	return ErrNotParticipant
}

// SOOperationSet is a set of verified operations of one shared object.
// Adding operations in any order yields the same set, heads and evidence.
type SOOperationSet struct {
	sharedObjectID string
	ops            map[string]*SOOperationInner
	byAuthorSeq    map[soAuthorSeq][]string
}

// soAuthorSeq identifies one position in one author's chain.
type soAuthorSeq struct {
	author string
	nonce  uint64
}

// SOEquivocation is evidence that an author signed several operations at one sequence.
type SOEquivocation struct {
	// PeerID is the author.
	PeerID string
	// Nonce is the author sequence.
	Nonce uint64
	// Hashes are the conflicting operations in byte order.
	Hashes [][]byte
}

// NewSOOperationSet returns an empty operation set for one shared object.
func NewSOOperationSet(sharedObjectID string) *SOOperationSet {
	return &SOOperationSet{
		sharedObjectID: sharedObjectID,
		ops:            make(map[string]*SOOperationInner),
		byAuthorSeq:    make(map[soAuthorSeq][]string),
	}
}

// Add verifies op and adds it. It reports false when the set already holds it.
// An author's second operation at one sequence is kept as evidence.
func (s *SOOperationSet) Add(op *SOOperation) (bool, error) {
	// Verify the operation and skip one the set holds.
	inner, err := op.Verify(s.sharedObjectID)
	if err != nil {
		return false, err
	}
	key := string(op.Hash())
	if _, ok := s.ops[key]; ok {
		return false, nil
	}

	// Index it by hash and by its author position.
	s.ops[key] = inner
	pos := soAuthorSeq{author: inner.GetPeerId(), nonce: inner.GetNonce()}
	s.byAuthorSeq[pos] = append(s.byAuthorSeq[pos], key)
	return true, nil
}

// Len returns the number of operations in the set.
func (s *SOOperationSet) Len() int {
	return len(s.ops)
}

// Heads returns the operations no other operation in the set names, in byte order.
func (s *SOOperationSet) Heads() [][]byte {
	// Collect every hash an operation names.
	named := make(map[string]struct{}, len(s.ops))
	for _, inner := range s.ops {
		named[string(inner.GetPrevOpHash())] = struct{}{}
		for _, parent := range inner.GetParentHashes() {
			named[string(parent)] = struct{}{}
		}
	}

	// The heads are the operations left unnamed.
	heads := make([][]byte, 0, len(s.ops))
	for key := range s.ops {
		if _, ok := named[key]; !ok {
			heads = append(heads, []byte(key))
		}
	}
	slices.SortFunc(heads, bytes.Compare)
	return heads
}

// Equivocations returns every author sequence with more than one operation,
// ordered by author and sequence.
func (s *SOOperationSet) Equivocations() []SOEquivocation {
	var out []SOEquivocation
	for pos, keys := range s.byAuthorSeq {
		if len(keys) < 2 {
			continue
		}
		hashes := make([][]byte, len(keys))
		for i, key := range keys {
			hashes[i] = []byte(key)
		}
		slices.SortFunc(hashes, bytes.Compare)
		out = append(out, SOEquivocation{PeerID: pos.author, Nonce: pos.nonce, Hashes: hashes})
	}
	slices.SortFunc(out, func(a, b SOEquivocation) int {
		if c := strings.Compare(a.PeerID, b.PeerID); c != 0 {
			return c
		}
		return cmp.Compare(a.Nonce, b.Nonce)
	})
	return out
}
