package sobject

import (
	"context"
	"crypto/rand"

	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/envelope"
	bifrost_peer "github.com/s4wave/spacewave/net/peer"
)

// soEntityRecoveryEnvelopeContext is the application context string for SO recovery envelopes.
const soEntityRecoveryEnvelopeContext = "sobject entity recovery v1"

// BuildSOEntityRecoveryEnvelope builds an entity recovery envelope for the current SO grant material.
func BuildSOEntityRecoveryEnvelope(
	entityID string,
	keyEpoch uint64,
	cfg *SharedObjectConfig,
	material *SOEntityRecoveryMaterial,
	recipientPubs []bifrost_crypto.PubKey,
) (*SOEntityRecoveryEnvelope, error) {
	// Validate recovery inputs and recipients.
	if entityID == "" {
		return nil, errors.New("entity id is required")
	}
	if cfg == nil {
		return nil, errors.New("shared object config is required")
	}
	if material == nil {
		return nil, errors.New("recovery material is required")
	}
	if len(recipientPubs) == 0 {
		return nil, errors.New("at least one recipient pubkey is required")
	}

	// Marshal and scrub the recovery material.
	materialData, err := material.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal recovery material")
	}
	defer scrub.Scrub(materialData)

	// Build one envelope grant for each recipient.
	grantConfigs := make([]*envelope.EnvelopeGrantConfig, len(recipientPubs))
	for i := range recipientPubs {
		grantConfigs[i] = &envelope.EnvelopeGrantConfig{
			ShareCount:     1,
			KeypairIndexes: []uint32{uint32(i)}, //nolint:gosec
		}
	}
	config := &envelope.EnvelopeConfig{
		Threshold:    0,
		GrantConfigs: grantConfigs,
	}

	// Encrypt the recovery payload for all recipients.
	env, err := envelope.BuildEnvelope(
		rand.Reader,
		soEntityRecoveryEnvelopeContext,
		materialData,
		recipientPubs,
		config,
	)
	if err != nil {
		return nil, errors.Wrap(err, "build recovery envelope")
	}

	// Marshal the encrypted envelope.
	envData, err := env.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal recovery envelope")
	}

	// Return the envelope with config-chain identity.
	return &SOEntityRecoveryEnvelope{
		EntityId:         entityID,
		KeyEpoch:         keyEpoch,
		ConfigChainSeqno: cfg.GetConfigChainSeqno(),
		ConfigChainHash:  cfg.GetConfigChainHash(),
		EnvelopeData:     envData,
	}, nil
}

// UnlockSOEntityRecoveryEnvelope decrypts a recovery envelope into recovery material.
func UnlockSOEntityRecoveryEnvelope(entityPrivKeys []bifrost_crypto.PrivKey, env *SOEntityRecoveryEnvelope) (*SOEntityRecoveryMaterial, error) {
	// Validate and decode the recovery envelope.
	if env == nil {
		return nil, errors.New("recovery envelope is required")
	}
	if len(entityPrivKeys) == 0 {
		return nil, ErrSharedObjectRecoveryCredentialRequired
	}

	// Read and authenticate the envelope payload.
	data := env.GetEnvelopeData()
	if len(data) == 0 {
		return nil, errors.New("recovery envelope data is required")
	}

	// Decode the recovery envelope message.
	envMsg := &envelope.Envelope{}
	if err := envMsg.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal recovery envelope")
	}

	// Unlock the envelope with entity keys.
	payload, result, err := envelope.UnlockEnvelope(
		soEntityRecoveryEnvelopeContext,
		envMsg,
		entityPrivKeys,
	)
	if err != nil {
		return nil, errors.Wrap(err, "unlock recovery envelope")
	}
	if !result.GetSuccess() {
		return nil, ErrSharedObjectRecoveryCredentialRequired
	}
	defer scrub.Scrub(payload)

	// Decode the recovered material.
	material := &SOEntityRecoveryMaterial{}
	if err := material.UnmarshalVT(payload); err != nil {
		return nil, errors.Wrap(err, "unmarshal recovery material")
	}
	return material, nil
}

