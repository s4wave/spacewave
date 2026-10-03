package sobject

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"maps"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// SOReplayVersion is the replay rule version this build computes Worlds with.
const SOReplayVersion = 1

// soCheckpointHashDomain separates checkpoint hashes from other SHA-256 digests.
const soCheckpointHashDomain = "spacewave/sharedobject/checkpoint/v1"

// SOCheckpointSignatureContext is the signature context of every checkpoint.
// The signed body binds the object, so the context needs no per-checkpoint data.
var SOCheckpointSignatureContext = baseCryptoContext + "checkpoint_signature"

// HashSOCheckpointInner returns the identity of a checkpoint from its signed body.
func HashSOCheckpointInner(inner []byte) []byte {
	// Prefix the length-framed body with the domain.
	preimage := make([]byte, 0, len(soCheckpointHashDomain)+8+len(inner))
	preimage = append(preimage, soCheckpointHashDomain...)
	preimage = binary.BigEndian.AppendUint64(preimage, uint64(len(inner)))
	preimage = append(preimage, inner...)
	digest := sha256.Sum256(preimage)
	return digest[:]
}

// Hash returns the identity of the checkpoint.
func (c *SOCheckpoint) Hash() []byte {
	return HashSOCheckpointInner(c.GetInner())
}

// BuildSOCheckpoint signs inner as a checkpoint by the owner of privKey.
// inner.StateData must already be encrypted with the key of inner.KeyEpoch.
func BuildSOCheckpoint(privKey crypto.PrivKey, inner *SOCheckpointInner) (*SOCheckpoint, error) {
	// Encode a well-formed body.
	if err := inner.Validate(); err != nil {
		return nil, err
	}
	innerData, err := inner.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal checkpoint inner")
	}

	// Sign exactly the encoded body.
	checkpoint := &SOCheckpoint{Inner: innerData}
	if err := checkpoint.CoSign(privKey); err != nil {
		return nil, err
	}
	return checkpoint, nil
}

// CoSign adds a signature by the owner of privKey. The checkpoint's identity
// covers only its body, so the hash is unchanged.
func (c *SOCheckpoint) CoSign(privKey crypto.PrivKey) error {
	sig, err := peer.NewSignature(SOCheckpointSignatureContext, privKey, hash.RecommendedHashType, c.GetInner(), true)
	if err != nil {
		return errors.Wrap(err, "sign checkpoint")
	}
	c.Signatures = append(c.Signatures, sig)
	return nil
}

// BuildGenesisSOCheckpoint signs the height 0 checkpoint of a new shared
// object. stateDataEnc is the initial state encrypted with the key of epoch 0.
func BuildGenesisSOCheckpoint(
	privKey crypto.PrivKey,
	sharedObjectID string,
	configHash []byte,
	stateDataEnc []byte,
) (*SOCheckpoint, error) {
	return BuildSOCheckpoint(privKey, &SOCheckpointInner{
		SharedObjectId: sharedObjectID,
		ConfigHash:     configHash,
		StateData:      stateDataEnc,
		ReplayVersion:  SOReplayVersion,
	})
}

// BuildNextCheckpoint signs, as the owner of privKey, the checkpoint after the
// held one that covers every held operation under the current config.
// stateDataEnc is the state after those operations, encrypted with the key of
// the current key epoch.
func (s *SOState) BuildNextCheckpoint(sharedObjectID string, privKey crypto.PrivKey, stateDataEnc []byte) (*SOCheckpoint, error) {
	// Continue the chain from the held checkpoint.
	prev := s.GetCheckpoint()
	prevInner, err := s.GetCheckpointInner()
	if err != nil {
		return nil, err
	}
	if prevInner == nil {
		return nil, errors.New("state has no checkpoint to follow")
	}
	set, err := s.OperationSet(sharedObjectID)
	if err != nil {
		return nil, err
	}

	// Cover every held operation.
	return BuildSOCheckpoint(privKey, &SOCheckpointInner{
		SharedObjectId:     sharedObjectID,
		Height:             prevInner.GetHeight() + 1,
		PrevCheckpointHash: prev.Hash(),
		ConfigHash:         s.GetConfig().GetConfigChainHash(),
		Frontier:           set.Heads(),
		StateData:          stateDataEnc,
		ReplayVersion:      SOReplayVersion,
		KeyEpoch:           s.CurrentKeyEpoch().GetEpoch(),
		Authors:            set.coveredAuthors(),
	})
}

// UnmarshalInner decodes and checks the checkpoint body.
func (c *SOCheckpoint) UnmarshalInner() (*SOCheckpointInner, error) {
	inner := &SOCheckpointInner{}
	if err := inner.UnmarshalVT(c.GetInner()); err != nil {
		return nil, errors.Wrap(err, "unmarshal checkpoint inner")
	}
	if err := inner.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid checkpoint inner")
	}
	return inner, nil
}

