package sobject

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// MaxVotingWeight bounds one device's voting weight, so weight sums over
// MaxParticipants devices cannot overflow.
const MaxVotingWeight = 1 << 16

// maxControlValueSize bounds the value a control message carries: a
// checkpoint body with its World, or a control record.
const maxControlValueSize = MaxStateDataSize + 1024*1024

// soControlMessageHashDomain separates control message hashes from other
// SHA-256 digests.
const soControlMessageHashDomain = "spacewave/sharedobject/control/v1"

// SOControlMessageSignatureContext is the signature context of every control
// message. The signed body binds the object and the instance, so the context
// needs no per-message data.
var SOControlMessageSignatureContext = baseCryptoContext + "control_message_signature"

// IsGroupControl reports whether the config's voters, not an owner, decide its
// control records and checkpoints.
func (c *SharedObjectConfig) IsGroupControl() bool {
	return c.GetControl() == SOControl_SO_CONTROL_GROUP
}

// VotingWeight returns peerID's voting weight under group control, or zero.
func (c *SharedObjectConfig) VotingWeight(peerID string) uint64 {
	if !c.IsGroupControl() {
		return 0
	}
	for _, p := range c.GetParticipants() {
		if p.GetPeerId() == peerID {
			return uint64(p.GetVotingWeight())
		}
	}
	return 0
}

// TotalVotingWeight returns the sum of every voter's weight under group
// control, or zero.
func (c *SharedObjectConfig) TotalVotingWeight() uint64 {
	if !c.IsGroupControl() {
		return 0
	}
	var total uint64
	for _, p := range c.GetParticipants() {
		total += uint64(p.GetVotingWeight())
	}
	return total
}

// Voters returns the peer IDs that vote under group control, sorted.
func (c *SharedObjectConfig) Voters() []string {
	// Only group control has voters.
	if !c.IsGroupControl() {
		return nil
	}

	// Collect and sort the peers with weight.
	var voters []string
	for _, p := range c.GetParticipants() {
		if p.GetVotingWeight() != 0 {
			voters = append(voters, p.GetPeerId())
		}
	}
	slices.Sort(voters)
	return voters
}

// HasQuorum reports whether weight is more than two thirds of total. Two such
// quorums share more than one third of the weight, so they share an honest
// voter while dishonest voters hold less than one third.
func HasQuorum(weight, total uint64) bool {
	return total != 0 && weight*3 > total*2
}

// exceedsFaultyWeight reports whether weight is more than one third of total,
// so it includes an honest voter.
func exceedsFaultyWeight(weight, total uint64) bool {
	return weight*3 > total
}

// DefaultVotingWeights returns participants with the voting weights group
// control starts with: one for the first device of each entity that can
// write, in participant order, and one for each writing device without an
// entity. Every other device does not vote. Votes are per device because each
// device keeps its own locks.
func DefaultVotingWeights(participants []*SOParticipantConfig) []*SOParticipantConfig {
	out := make([]*SOParticipantConfig, len(participants))
	seen := make(map[string]struct{}, len(participants))
	for i, p := range participants {
		out[i] = p.CloneVT()
		out[i].VotingWeight = 0
		if !CanWriteOps(p.GetRole()) {
			continue
		}
		if entityID := p.GetEntityId(); entityID != "" {
			if _, ok := seen[entityID]; ok {
				continue
			}
			seen[entityID] = struct{}{}
		}
		out[i].VotingWeight = 1
	}
	return out
}

// validateControl checks the control setting and voting weights: under owner
// control nobody votes; under group control only writers and owners vote, at
// least one does, and the config names the checkpoint it sealed.
func (c *SharedObjectConfig) validateControl() error {
	// Owner control has no voters.
	switch c.GetControl() {
	case SOControl_SO_CONTROL_OWNER:
		for i, p := range c.GetParticipants() {
			if p.GetVotingWeight() != 0 {
				return errors.Errorf("participants[%d]: voting weight under owner control", i)
			}
		}
		if sealed := c.GetSealedCheckpoint(); sealed != nil {
			return sealed.Validate()
		}
		return nil
	case SOControl_SO_CONTROL_GROUP:
	default:
		return errors.Errorf("unknown control %v", c.GetControl())
	}

	// Group control needs a writing voter and the sealed checkpoint.
	var total uint64
	for i, p := range c.GetParticipants() {
		w := p.GetVotingWeight()
		if w > MaxVotingWeight {
			return errors.Errorf("participants[%d]: voting weight above %d", i, MaxVotingWeight)
		}
		if w != 0 && !CanWriteOps(p.GetRole()) {
			return errors.Errorf("participants[%d]: only writers and owners vote", i)
		}
		total += uint64(w)
	}
	if total == 0 {
		return errors.New("group control needs a voter")
	}
	if c.GetSealedCheckpoint() == nil {
		return errors.New("group control needs a sealed checkpoint")
	}
	return c.GetSealedCheckpoint().Validate()
}

