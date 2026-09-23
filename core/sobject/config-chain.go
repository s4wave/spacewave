package sobject

import (
	"bytes"
	"crypto/sha256"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// HashSOConfigChange computes the SHA-256 hash of a serialized SOConfigChange.
// The signature field is excluded from the hash by zeroing it before marshaling.
func HashSOConfigChange(entry *SOConfigChange) ([]byte, error) {
	// Exclude the signature from the content-addressed chain entry.
	clone := entry.CloneVT()
	clone.Signature = nil
	data, err := clone.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal config change for hash")
	}

	// Hash the canonical generated encoding.
	h := sha256.Sum256(data)
	return h[:], nil
}

// VerifyConfigChain verifies a config change chain from genesis to current.
// Each entry must have:
// 1. Monotonically increasing config_seqno (starting from 0)
// 2. previous_hash matching the hash of the prior entry (genesis has zero previous_hash)
// 3. Valid authorization according to the change type
func VerifyConfigChain(entries []*SOConfigChange) error {
	// Require a chain before validating its bootstrap entry.
	if len(entries) == 0 {
		return errors.New("config chain is empty")
	}

	// Genesis entry must have seqno 0.
	genesis := entries[0]
	if genesis.GetConfigSeqno() != 0 {
		return errors.Errorf("genesis entry has seqno %d, expected 0", genesis.GetConfigSeqno())
	}
	if len(genesis.GetPreviousHash()) != 0 {
		return errors.New("genesis entry must have empty previous_hash")
	}
	if genesis.GetChangeType() != SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS {
		return errors.Errorf("genesis entry has change type %s, expected GENESIS", genesis.GetChangeType().String())
	}

	// Track the current effective config for authorization checks. After an
	// entry is accepted, the config-chain head becomes that entry's hash/seqno
	// even though entry.Config still carries the pre-head metadata snapshot.
	// Genesis must grant owner authority before any later change can be signed.
	currentConfig := genesis.GetConfig()
	if len(currentConfig.GetParticipants()) == 0 {
		return errors.Wrap(ErrEmptyParticipants, "genesis entry")
	}
	if err := currentConfig.Validate(); err != nil {
		return errors.Wrap(err, "genesis entry")
	}

	// Cloud bootstrap currently emits an unsigned genesis entry before any
	// client-signed config changes exist. Accept that legacy shape, but keep
	// requiring signatures for every subsequent config change.
	if genesis.GetSignature() != nil {
		if err := verifyConfigChangeSignature(genesis, currentConfig); err != nil {
			return errors.Wrap(err, "genesis entry")
		}
	}

	// Establish the effective genesis head before verifying later transitions.
	prevHash, err := HashSOConfigChange(genesis)
	if err != nil {
		return errors.Wrap(err, "hash genesis entry")
	}
	currentConfig = configWithAppliedConfigChainHead(
		currentConfig,
		genesis.GetConfigSeqno(),
		prevHash,
	)

	// Verify each transition under the preceding configuration.
	for i := 1; i < len(entries); i++ {
		currentConfig, err = VerifyConfigChange(currentConfig, entries[i])
		if err != nil {
			return errors.Wrapf(err, "entry[%d]", i)
		}
	}

	return nil
}

// VerifyConfigChange verifies one signed transition against the held configuration
// and returns an independent configuration with its resulting chain head.
// The resulting configuration must pass Validate, so a nonempty result keeps an OWNER.
// An empty held head permits sequence zero for locally authorized bootstrap.
// SELF_ENROLL_PEER requires a separate authenticated peer-to-entity binding.
func VerifyConfigChange(current *SharedObjectConfig, entry *SOConfigChange) (*SharedObjectConfig, error) {
	// Require both configurations before checking the chain and its authority.
	if current == nil || entry.GetConfig() == nil {
		return nil, errors.New("config change requires current and next configurations")
	}
	if !bytes.Equal(entry.GetPreviousHash(), current.GetConfigChainHash()) {
		return nil, errors.New("config change previous_hash does not match current config_chain_hash")
	}
	var expected uint64
	if len(current.GetConfigChainHash()) != 0 {
		expected = current.GetConfigChainSeqno() + 1
		if expected == 0 {
			return nil, errors.New("config change sequence exhausted")
		}
	}
	if entry.GetConfigSeqno() != expected {
		return nil, errors.Errorf("config change seqno %d does not match expected %d", entry.GetConfigSeqno(), expected)
	}
	if err := verifyConfigChangeSignature(entry, current); err != nil {
		return nil, errors.Wrap(err, "verify config change")
	}

	// Derive the accepted head from the signed entry without changing its input.
	entryHash, err := HashSOConfigChange(entry)
	if err != nil {
		return nil, errors.Wrap(err, "hash config change entry")
	}
	next := configWithAppliedConfigChainHead(entry.GetConfig(), entry.GetConfigSeqno(), entryHash)

	// An authorized signer still cannot produce an unusable configuration.
	if err := next.Validate(); err != nil {
		return nil, errors.Wrap(err, "config change result")
	}
	return next, nil
}

