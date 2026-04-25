package provider_spacewave

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

// tryRecoverMissingSharedObjectPeer repairs same-entity missing-peer access for
// a cloud shared object before mount completes.
func (t *sobjectTracker) tryRecoverMissingSharedObjectPeer(
	ctx context.Context,
	ref *sobject.SharedObjectRef,
	so *SharedObject,
	cli *SessionClient,
) error {
	if !t.a.canSelfEnrollCloudObjects() {
		return nil
	}

	// Cold-start gate: if the hydrated verified cache shows our local peer
	// already holds a grant in the current key epoch, the entity is enrolled
	// and rejoin would just reissue the same enrollment. Skip the entire
	// recovery path to avoid GET /recovery-envelope, GET
	// /recovery-entity-keypairs, and POST /config-state on every cold mount.
	// A stale cache (key rotation that excluded us between mounts) is
	// recovered on the next config-chain re-verify, which still rotates the
	// host into the new participant set.
	if peerEnrolledInCurrentEpoch(so.host.GetKeyEpochs(), so.localPid.String()) {
		t.a.le.WithField("sobject-id", t.id).
			WithField("peer-id", so.localPid.String()).
			Debug("self-rejoin gate held: peer already enrolled in cached current epoch")
		return nil
	}

	if err := so.host.ensureInitialState(ctx, SeedReasonRejoin); err != nil {
		return errors.Wrap(err, "initial state pull")
	}

	relLock, err := so.host.writeMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer relLock()

	for attempt := range maxWriteRetries {
		state, currentCfg, epochs, err := so.loadLatestConfigState(ctx)
		if err != nil {
			return err
		}
		localPeerID := so.localPid.String()
		localParticipant := participantConfigForPeer(currentCfg, localPeerID)
		epoch := currentEpochWithFallback(state, epochs)
		if epoch == nil {
			return errSharedObjectCurrentKeyEpochMissing
		}
		if localParticipant != nil && soGrantSliceHasPeerID(epoch.GetGrants(), localPeerID) {
			stateGrantExists := state != nil &&
				soGrantSliceHasPeerID(state.GetRootGrants(), localPeerID)
			if !peerEnrolledInCurrentEpoch(epochs, localPeerID) || !stateGrantExists {
				so.host.applyKeyEpoch(ctx, epoch)
			}
			return nil
		}

		role := readableParticipantRoleForEntity(currentCfg, t.a.accountID)
		if !sobject.CanReadState(role) {
			return sobject.ErrNotParticipant
		}

		material, err := t.resolveRejoinRecoveryMaterial(
			ctx,
			ref,
			so.GetSharedObjectID(),
			cli,
			epoch.GetEpoch(),
		)
		if err != nil {
			return err
		}
		if material.GetGrantInner() == nil {
			return errors.New("shared object recovery material is missing grant inner")
		}

		rejoinRole := material.GetRole()
		if !sobject.CanReadState(rejoinRole) || role < rejoinRole {
			rejoinRole = role
		}

		grant, err := sobject.BuildSelfEnrollPeerGrant(
			so.privKey,
			so.localPid,
			so.GetSharedObjectID(),
			material,
		)
		if err != nil {
			return errors.Wrap(err, "build self-enroll grant")
		}
		epoch.Grants = append(epoch.GetGrants(), grant)

		var entry *sobject.SOConfigChange
		var entryData []byte
		recoveryCfg := currentCfg
		if localParticipant == nil {
			entry, err = sobject.BuildSelfEnrollPeerConfigChange(
				currentCfg,
				so.privKey,
				localPeerID,
				t.a.accountID,
				rejoinRole,
			)
			if err != nil {
				return errors.Wrap(err, "build self-enroll config change")
			}
			entryData, err = entry.MarshalVT()
			if err != nil {
				return errors.Wrap(err, "marshal self-enroll config change")
			}
			recoveryCfg, err = configWithConfigChangeHash(entry)
			if err != nil {
				return errors.Wrap(err, "build recovery config snapshot")
			}
		}
		recoveryEnvelopes, err := buildSORecoveryEnvelopesFromCache(
			ctx,
			t.a,
			cli,
			so.GetSharedObjectID(),
			recoveryCfg,
			epoch.GetEpoch(),
			material.GetGrantInner(),
		)
		if err != nil {
			return errors.Wrap(err, "build recovery envelopes")
		}

		if localParticipant != nil {
			if err := cli.PostKeyEpoch(
				ctx,
				so.GetSharedObjectID(),
				epoch,
				recoveryEnvelopes,
			); err != nil {
				var ce *cloudError
				if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
					return err
				}
				// Exponential backoff: 100ms, 200ms, 400ms.
				backoff := 100 * time.Millisecond << uint(attempt)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
				continue
			}
			t.persistOwnRecoveryEnvelope(ctx, so.GetSharedObjectID(), recoveryEnvelopes)
			so.host.applyKeyEpoch(ctx, epoch)
			return nil
		}

		if err := cli.PostConfigState(
			ctx,
			so.GetSharedObjectID(),
			entryData,
			nil,
			epoch,
			recoveryEnvelopes,
		); err != nil {
			var ce *cloudError
			if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
				return err
			}
			// Exponential backoff: 100ms, 200ms, 400ms.
			backoff := 100 * time.Millisecond << uint(attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			continue
		}
		t.persistOwnRecoveryEnvelope(ctx, so.GetSharedObjectID(), recoveryEnvelopes)
		return so.host.applyConfigMutation(ctx, entry, nil, epoch)
	}

	return errors.New("self-enroll recovery failed after max retries due to config conflicts")
}

