package provider_spacewave

import (
	"context"
	"maps"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
)

type missingRecoveryKeypairsError struct {
	entityID string
}

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

	// Seal the grant to each entity's keypairs.
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
