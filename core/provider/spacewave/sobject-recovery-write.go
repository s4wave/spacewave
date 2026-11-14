package provider_spacewave

import (
	"context"
	"maps"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

type missingRecoveryKeypairsError struct {
	entityID string
}

// Error names the entity that has no recovery keypairs.
func (e *missingRecoveryKeypairsError) Error() string {
	return "missing recovery keypairs for entity " + e.entityID
}

// buildSORecoveryEnvelopes builds the full recovery-envelope set for readable
// entity participants on the shared object.
func buildSORecoveryEnvelopes(
	ctx context.Context,
	cli *SessionClient,
	soID string,
	cfg *sobject.SharedObjectConfig,
	keyEpoch uint64,
	grantInner *sobject.SOGrantInner,
) ([]*sobject.SOEntityRecoveryEnvelope, error) {
	// Require a live context, the client, the config and the grant.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cli == nil {
		return nil, errors.New("session client is required")
	}
	if cfg == nil {
		return nil, errors.New("shared object config is required")
	}
	if grantInner == nil {
		return nil, errors.New("grant inner is required")
	}

	// List the keypairs of each readable entity.
	entityRoles := listReadableEntityRoles(cfg)
	if len(entityRoles) == 0 {
		return nil, nil
	}
	entityIDs := slices.Sorted(maps.Keys(entityRoles))
	resp, err := cli.recoveryEntityKeypairs(ctx, soID, entityIDs)
	if err != nil {
		return nil, err
	}
	pubKeysByEntity := make(map[string][]crypto.PubKey, len(resp.GetEntities()))
	for _, entity := range resp.GetEntities() {
		pubs, err := pubKeysFromEntityKeypairs(entity.GetKeypairs())
		if err != nil {
			return nil, errors.Wrapf(err, "extract entity pubkey: %s", entity.GetEntityId())
		}
		pubKeysByEntity[entity.GetEntityId()] = pubs
	}
	return sealSORecoveryEnvelopes(cfg, keyEpoch, grantInner, entityRoles, pubKeysByEntity)
}

// sealSORecoveryEnvelopes seals grantInner to the keys of each entity of
// entityRoles, the readable entities of cfg, in entity order. An entity with
// no keys fails with missingRecoveryKeypairsError.
func sealSORecoveryEnvelopes(
	cfg *sobject.SharedObjectConfig,
	keyEpoch uint64,
	grantInner *sobject.SOGrantInner,
	entityRoles map[string]sobject.SOParticipantRole,
	pubKeysByEntity map[string][]crypto.PubKey,
) ([]*sobject.SOEntityRecoveryEnvelope, error) {
	// Seal the grant to each entity's keypairs, in entity order.
	entityIDs := slices.Sorted(maps.Keys(entityRoles))
	envs := make([]*sobject.SOEntityRecoveryEnvelope, 0, len(entityIDs))
	for _, entityID := range entityIDs {
		pubKeys := pubKeysByEntity[entityID]
		if len(pubKeys) == 0 {
			return nil, &missingRecoveryKeypairsError{entityID: entityID}
		}
		env, err := sobject.BuildSOEntityRecoveryEnvelope(
			entityID,
			keyEpoch,
			cfg,
			&sobject.SOEntityRecoveryMaterial{
				EntityId:   entityID,
				Role:       entityRoles[entityID],
				GrantInner: grantInner.CloneVT(),
			},
			pubKeys,
		)
		if err != nil {
			return nil, errors.Wrapf(err, "build recovery envelope: %s", entityID)
		}
		envs = append(envs, env)
	}
	return envs, nil
}

// GenesisRecoveryEnvelopes seals the grant a genesis state gives its owner
// to the recovery keys of each entity that may read the Space, for a write
// that sends that genesis to the cloud. owner is the genesis signer, whose own
// grant it opens. The keys are the account's own, since a genesis names only
// the account. An account with no recovery keypairs yet gets no envelope, as
// when it creates a Space in the cloud directly.
func (a *ProviderAccount) GenesisRecoveryEnvelopes(ctx context.Context, sharedObjectID string, owner crypto.PrivKey, state *sobject.SOState) ([]*sobject.SOEntityRecoveryEnvelope, error) {
	// Open the owner's grant in the genesis key epoch.
	ownerID, err := peer.IDFromPrivateKey(owner)
	if err != nil {
		return nil, err
	}
	epoch := state.CurrentKeyEpoch()
	idx := slices.IndexFunc(epoch.GetGrants(), func(g *sobject.SOGrant) bool {
		return g.GetPeerId() == ownerID.String()
	})
	if idx < 0 {
		return nil, errors.New("genesis has no grant for its owner")
	}
	inner, err := epoch.GetGrants()[idx].DecryptInnerData(owner, sharedObjectID)
	if err != nil {
		return nil, err
	}

	// Require a genesis that names only the account as a reader.
	cfg := state.GetConfig()
	entityRoles := listReadableEntityRoles(cfg)
	for entityID := range entityRoles {
		if entityID != a.accountID {
			return nil, errors.Errorf("genesis names entity %s besides the account", entityID)
		}
	}
	if len(entityRoles) == 0 {
		return nil, nil
	}

	// Read the account's own keys: the cloud lists a Space's entity keys
	// only once the Space exists.
	keypairs, err := a.GetSessionClient().ListKeypairs(ctx)
	if err != nil {
		return nil, err
	}
	pubs, err := pubKeysFromEntityKeypairs(keypairs)
	if err != nil {
		return nil, errors.Wrap(err, "extract account pubkey")
	}
	if len(pubs) == 0 {
		return nil, nil
	}

	// Seal it to them.
	return sealSORecoveryEnvelopes(cfg, epoch.GetEpoch(), inner, entityRoles, map[string][]crypto.PubKey{a.accountID: pubs})
}

// listReadableEntityRoles returns the highest readable role for each entity.
func listReadableEntityRoles(
	cfg *sobject.SharedObjectConfig,
) map[string]sobject.SOParticipantRole {
	entityRoles := make(map[string]sobject.SOParticipantRole)
	for _, participant := range cfg.GetParticipants() {
		entityID := participant.GetEntityId()
		if entityID == "" {
			continue
		}
		role := participant.GetRole()
		if !sobject.CanReadState(role) {
			continue
		}
		if role > entityRoles[entityID] {
			entityRoles[entityID] = role
		}
	}
	return entityRoles
}

// pubKeysFromEntityKeypairs derives the public keys of one entity's listed
// keypairs.
func pubKeysFromEntityKeypairs(keypairs []*session.EntityKeypair) ([]crypto.PubKey, error) {
	pubs := make([]crypto.PubKey, 0, len(keypairs))
	for _, kp := range keypairs {
		if kp.GetPeerId() == "" {
			continue
		}
		pub, err := session.ExtractPublicKeyFromPeerID(kp.GetPeerId())
		if err != nil {
			return nil, err
		}
		pubs = append(pubs, pub)
	}
	return pubs, nil
}