// VerifyConfigChainSuffix authenticates a candidate configuration from a held,
// nonempty checkpoint. Every transition must be authorized by its predecessor.
// Genesis and self-enrollment are unavailable on this peer verification path.
// Empty suffixes require every candidate field to equal the checkpoint,
// independent of participant order.
// This verifies configuration authority only, not root or content acceptance.
func VerifyConfigChainSuffix(current, candidate *SharedObjectConfig, entries []*SOConfigChange) error {
	// Trust must already exist at the receiver before remote history is examined.
	if len(current.GetConfigChainHash()) == 0 {
		return errors.New("config suffix requires a nonempty trusted checkpoint")
	}
	if err := current.Validate(); err != nil {
		return errors.Wrap(err, "trusted config")
	}

	// Advance only through owner-signed changes linked to the evolving head.
	for i, entry := range entries {
		switch entry.GetChangeType() {
		case SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_INCREMENT_INVITE_USES:
		default:
			return errors.Errorf("entry[%d]: unsupported peer config change %s", i, entry.GetChangeType())
		}
		next, err := VerifyConfigChange(current, entry)
		if err != nil {
			return errors.Wrapf(err, "entry[%d]", i)
		}
		current = next
	}

	// Bind both the effective configuration and the computed head to the candidate.
	if !EqualSOConfigs(current, candidate) {
		return errors.New("config suffix does not match candidate configuration")
	}
	return nil
}

// configWithAppliedConfigChainHead clones a configuration with a verified head.
func configWithAppliedConfigChainHead(
	cfg *SharedObjectConfig,
	seqno uint64,
	hash []byte,
) *SharedObjectConfig {
	// Preserve an absent configuration for callers validating optional input.
	if cfg == nil {
		return nil
	}

	// Keep returned head metadata independent of caller-owned buffers.
	next := cfg.CloneVT()
	next.ConfigChainSeqno = seqno
	next.ConfigChainHash = bytes.Clone(hash)
	return next
}

// BuildSOConfigChange constructs and signs a SOConfigChange entry.
//
// currentConfig is the config before the change (used for previous_hash and seqno).
// nextConfig is the desired config after the change.
// changeType describes the kind of mutation in this entry.
// signerPrivKey is the private key of an OWNER in the current config, or the
// self-enrolling peer for SELF_ENROLL_PEER changes.
// revInfo is optional revocation metadata (only for REMOVE_PARTICIPANT changes).
func BuildSOConfigChange(
	currentConfig *SharedObjectConfig,
	nextConfig *SharedObjectConfig,
	changeType SOConfigChangeType,
	signerPrivKey crypto.PrivKey,
	revInfo *SORevocationInfo,
) (*SOConfigChange, error) {
	// Compute the next seqno from the current chain state.
	// If config_chain_hash is empty this is a genesis entry (seqno 0).
	// Otherwise the next seqno is config_chain_seqno + 1.
	var nextSeqno uint64
	if len(currentConfig.GetConfigChainHash()) != 0 {
		nextSeqno = currentConfig.GetConfigChainSeqno() + 1
	}

	// Capture the new configuration without changing the caller's signed data.
	entry := &SOConfigChange{
		ConfigSeqno:    nextSeqno,
		Config:         nextConfig.CloneVT(),
		ChangeType:     changeType,
		PreviousHash:   currentConfig.GetConfigChainHash(),
		RevocationInfo: revInfo,
	}

	// Sign the entry.
	data, err := entry.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal config change for signing")
	}
	sig, err := peer.NewSignature("sobject config change", signerPrivKey, hash.HashType_HashType_SHA256, data, true)
	if err != nil {
		return nil, errors.Wrap(err, "sign config change")
	}
	entry.Signature = sig

	return entry, nil
}

// verifyConfigChangeSignature verifies that the SOConfigChange is authorized by
// the given config.
func verifyConfigChangeSignature(entry *SOConfigChange, cfg *SharedObjectConfig) error {
	// Recover the signing peer from the supplied signature.
	sig := entry.GetSignature()
	if sig == nil {
		return errors.New("missing signature")
	}

	sigPubKey, err := sig.ParsePubKey()
	if err != nil {
		return errors.Wrap(err, "parse signature public key")
	}
	if sigPubKey == nil {
		return peer.ErrEmptyPeerID
	}

	sigPeerID, err := peer.IDFromPublicKey(sigPubKey)
	if err != nil {
		return errors.Wrap(err, "derive peer ID from signature")
	}
	sigPeerIDStr := sigPeerID.String()

	// Check the signing peer against the preceding configuration's authority.
	if entry.GetChangeType() == SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER {
		if err := validateSelfEnrollPeerChange(entry, cfg, sigPeerIDStr); err != nil {
			return err
		}
	} else if !isOwnerPeer(cfg, sigPeerIDStr) {
		return errors.Errorf("signer %s is not an OWNER in the config", sigPeerIDStr)
	}

	// Verify the signature over the entry without the signature field.
	clone := entry.CloneVT()
	clone.Signature = nil
	data, err := clone.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal entry for signature verification")
	}

	valid, err := sig.VerifyWithPublic("sobject config change", sigPubKey, data)
	if err != nil {
		return errors.Wrap(err, "verify signature")
	}
	if !valid {
		return errors.New("invalid signature")
	}

	return nil
}