// resolveRejoinRecoveryMaterial resolves recovery material via the cache-aware
// classifier. It returns the decrypted material for the local entity, fetching
// /recovery-envelope and/or /recovery-entity-keypairs only when the
// corresponding caches are cold or stale relative to currentEpoch.
func (t *sobjectTracker) resolveRejoinRecoveryMaterial(
	ctx context.Context,
	ref *sobject.SharedObjectRef,
	soID string,
	cli *SessionClient,
	currentEpoch uint64,
) (*sobject.SOEntityRecoveryMaterial, error) {
	cachedEnv, err := t.a.loadRecoveryEnvelopeCache(ctx, soID)
	if err != nil {
		t.a.le.WithError(err).WithField("sobject-id", soID).
			Warn("failed to load cached recovery envelope")
		cachedEnv = nil
	}
	envCacheValid := cachedEnv != nil &&
		cachedEnv.GetKeyEpoch() == currentEpoch &&
		(cachedEnv.GetEntityId() == "" || cachedEnv.GetEntityId() == t.a.accountID)

	if envCacheValid {
		material, err := t.decryptRecoveryEnvelope(ctx, cachedEnv)
		if err == nil {
			return material, nil
		}
		// Cached envelope failed to decrypt: clear it and fall through to a
		// fresh fetch on this attempt rather than waiting for the outer
		// maxWriteRetries loop. The failure is exceptional (key rotation
		// between mounts, corrupted persisted bytes); reclassify in-line.
		if delErr := t.a.deleteRecoveryEnvelopeCache(ctx, soID); delErr != nil {
			t.a.le.WithError(delErr).WithField("sobject-id", soID).
				Warn("failed to clear stale recovery envelope cache")
		}
		t.a.le.WithError(err).WithField("sobject-id", soID).
			Debug("cached recovery envelope failed to decrypt; refetching")
	}

	fetchedEnv, err := cli.GetSORecoveryEnvelope(ctx, soID)
	if err != nil {
		return nil, err
	}
	if fetchedEnv.GetEntityId() != "" && fetchedEnv.GetEntityId() != t.a.accountID {
		return nil, sobject.ErrSharedObjectRecoveryEntityMismatch
	}
	material, err := t.decryptRecoveryEnvelope(ctx, fetchedEnv)
	if err != nil {
		return nil, err
	}
	if writeErr := t.a.writeRecoveryEnvelopeCache(ctx, soID, fetchedEnv); writeErr != nil {
		t.a.le.WithError(writeErr).WithField("sobject-id", soID).
			Warn("failed to persist recovery envelope cache")
	}
	return material, nil
}

// decryptRecoveryEnvelope decrypts env into recovery material using the
// account's cached entity keys.
func (t *sobjectTracker) decryptRecoveryEnvelope(
	ctx context.Context,
	env *sobject.SOEntityRecoveryEnvelope,
) (*sobject.SOEntityRecoveryMaterial, error) {
	decoder, err := t.a.GetSharedObjectRecoveryDecoder(ctx)
	if err != nil {
		return nil, err
	}
	material, err := decoder.DecryptSharedObjectRecoveryEnvelope(ctx, env)
	if err != nil {
		return nil, err
	}
	if material.GetEntityId() != "" && material.GetEntityId() != t.a.accountID {
		return nil, sobject.ErrSharedObjectRecoveryEntityMismatch
	}
	return material, nil
}

// persistOwnRecoveryEnvelope stores the local entity's freshly built recovery
// envelope so subsequent mounts of soID hit the cache. Failures are warned and
// not propagated; the cache is opportunistic.
func (t *sobjectTracker) persistOwnRecoveryEnvelope(
	ctx context.Context,
	soID string,
	envs []*sobject.SOEntityRecoveryEnvelope,
) {
	for _, env := range envs {
		if env == nil || env.GetEntityId() != t.a.accountID {
			continue
		}
		if err := t.a.writeRecoveryEnvelopeCache(ctx, soID, env); err != nil {
			t.a.le.WithError(err).WithField("sobject-id", soID).
				Warn("failed to persist post-rejoin recovery envelope")
		}
		return
	}
}

func readableParticipantRoleForEntity(
	cfg *sobject.SharedObjectConfig,
	entityID string,
) sobject.SOParticipantRole {
	var role sobject.SOParticipantRole
	for _, participant := range cfg.GetParticipants() {
		if participant.GetEntityId() != entityID {
			continue
		}
		nextRole := participant.GetRole()
		if !sobject.CanReadState(nextRole) {
			continue
		}
		if nextRole > role {
			role = nextRole
		}
	}
	return role
}

// peerEnrolledInCurrentEpoch reports whether peerID has a grant in the
// current (highest-numbered) key epoch from the supplied list. Used as the
// cold-start rejoin precondition: a hydrated verified cache containing such
// a grant proves the peer is already enrolled, so the recovery sweep can
// short-circuit.
func peerEnrolledInCurrentEpoch(epochs []*sobject.SOKeyEpoch, peerID string) bool {
	if len(epochs) == 0 || peerID == "" {
		return false
	}
	current := sobject.CurrentEpochNumber(epochs)
	for _, ep := range epochs {
		if ep.GetEpoch() != current {
			continue
		}
		for _, grant := range ep.GetGrants() {
			if grant.GetPeerId() == peerID {
				return true
			}
		}
	}
	return false
}

func participantConfigForPeer(
	cfg *sobject.SharedObjectConfig,
	peerID string,
) *sobject.SOParticipantConfig {
	for _, participant := range cfg.GetParticipants() {
		if participant.GetPeerId() == peerID {
			return participant
		}
	}
	return nil
}
