package provider_spacewave

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
)

// SelfEnrollSpacePeer self-enrolls the authenticated session peer into a
// shared object for the same readable entity using recovery-envelope material.
//
// Returns true when the command wrote new config/key-epoch state, or false when
// the current peer was already enrolled in the current epoch.
func (c *SessionClient) SelfEnrollSpacePeer(
	ctx context.Context,
	entityPriv crypto.PrivKey,
	entityID string,
	spaceID string,
) (bool, error) {
	if c == nil {
		return false, errors.New("session client is required")
	}
	if entityPriv == nil {
		return false, errors.New("entity private key is required")
	}
	if entityID == "" {
		return false, errors.New("entity id is required")
	}
	if spaceID == "" {
		return false, errors.New("space id is required")
	}
	if c.priv == nil {
		return false, errors.New("session private key not available")
	}
	if c.peerID == "" {
		return false, errors.New("session peer id not available")
	}

	for attempt := range maxWriteRetries {
		state, currentCfg, epochs, err := c.loadStandaloneConfigState(ctx, spaceID)
		if err != nil {
			return false, err
		}

		localPeerID := c.peerID.String()
		localParticipant := participantConfigForPeer(currentCfg, localPeerID)
		epoch := currentEpochWithFallback(state, epochs)
		if epoch == nil {
			return false, errSharedObjectCurrentKeyEpochMissing
		}
		if localParticipant != nil && soGrantSliceHasPeerID(epoch.GetGrants(), localPeerID) {
			return false, nil
		}

		role := readableParticipantRoleForEntity(currentCfg, entityID)
		if !sobject.CanReadState(role) {
			return false, sobject.ErrNotParticipant
		}

		env, err := c.GetSORecoveryEnvelope(ctx, spaceID)
		if err != nil {
			return false, err
		}
		if env.GetEntityId() != "" && env.GetEntityId() != entityID {
			return false, sobject.ErrSharedObjectRecoveryEntityMismatch
		}

		material, err := sobject.UnlockSOEntityRecoveryEnvelope([]crypto.PrivKey{entityPriv}, env)
		if err != nil {
			return false, err
		}
		if material.GetEntityId() != "" && material.GetEntityId() != entityID {
			return false, sobject.ErrSharedObjectRecoveryEntityMismatch
		}
		if material.GetGrantInner() == nil {
			return false, errors.New("shared object recovery material is missing grant inner")
		}

		rejoinRole := material.GetRole()
		if !sobject.CanReadState(rejoinRole) || role < rejoinRole {
			rejoinRole = role
		}

		grant, err := sobject.BuildSelfEnrollPeerGrant(
			c.priv,
			c.peerID,
			spaceID,
			material,
		)
		if err != nil {
			return false, errors.Wrap(err, "build self-enroll grant")
		}
		epoch.Grants = append(epoch.GetGrants(), grant)

		var (
			entry       *sobject.SOConfigChange
			entryData   []byte
			recoveryCfg = currentCfg
		)
		if localParticipant == nil {
			entry, err = sobject.BuildSelfEnrollPeerConfigChange(
				currentCfg,
				c.priv,
				localPeerID,
				entityID,
				rejoinRole,
			)
			if err != nil {
				return false, errors.Wrap(err, "build self-enroll config change")
			}
			entryData, err = entry.MarshalVT()
			if err != nil {
				return false, errors.Wrap(err, "marshal self-enroll config change")
			}
			recoveryCfg, err = configWithConfigChangeHash(entry)
			if err != nil {
				return false, errors.Wrap(err, "build recovery config snapshot")
			}
		}

		recoveryEnvelopes, err := buildSORecoveryEnvelopes(
			ctx,
			c,
			spaceID,
			recoveryCfg,
			epoch.GetEpoch(),
			material.GetGrantInner(),
		)
		if err != nil {
			return false, errors.Wrap(err, "build recovery envelopes")
		}

		if localParticipant != nil {
			if err := c.PostKeyEpoch(
				ctx,
				spaceID,
				epoch,
				recoveryEnvelopes,
			); err != nil {
				var ce *cloudError
				if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
					return false, err
				}
				backoff := 100 * time.Millisecond << uint(attempt)
				select {
				case <-ctx.Done():
					return false, ctx.Err()
				case <-time.After(backoff):
				}
				continue
			}
			return true, nil
		}

		if err := c.PostConfigState(
			ctx,
			spaceID,
			entryData,
			nil,
			epoch,
			recoveryEnvelopes,
		); err != nil {
			var ce *cloudError
			if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
				return false, err
			}
			backoff := 100 * time.Millisecond << uint(attempt)
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(backoff):
			}
			continue
		}
		return true, nil
	}

	return false, errors.New("self-enroll recovery failed after max retries due to config conflicts")
}