// isOwnerPeer reports whether a peer holds owner authority in the configuration.
func isOwnerPeer(cfg *SharedObjectConfig, peerID string) bool {
	for _, p := range cfg.GetParticipants() {
		if p.GetPeerId() == peerID && IsOwner(p.GetRole()) {
			return true
		}
	}
	return false
}

// participantRoleForEntity returns the strongest role granted to an entity.
func participantRoleForEntity(cfg *SharedObjectConfig, entityID string) SOParticipantRole {
	role := SOParticipantRole_SOParticipantRole_UNKNOWN
	for _, p := range cfg.GetParticipants() {
		if p.GetEntityId() != entityID {
			continue
		}
		if p.GetRole() > role {
			role = p.GetRole()
		}
	}
	return role
}

// validateSelfEnrollPeerChange checks enrollment shape and existing entity role bounds.
// The caller must independently authenticate the peer-to-entity relationship.
func validateSelfEnrollPeerChange(entry *SOConfigChange, cfg *SharedObjectConfig, signerPeerID string) error {
	// Enrollment preserves all configuration metadata.
	if cfg == nil {
		return errors.New("current config is required")
	}
	nextCfg := entry.GetConfig()
	if nextCfg == nil {
		return errors.New("next config is required")
	}
	if (nextCfg.GetConsensusMode() != cfg.GetConsensusMode()) ||
		!bytes.Equal(nextCfg.GetConfigChainHash(), cfg.GetConfigChainHash()) ||
		nextCfg.GetConfigChainSeqno() != cfg.GetConfigChainSeqno() {
		return errors.New("self-enroll may not mutate config metadata")
	}

	// Index the participant sets to check that exactly one peer is added.
	prevParticipants := cfg.GetParticipants()
	nextParticipants := nextCfg.GetParticipants()
	if len(nextParticipants) != len(prevParticipants)+1 {
		return errors.New("self-enroll must add exactly one participant")
	}

	prevByPeer := make(map[string]*SOParticipantConfig, len(prevParticipants))
	for _, p := range prevParticipants {
		prevByPeer[p.GetPeerId()] = p
	}
	nextByPeer := make(map[string]*SOParticipantConfig, len(nextParticipants))
	for _, p := range nextParticipants {
		nextByPeer[p.GetPeerId()] = p
	}

	// Preserve every existing participant and its authority.
	for peerID, prevParticipant := range prevByPeer {
		nextParticipant, ok := nextByPeer[peerID]
		if !ok {
			return errors.New("self-enroll must preserve existing participants")
		}
		if !prevParticipant.EqualVT(nextParticipant) {
			return errors.New("self-enroll may not modify existing participants")
		}
	}

	// Bind the sole added participant to the signature and an existing entity.
	if _, exists := prevByPeer[signerPeerID]; exists {
		return errors.New("self-enroll signer is already a participant")
	}

	addedParticipants := slices.DeleteFunc(
		slices.Clone(nextParticipants),
		func(p *SOParticipantConfig) bool {
			_, ok := prevByPeer[p.GetPeerId()]
			return ok
		},
	)
	if len(addedParticipants) != 1 {
		return errors.New("self-enroll must add exactly one new participant")
	}
	addedParticipant := addedParticipants[0]
	if addedParticipant.GetPeerId() != signerPeerID {
		return errors.New("self-enroll signer must match the added participant")
	}
	if addedParticipant.GetEntityId() == "" {
		return errors.New("self-enroll participant requires entity_id")
	}
	// Note: this validator only verifies the config-chain shape and same-entity
	// role bounds. Callers must separately verify that signerPeerID actually
	// belongs to addedParticipant.entity_id. The cloud path enforces that by
	// binding the authenticated account header to SELF_ENROLL_PEER requests.
	// Any future non-cloud / P2P path must provide an equivalent peer-to-entity
	// binding before accepting this change type.

	// Bound the enrollment role by the entity's existing authority.
	currentRole := participantRoleForEntity(cfg, addedParticipant.GetEntityId())
	if currentRole == SOParticipantRole_SOParticipantRole_UNKNOWN {
		return errors.New("self-enroll entity is not a current participant")
	}
	if addedParticipant.GetRole() > currentRole {
		return errors.New("self-enroll role escalation is not allowed")
	}

	return nil
}
