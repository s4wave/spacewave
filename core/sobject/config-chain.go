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
	clone := entry.CloneVT()
	clone.Signature = nil
	data, err := clone.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal config change for hash")
	}
	h := sha256.Sum256(data)
	return h[:], nil
}

// VerifyConfigChain verifies a config change chain from genesis to current.
// Each entry must have:
// 1. Monotonically increasing config_seqno (starting from 0)
// 2. previous_hash matching the hash of the prior entry (genesis has zero previous_hash)
// 3. Valid authorization according to the change type
func VerifyConfigChain(entries []*SOConfigChange) error {
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

	// Track the current effective config for authorization checks. After an
	// entry is accepted, the config-chain head becomes that entry's hash/seqno
	// even though entry.Config still carries the pre-head metadata snapshot.
	currentConfig := genesis.GetConfig()

	// Cloud bootstrap currently emits an unsigned genesis entry before any
	// client-signed config changes exist. Accept that legacy shape, but keep
	// requiring signatures for every subsequent config change.
	if genesis.GetSignature() == nil {
		if err := validateUnsignedGenesisConfig(currentConfig); err != nil {
			return errors.Wrap(err, "genesis entry")
		}
	} else if err := verifyConfigChangeSignature(genesis, currentConfig); err != nil {
		return errors.Wrap(err, "genesis entry")
	}

	prevHash, err := HashSOConfigChange(genesis)
	if err != nil {
		return errors.Wrap(err, "hash genesis entry")
	}
	currentConfig = configWithAppliedConfigChainHead(
		currentConfig,
		genesis.GetConfigSeqno(),
		prevHash,
	)

	for i := 1; i < len(entries); i++ {
		entry := entries[i]

		// Check seqno is monotonically increasing.
		if entry.GetConfigSeqno() != uint64(i) {
			return errors.Errorf("entry[%d]: expected seqno %d, got %d", i, i, entry.GetConfigSeqno())
		}

		// Check previous_hash matches the hash of the prior entry.
		if !bytes.Equal(entry.GetPreviousHash(), prevHash) {
			return errors.Errorf("entry[%d]: previous_hash does not match", i)
		}

		// Verify authorization from the current config (before this change).
		if err := verifyConfigChangeSignature(entry, currentConfig); err != nil {
			return errors.Wrapf(err, "entry[%d]", i)
		}

		prevHash, err = HashSOConfigChange(entry)
		if err != nil {
			return errors.Wrapf(err, "hash entry[%d]", i)
		}
		currentConfig = configWithAppliedConfigChainHead(
			entry.GetConfig(),
			entry.GetConfigSeqno(),
			prevHash,
		)
	}

	return nil
}

func configWithAppliedConfigChainHead(
	cfg *SharedObjectConfig,
	seqno uint64,
	hash []byte,
) *SharedObjectConfig {
	if cfg == nil {
		return nil
	}
	next := cfg.CloneVT()
	next.ConfigChainSeqno = seqno
	next.ConfigChainHash = slices.Clone(hash)
	return next
}

// validateUnsignedGenesisConfig validates the legacy unsigned genesis config
// emitted by cloud bootstrap. This is a compatibility carve-out only for the
// first config-chain entry.
func validateUnsignedGenesisConfig(cfg *SharedObjectConfig) error {
	if cfg == nil {
		return errors.New("missing config")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	for _, p := range cfg.GetParticipants() {
		if IsOwner(p.GetRole()) {
			return nil
		}
	}
	return errors.New("genesis config has no owner")
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
	sig := entry.GetSignature()
	if sig == nil {
		return errors.New("missing signature")
	}

	sigPubKey, err := sig.ParsePubKey()
	if err != nil {
		return errors.Wrap(err, "parse signature public key")
	}

	sigPeerID, err := peer.IDFromPublicKey(sigPubKey)
	if err != nil {
		return errors.Wrap(err, "derive peer ID from signature")
	}
	sigPeerIDStr := sigPeerID.String()

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

func isOwnerPeer(cfg *SharedObjectConfig, peerID string) bool {
	for _, p := range cfg.GetParticipants() {
		if p.GetPeerId() == peerID && IsOwner(p.GetRole()) {
			return true
		}
	}
	return false
}

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

func sameParticipant(a, b *SOParticipantConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.GetPeerId() == b.GetPeerId() &&
		a.GetRole() == b.GetRole() &&
		a.GetEntityId() == b.GetEntityId()
}

func validateSelfEnrollPeerChange(entry *SOConfigChange, cfg *SharedObjectConfig, signerPeerID string) error {
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

	for peerID, prevParticipant := range prevByPeer {
		nextParticipant, ok := nextByPeer[peerID]
		if !ok {
			return errors.New("self-enroll must preserve existing participants")
		}
		if !sameParticipant(prevParticipant, nextParticipant) {
			return errors.New("self-enroll may not modify existing participants")
		}
	}

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

	currentRole := participantRoleForEntity(cfg, addedParticipant.GetEntityId())
	if currentRole == SOParticipantRole_SOParticipantRole_UNKNOWN {
		return errors.New("self-enroll entity is not a current participant")
	}
	if addedParticipant.GetRole() > currentRole {
		return errors.New("self-enroll role escalation is not allowed")
	}

	return nil
}