// Validate checks that the head names a checkpoint.
func (h *SOCheckpointHead) Validate() error {
	if len(h.GetHash()) != sha256.Size {
		return errors.New("checkpoint head hash must be a 32-byte hash")
	}
	return nil
}

// HashSOControlMessageInner returns the identity of a control message from its
// signed body.
func HashSOControlMessageInner(inner []byte) []byte {
	// Prefix the length-framed body with the domain.
	preimage := make([]byte, 0, len(soControlMessageHashDomain)+8+len(inner))
	preimage = append(preimage, soControlMessageHashDomain...)
	preimage = binary.BigEndian.AppendUint64(preimage, uint64(len(inner)))
	preimage = append(preimage, inner...)
	digest := sha256.Sum256(preimage)
	return digest[:]
}

// Hash returns the identity of the message.
func (m *SOControlMessage) Hash() []byte {
	return HashSOControlMessageInner(m.GetInner())
}

// ControlValueHash returns the identity of a value of kind: the hash of a
// control record, which must be encoded with signatures and commit cleared, or
// of a checkpoint body.
func ControlValueHash(kind SODecisionKind, value []byte) ([]byte, error) {
	switch kind {
	case SODecisionKind_SO_DECISION_KIND_CONFIG:
		// The record's hash is over its canonical encoding, so the value must
		// be that encoding.
		entry := &SOConfigChange{}
		if err := entry.UnmarshalVT(value); err != nil {
			return nil, errors.Wrap(err, "unmarshal control record value")
		}
		body, err := configChangeSignedBody(entry)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(body, value) {
			return nil, errors.New("control record value is not its signed body")
		}
		h := sha256.Sum256(body)
		return h[:], nil
	case SODecisionKind_SO_DECISION_KIND_CHECKPOINT:
		return HashSOCheckpointInner(value), nil
	default:
		return nil, errors.Errorf("unknown decision kind %v", kind)
	}
}

// BuildSOControlMessage signs inner as the voter of privKey, which it names as
// the message's peer.
func BuildSOControlMessage(privKey crypto.PrivKey, inner *SOControlMessageInner) (*SOControlMessage, error) {
	// Name the signer and check the body.
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	inner = inner.CloneVT()
	inner.PeerId = peerID.String()
	if err := inner.Validate(); err != nil {
		return nil, err
	}
	data, err := inner.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal control message inner")
	}

	// Sign exactly the encoded body.
	sig, err := peer.NewSignature(SOControlMessageSignatureContext, privKey, hash.RecommendedHashType, data, true)
	if err != nil {
		return nil, errors.Wrap(err, "sign control message")
	}
	return &SOControlMessage{Inner: data, Signature: sig}, nil
}

// Validate checks the message body's structure: the instance it belongs to,
// and the fields its type carries.
func (i *SOControlMessageInner) Validate() error {
	// The body names an object, a voter and an instance.
	if i.GetSharedObjectId() == "" {
		return ErrEmptySharedObjectID
	}
	if _, err := parsePeerIDField(i.GetPeerId()); err != nil {
		return errors.Wrap(err, "control message peer_id")
	}
	if len(i.GetConfigHash()) != sha256.Size {
		return errors.New("control message config_hash must be a 32-byte hash")
	}
	if n := len(i.GetValueHash()); n != 0 && n != sha256.Size {
		return errors.New("control message value_hash must be empty or a 32-byte hash")
	}

	// Each type carries its own fields.
	switch i.GetType() {
	case SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_AGREE:
		if i.GetKind() != SODecisionKind_SO_DECISION_KIND_CONFIG {
			return errors.New("only control records are agreed to")
		}
		if i.GetHeight() != 0 || i.GetRound() != 0 || i.GetValidRound() != 0 {
			return errors.New("agreement must not name a height or round")
		}
		return i.validateValue()
	case SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PROPOSAL:
		if i.GetHeight() == 0 || i.GetRound() == 0 || i.GetValidRound() >= i.GetRound() {
			return errors.New("proposal must name a height and a round after its valid round")
		}
		return i.validateValue()
	case SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PREVOTE,
		SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT:
		if i.GetKind() != SODecisionKind_SO_DECISION_KIND_UNKNOWN {
			return errors.New("vote must not name a value kind")
		}
		if i.GetHeight() == 0 || i.GetRound() == 0 || i.GetValidRound() != 0 || len(i.GetValue()) != 0 {
			return errors.New("vote must name a height and round and carry no value")
		}
		return nil
	default:
		return errors.Errorf("unknown control message type %v", i.GetType())
	}
}

