package provider_spacewave

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
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

	entityRoles := listReadableEntityRoles(cfg)
	if len(entityRoles) == 0 {
		return nil, nil
	}

	resp, err := cli.ListSORecoveryEntityKeypairs(ctx, soID)
	if err != nil {
		return nil, err
	}

	pubKeysByEntity := make(map[string][]crypto.PubKey, len(resp.GetEntities()))
	for _, entity := range resp.GetEntities() {
		entityID := entity.GetEntityId()
		if entityID == "" {
			continue
		}
		for _, kp := range entity.GetKeypairs() {
			if kp.GetPeerId() == "" {
				continue
			}
			pub, err := session.ExtractPublicKeyFromPeerID(kp.GetPeerId())
			if err != nil {
				return nil, errors.Wrapf(err, "extract entity pubkey: %s", entityID)
			}
			pubKeysByEntity[entityID] = append(pubKeysByEntity[entityID], pub)
		}
	}

	entityIDs := make([]string, 0, len(entityRoles))
	for entityID := range entityRoles {
		entityIDs = append(entityIDs, entityID)
	}
	sort.Strings(entityIDs)

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

// buildSORecoveryEnvelopesFromCache mirrors buildSORecoveryEnvelopes but
// consults the per-entity recovery-keypair cache before issuing
// /recovery-entity-keypairs. A single cold fetch refills the cache and is
// shared across every readable entity in the SO config; subsequent SOs that
// share those entities skip the network round-trip entirely.
func buildSORecoveryEnvelopesFromCache(
	ctx context.Context,
	a *ProviderAccount,
	cli *SessionClient,
	soID string,
	cfg *sobject.SharedObjectConfig,
	keyEpoch uint64,
	grantInner *sobject.SOGrantInner,
) ([]*sobject.SOEntityRecoveryEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil {
		return nil, errors.New("provider account is required")
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

	entityRoles := listReadableEntityRoles(cfg)
	if len(entityRoles) == 0 {
		return nil, nil
	}

	pubKeysByEntity := make(map[string][]crypto.PubKey, len(entityRoles))
	cacheMiss := false
	for entityID := range entityRoles {
		cached, err := a.loadRecoveryEntityKeypairsCache(ctx, entityID)
		if err != nil {
			return nil, errors.Wrapf(err, "load cached entity keypairs: %s", entityID)
		}
		if cached == nil || len(cached.GetKeypairs()) == 0 {
			cacheMiss = true
			continue
		}
		pubs, err := pubKeysFromEntityKeypairs(cached.GetKeypairs())
		if err != nil {
			return nil, errors.Wrapf(err, "extract entity pubkey: %s", entityID)
		}
		if len(pubs) == 0 {
			cacheMiss = true
			continue
		}
		pubKeysByEntity[entityID] = pubs
	}

	if cacheMiss {
		resp, err := cli.ListSORecoveryEntityKeypairs(ctx, soID)
		if err != nil {
			return nil, err
		}
		if err := persistRecoveryEntityKeypairs(ctx, a, resp); err != nil {
			return nil, err
		}
		for _, entity := range resp.GetEntities() {
			entityID := entity.GetEntityId()
			if entityID == "" {
				continue
			}
			pubs, err := pubKeysFromEntityKeypairs(entity.GetKeypairs())
			if err != nil {
				return nil, errors.Wrapf(err, "extract entity pubkey: %s", entityID)
			}
			pubKeysByEntity[entityID] = pubs
		}
	}

	entityIDs := make([]string, 0, len(entityRoles))
	for entityID := range entityRoles {
		entityIDs = append(entityIDs, entityID)
	}
	sort.Strings(entityIDs)

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

// pubKeysFromEntityKeypairs derives the public-key slice for one entity's
// cached keypairs.
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

// persistRecoveryEntityKeypairs decomposes a /recovery-entity-keypairs
// response into per-entity cache entries.
func persistRecoveryEntityKeypairs(
	ctx context.Context,
	a *ProviderAccount,
	resp *api.ListSORecoveryEntityKeypairsResponse,
) error {
	if resp == nil {
		return nil
	}
	for _, entity := range resp.GetEntities() {
		if entity.GetEntityId() == "" {
			continue
		}
		if err := a.writeRecoveryEntityKeypairsCache(ctx, entity); err != nil {
			return errors.Wrapf(err, "write entity keypairs cache: %s", entity.GetEntityId())
		}
	}
	return nil
}