// Validate checks the checkpoint body's structure.
func (i *SOCheckpointInner) Validate() error {
	// The body names an object, a config and a known replay rule.
	if i.GetSharedObjectId() == "" {
		return ErrEmptySharedObjectID
	}
	if len(i.GetConfigHash()) != sha256.Size {
		return errors.New("checkpoint config_hash must be a 32-byte hash")
	}
	if i.GetReplayVersion() != SOReplayVersion {
		return errors.Errorf("unsupported checkpoint replay version %d", i.GetReplayVersion())
	}
	if len(i.GetStateData()) > MaxStateDataSize {
		return ErrMaxSizeExceeded
	}

	// Genesis starts the chain; every later checkpoint names its predecessor.
	if i.GetHeight() == 0 {
		if len(i.GetPrevCheckpointHash()) != 0 || len(i.GetFrontier()) != 0 || len(i.GetAuthors()) != 0 {
			return errors.New("genesis checkpoint must not name previous operations")
		}
		return nil
	}
	if len(i.GetPrevCheckpointHash()) != sha256.Size {
		return errors.New("checkpoint prev_checkpoint_hash must be a 32-byte hash")
	}

	// The frontier holds distinct hashes in byte order.
	for j, h := range i.GetFrontier() {
		if len(h) != sha256.Size {
			return errors.Errorf("frontier[%d] must be a 32-byte hash", j)
		}
		if j > 0 && bytes.Compare(i.GetFrontier()[j-1], h) >= 0 {
			return errors.New("checkpoint frontier must be strictly sorted")
		}
	}

	return validateAuthorHeads("authors", i.GetAuthors())
}

// validateAuthorHeads checks that each author appears once, in peer ID order,
// at a real operation. field names the list in errors.
func validateAuthorHeads(field string, authors []*SOCheckpointAuthor) error {
	for j, author := range authors {
		if _, err := parsePeerIDField(author.GetPeerId()); err != nil {
			return errors.Wrapf(err, "%s[%d]", field, j)
		}
		if author.GetNonce() == 0 || len(author.GetOpHash()) != sha256.Size {
			return errors.Errorf("%s[%d] must name an operation", field, j)
		}
		if j > 0 && strings.Compare(authors[j-1].GetPeerId(), author.GetPeerId()) >= 0 {
			return errors.Errorf("%s must be strictly sorted by peer_id", field)
		}
	}
	return nil
}

// Verify authenticates every signature on the checkpoint as written for
// sharedObjectID and returns the body and the signers. It does not check that
// a signer holds authority; ValidateAuthority does.
func (c *SOCheckpoint) Verify(sharedObjectID string) (*SOCheckpointInner, []string, error) {
	// Decode the well-formed body bound to this object.
	if len(c.GetInner()) == 0 {
		return nil, nil, ErrEmptyInnerData
	}
	if len(c.GetSignatures()) == 0 {
		return nil, nil, errors.New("checkpoint has no signature")
	}
	if len(c.GetSignatures()) > MaxParticipants {
		return nil, nil, ErrMaxCountExceeded
	}
	inner, err := c.UnmarshalInner()
	if err != nil {
		return nil, nil, err
	}
	if inner.GetSharedObjectId() != sharedObjectID {
		return nil, nil, errors.New("checkpoint is bound to another shared object")
	}

	// Every signature covers exactly the encoded body, once per signer.
	signers := make([]string, 0, len(c.GetSignatures()))
	for i, sig := range c.GetSignatures() {
		pubKey, err := sig.ParsePubKey()
		if err != nil {
			return nil, nil, errors.Wrapf(err, "signatures[%d]", i)
		}
		if pubKey == nil {
			return nil, nil, peer.ErrEmptyPeerID
		}
		signer, err := peer.IDFromPublicKey(pubKey)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "signatures[%d]", i)
		}
		if slices.Contains(signers, signer.String()) {
			return nil, nil, errors.Errorf("signatures[%d]: duplicate signer", i)
		}
		valid, err := sig.VerifyWithPublic(SOCheckpointSignatureContext, pubKey, c.GetInner())
		if err != nil {
			return nil, nil, errors.Wrapf(err, "signatures[%d]", i)
		}
		if !valid {
			return nil, nil, peer.ErrSignatureInvalid
		}
		signers = append(signers, signer.String())
	}
	return inner, signers, nil
}

// ValidateAuthority authenticates the checkpoint and checks that an owner
// under participants signed it.
func (c *SOCheckpoint) ValidateAuthority(sharedObjectID string, participants []*SOParticipantConfig) (*SOCheckpointInner, error) {
	inner, signers, err := c.Verify(sharedObjectID)
	if err != nil {
		return nil, err
	}
	for _, p := range participants {
		if IsOwner(p.GetRole()) && slices.Contains(signers, p.GetPeerId()) {
			return inner, nil
		}
	}
	return nil, errors.New("checkpoint is not signed by an owner")
}

// coveredAuthors returns the last operation of each author the set and its
// checkpoint hold, sorted by peer ID.
func (s *SOOperationSet) coveredAuthors() []*SOCheckpointAuthor {
	// Start from the authors the checkpoint covers.
	heads := make(map[string]*SOCheckpointAuthor, len(s.authors))
	maps.Copy(heads, s.authors)

	// Raise each author to its highest operation in the set.
	for key, inner := range s.ops {
		head, ok := heads[inner.GetPeerId()]
		if !ok || inner.GetNonce() > head.GetNonce() {
			heads[inner.GetPeerId()] = &SOCheckpointAuthor{
				PeerId: inner.GetPeerId(),
				Nonce:  inner.GetNonce(),
				OpHash: []byte(key),
			}
		}
	}

	// Sort by peer ID.
	out := make([]*SOCheckpointAuthor, 0, len(heads))
	for _, author := range heads {
		out = append(out, author)
	}
	slices.SortFunc(out, func(a, b *SOCheckpointAuthor) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})
	return out
}