// validateValue checks that the message carries the value it names.
func (i *SOControlMessageInner) validateValue() error {
	// The value must be present and bounded.
	if len(i.GetValue()) == 0 || len(i.GetValue()) > maxControlValueSize {
		return errors.New("control message must carry a bounded value")
	}

	// The value must match its hash.
	h, err := ControlValueHash(i.GetKind(), i.GetValue())
	if err != nil {
		return err
	}
	if !bytes.Equal(h, i.GetValueHash()) {
		return errors.New("control message value does not match value_hash")
	}
	return nil
}

// Verify authenticates the message as signed for sharedObjectID by the voter
// it names, and returns its body. It does not check that the signer votes.
func (m *SOControlMessage) Verify(sharedObjectID string) (*SOControlMessageInner, error) {
	// Decode the well-formed body bound to this object.
	if len(m.GetInner()) == 0 {
		return nil, ErrEmptyInnerData
	}
	if err := m.GetSignature().Validate(); err != nil {
		return nil, err
	}
	inner := &SOControlMessageInner{}
	if err := inner.UnmarshalVT(m.GetInner()); err != nil {
		return nil, errors.Wrap(err, "unmarshal control message inner")
	}
	if err := inner.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid control message inner")
	}
	if inner.GetSharedObjectId() != sharedObjectID {
		return nil, errors.New("control message is bound to another shared object")
	}

	// The signer must be the named voter.
	sig := m.GetSignature()
	pubKey, err := sig.ParsePubKey()
	if err != nil {
		return nil, err
	}
	if pubKey == nil {
		return nil, peer.ErrEmptyPeerID
	}
	signer, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return nil, errors.Wrap(err, "invalid peer ID in signature")
	}
	if signer.String() != inner.GetPeerId() {
		return nil, errors.New("control message is not signed by its peer")
	}

	// The signature covers exactly the encoded body.
	valid, err := sig.VerifyWithPublic(SOControlMessageSignatureContext, pubKey, m.GetInner())
	if err != nil {
		return nil, errors.Wrap(err, "verify control message signature")
	}
	if !valid {
		return nil, peer.ErrSignatureInvalid
	}
	return inner, nil
}

// VerifyCommit checks that commit decides valueHash for the instance at
// height under cfg: precommits of one round for valueHash under cfg's
// head, each from a distinct voter of cfg, together holding more than two
// thirds of its voting weight.
func VerifyCommit(
	sharedObjectID string,
	cfg *SharedObjectConfig,
	height uint64,
	valueHash []byte,
	commit []*SOControlMessage,
) error {
	// Only group control decides by commit.
	if !cfg.IsGroupControl() {
		return errors.New("commit requires group control")
	}
	if len(commit) == 0 || len(commit) > MaxParticipants {
		return errors.New("commit must hold between one and the participant limit of precommits")
	}

	// Sum the weight of distinct voters precommitting valueHash in one round.
	var round uint32
	var weight uint64
	seen := make(map[string]struct{}, len(commit))
	for i, msg := range commit {
		inner, err := msg.Verify(sharedObjectID)
		if err != nil {
			return errors.Wrapf(err, "commit[%d]", i)
		}
		if inner.GetType() != SOControlMessageType_SO_CONTROL_MESSAGE_TYPE_PRECOMMIT ||
			inner.GetHeight() != height ||
			!bytes.Equal(inner.GetConfigHash(), cfg.GetConfigChainHash()) ||
			!bytes.Equal(inner.GetValueHash(), valueHash) {
			return errors.Errorf("commit[%d]: not a precommit of this decision", i)
		}
		if i == 0 {
			round = inner.GetRound()
		} else if inner.GetRound() != round {
			return errors.Errorf("commit[%d]: precommits span rounds", i)
		}
		if _, ok := seen[inner.GetPeerId()]; ok {
			return errors.Errorf("commit[%d]: duplicate voter", i)
		}
		seen[inner.GetPeerId()] = struct{}{}
		w := cfg.VotingWeight(inner.GetPeerId())
		if w == 0 {
			return errors.Errorf("commit[%d]: %s does not vote", i, inner.GetPeerId())
		}
		weight += w
	}
	if !HasQuorum(weight, cfg.TotalVotingWeight()) {
		return errors.New("commit lacks more than two thirds of the voting weight")
	}
	return nil
}
