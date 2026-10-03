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
	// KeyEpoch is the key epoch whose key encrypts the operation data.
	KeyEpoch uint64
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
// opDataEnc should be opData encoded with the key of link.KeyEpoch.
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
		KeyEpoch:        link.KeyEpoch,
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

// SOOperationSet is a set of verified operations of one shared object above
// its checkpoint. Adding operations in any order yields the same set, heads
// and evidence.
type SOOperationSet struct {
	sharedObjectID string
	ops            map[string]*SOOperationInner
	byAuthorSeq    map[soAuthorSeq][]string
	// frontier holds the checkpoint's heads.
	frontier map[string]struct{}
	// authors holds the last covered operation of each author.
	authors map[string]*SOCheckpointAuthor
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

// NewSOOperationSet returns an empty operation set for one shared object above
// checkpoint, which may be nil before the first checkpoint.
func NewSOOperationSet(sharedObjectID string, checkpoint *SOCheckpointInner) *SOOperationSet {
	s := &SOOperationSet{
		sharedObjectID: sharedObjectID,
		ops:            make(map[string]*SOOperationInner),
		byAuthorSeq:    make(map[soAuthorSeq][]string),
		frontier:       make(map[string]struct{}, len(checkpoint.GetFrontier())),
		authors:        make(map[string]*SOCheckpointAuthor, len(checkpoint.GetAuthors())),
	}
	for _, h := range checkpoint.GetFrontier() {
		s.frontier[string(h)] = struct{}{}
	}
	for _, author := range checkpoint.GetAuthors() {
		s.authors[author.GetPeerId()] = author
	}
	return s
}

// Covers reports whether the checkpoint below the set covers the operation
// at nonce in peerID's chain.
func (s *SOOperationSet) Covers(peerID string, nonce uint64) bool {
	author, ok := s.authors[peerID]
	return ok && nonce <= author.GetNonce()
}

// below reports whether h names an operation the checkpoint covers.
func (s *SOOperationSet) below(h string) bool {
	if _, ok := s.frontier[h]; ok {
		return true
	}
	for _, author := range s.authors {
		if string(author.GetOpHash()) == h {
			return true
		}
	}
	return false
}

// Add verifies op and adds it. It reports false when the set already holds it
// or the checkpoint covers it. An author's second operation at one sequence is
// kept as evidence.
func (s *SOOperationSet) Add(op *SOOperation) (bool, error) {
	// Verify the operation and skip one the set or the checkpoint holds.
	inner, err := op.Verify(s.sharedObjectID)
	if err != nil {
		return false, err
	}
	if s.Covers(inner.GetPeerId(), inner.GetNonce()) {
		return false, nil
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

// Heads returns the operations and checkpoint heads no operation in the set
// names, in byte order.
func (s *SOOperationSet) Heads() [][]byte {
	// Collect every hash an operation names.
	named := make(map[string]struct{}, len(s.ops))
	for _, inner := range s.ops {
		named[string(inner.GetPrevOpHash())] = struct{}{}
		for _, parent := range inner.GetParentHashes() {
			named[string(parent)] = struct{}{}
		}
	}

	// The heads are the operations and checkpoint heads left unnamed.
	heads := make([][]byte, 0, len(s.ops)+len(s.frontier))
	for key := range s.ops {
		if _, ok := named[key]; !ok {
			heads = append(heads, []byte(key))
		}
	}
	for key := range s.frontier {
		if _, ok := named[key]; !ok {
			heads = append(heads, []byte(key))
		}
	}
	slices.SortFunc(heads, bytes.Compare)
	return heads
}

// AuthorHead returns the nonce and hash of peerID's latest operation in the
// set or its checkpoint, or 0 and nil when it has written none.
func (s *SOOperationSet) AuthorHead(peerID string) (uint64, []byte) {
	// Start from the checkpoint's last covered operation.
	var nonce uint64
	var head []byte
	if author, ok := s.authors[peerID]; ok {
		nonce, head = author.GetNonce(), author.GetOpHash()
	}

	// Raise it to the author's highest operation in the set.
	for key, inner := range s.ops {
		if inner.GetPeerId() == peerID && inner.GetNonce() > nonce {
			nonce, head = inner.GetNonce(), []byte(key)
		}
	}
	return nonce, head
}

// Get returns the verified body of the operation with hash h, or nil.
func (s *SOOperationSet) Get(h []byte) *SOOperationInner {
	return s.ops[string(h)]
}

// Find returns the hash of peerID's operation with localID, or nil.
func (s *SOOperationSet) Find(peerID, localID string) []byte {
	for key, inner := range s.ops {
		if inner.GetPeerId() == peerID && inner.GetLocalId() == localID {
			return []byte(key)
		}
	}
	return nil
}

// Ancestors returns the operations in the set that the operation with hash h
// descends from, through its author chain and its causal parents.
func (s *SOOperationSet) Ancestors(h []byte) map[string]struct{} {
	ancestors := make(map[string]struct{})
	stack := []string{string(h)}
	for len(stack) != 0 {
		// Visit the links of the next operation the set holds.
		inner := s.ops[stack[len(stack)-1]]
		stack = stack[:len(stack)-1]
		if inner == nil {
			continue
		}
		links := inner.GetParentHashes()
		if prev := inner.GetPrevOpHash(); len(prev) != 0 {
			links = append(slices.Clip(links), prev)
		}

		// Record each link the first time it is reached.
		for _, link := range links {
			if _, ok := ancestors[string(link)]; ok {
				continue
			}
			if _, ok := s.ops[string(link)]; ok {
				ancestors[string(link)] = struct{}{}
				stack = append(stack, string(link))
			}
		}
	}
	return ancestors
}

// Order returns the replay order of the operations whose ancestry the set or
// its checkpoint holds: a topological order of the operation DAG with
// concurrent operations in byte order of their hashes. Every member holding the
// same operations computes the same order. An operation naming one the set
// lacks waits, with its descendants, until the missing operation arrives.
func (s *SOOperationSet) Order() [][]byte {
	// Count the links of every operation and index its children.
	pending := make(map[string]int, len(s.ops))
	children := make(map[string][]string, len(s.ops))
	var ready []string
	for key, inner := range s.ops {
		links := inner.GetParentHashes()
		if prev := inner.GetPrevOpHash(); len(prev) != 0 {
			links = append(slices.Clip(links), prev)
		}
		n := 0
		for _, link := range links {
			if _, ok := s.ops[string(link)]; !ok && s.below(string(link)) {
				continue
			}
			children[string(link)] = append(children[string(link)], key)
			n++
		}
		pending[key] = n
		if n == 0 {
			ready = append(ready, key)
		}
	}
	slices.Sort(ready)

	// Place the lowest ready operation and release its children. A missing
	// link is never placed, so its descendants stay pending.
	order := make([][]byte, 0, len(s.ops))
	for len(ready) != 0 {
		key := ready[0]
		ready = ready[1:]
		order = append(order, []byte(key))
		for _, child := range children[key] {
			if pending[child]--; pending[child] == 0 {
				i, _ := slices.BinarySearch(ready, child)
				ready = slices.Insert(ready, i, child)
			}
		}
	}
	return order
}

// Equivocated reports whether the author of the operation with hash h signed
// another operation at the same sequence.
func (s *SOOperationSet) Equivocated(h []byte) bool {
	inner, ok := s.ops[string(h)]
	if !ok {
		return false
	}
	return len(s.byAuthorSeq[soAuthorSeq{author: inner.GetPeerId(), nonce: inner.GetNonce()}]) > 1
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