// BuildSelfEnrollPeerConfigChange builds a SELF_ENROLL_PEER config change for a
// missing same-entity session peer.
func BuildSelfEnrollPeerConfigChange(
	currentCfg *SharedObjectConfig,
	signerPriv bifrost_crypto.PrivKey,
	signerPeerID string,
	entityID string,
	role SOParticipantRole,
) (*SOConfigChange, error) {
	// Validate self-enrollment inputs.
	if currentCfg == nil {
		return nil, errors.New("current config is required")
	}
	if signerPriv == nil {
		return nil, errors.New("signer private key is required")
	}
	if signerPeerID == "" {
		return nil, errors.New("signer peer id is required")
	}
	if entityID == "" {
		return nil, errors.New("entity id is required")
	}
	if err := ValidateSOParticipantRole(role, false); err != nil {
		return nil, err
	}

	// Append the enrolling peer to the next configuration.
	nextCfg := currentCfg.CloneVT()
	nextCfg.Participants = append(nextCfg.Participants, &SOParticipantConfig{
		PeerId:   signerPeerID,
		Role:     role,
		EntityId: entityID,
	})

	// Sign the self-enrollment configuration change.
	return BuildSOConfigChange(
		currentCfg,
		nextCfg,
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER,
		signerPriv,
		nil,
	)
}

// BuildSelfEnrollPeerGrant builds a self-issued grant for the newly enrolled
// peer using recovered grant material.
func BuildSelfEnrollPeerGrant(
	signerPriv bifrost_crypto.PrivKey,
	peerID bifrost_peer.ID,
	sharedObjectID string,
	material *SOEntityRecoveryMaterial,
) (*SOGrant, error) {
	if signerPriv == nil {
		return nil, errors.New("signer private key is required")
	}
	if peerID == "" {
		return nil, errors.New("peer id is required")
	}
	if sharedObjectID == "" {
		return nil, errors.New("shared object id is required")
	}
	if material == nil || material.GetGrantInner() == nil {
		return nil, errors.New("recovery grant material is required")
	}
	pub, err := peerID.ExtractPublicKey()
	if err != nil {
		return nil, errors.Wrap(err, "extract peer public key")
	}
	return EncryptSOGrant(
		signerPriv,
		pub,
		sharedObjectID,
		material.GetGrantInner(),
	)
}

// ResolveSharedObjectRecoveryMaterial resolves the recovery material for an SO using the provider feature surface.
func ResolveSharedObjectRecoveryMaterial(
	ctx context.Context,
	provAcc provider.ProviderAccount,
	ref *SharedObjectRef,
) (*SOEntityRecoveryMaterial, error) {
	// Resolve the recovery provider feature.
	recoveryProv, err := GetSharedObjectRecoveryProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, err
	}

	// Resolve the current entity identity.
	entityID, err := recoveryProv.GetSelfEntityID(ctx)
	if err != nil {
		return nil, err
	}
	if entityID == "" {
		return nil, errors.New("self entity id is required")
	}

	// Read and validate the recovery envelope.
	env, err := recoveryProv.ReadSharedObjectRecoveryEnvelope(ctx, ref)
	if err != nil {
		return nil, err
	}
	if env.GetEntityId() != "" && env.GetEntityId() != entityID {
		return nil, ErrSharedObjectRecoveryEntityMismatch
	}

	// Acquire the recovery decoder.
	dec, err := recoveryProv.GetSharedObjectRecoveryDecoder(ctx)
	if err != nil {
		return nil, err
	}

	// Decrypt and validate recovered material.
	material, err := dec.DecryptSharedObjectRecoveryEnvelope(ctx, env)
	if err != nil {
		return nil, err
	}
	if material.GetEntityId() != "" && material.GetEntityId() != entityID {
		return nil, ErrSharedObjectRecoveryEntityMismatch
	}
	return material, nil
}
