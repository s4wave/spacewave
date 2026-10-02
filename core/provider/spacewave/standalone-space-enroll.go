package provider_spacewave

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

// EnrollSpaceMember resolves a target account's current session peers and adds
// them to the shared object as participants using the owner session key.
func (c *SessionClient) EnrollSpaceMember(
	ctx context.Context,
	le *logrus.Entry,
	ownerAccountID string,
	spaceID string,
	accountID string,
	role sobject.SOParticipantRole,
) (*s4wave_provider_spacewave.EnrollSpaceMemberResponse, error) {
	// Require the owner session, the Space and the member account.
	if c == nil {
		return nil, errors.New("session client is required")
	}
	if ownerAccountID == "" {
		return nil, errors.New("owner account id is required")
	}
	if spaceID == "" {
		return nil, errors.New("space id is required")
	}
	if accountID == "" {
		return nil, errors.New("account id is required")
	}
	if c.priv == nil {
		return nil, errors.New("session private key not available")
	}
	if c.peerID == "" {
		return nil, errors.New("session peer id not available")
	}

	// Ask the Cloud for the member's session peers and username.
	enrollResp, err := c.EnrollMember(ctx, spaceID, accountID, true)
	if err != nil {
		return nil, errors.Wrap(err, "resolve member peers")
	}
	peers := enrollResp.GetPeers()
	if len(peers) == 0 {
		return &s4wave_provider_spacewave.EnrollSpaceMemberResponse{}, nil
	}

	// Add each peer and record the outcome.
	results := make([]*s4wave_provider_spacewave.EnrollSpaceMemberResult, 0, len(peers))
	for _, p := range peers {
		peerID := p.GetPeerId()
		result := &s4wave_provider_spacewave.EnrollSpaceMemberResult{PeerId: peerID}
		targetPub, err := session.ExtractPublicKeyFromPeerID(peerID)
		if err != nil {
			result.Error = errors.Wrap(err, "extract pubkey").Error()
			results = append(results, result)
			continue
		}
		grant, err := c.addStandaloneParticipant(
			ctx,
			spaceID,
			accountID,
			enrollResp.GetEntityId(),
			peerID,
			targetPub,
			role,
		)
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			continue
		}
		result.AlreadyParticipant = grant == nil
		result.Enrolled = grant != nil
		results = append(results, result)
	}
	return &s4wave_provider_spacewave.EnrollSpaceMemberResponse{Results: results}, nil
}

// EnrollSpacePeer adds a standalone session peer as a participant using the
// caller's existing grant. It is intended for service-session recovery paths
// where the target peer is known but cannot be resolved through an account
// membership query.
func (c *SessionClient) EnrollSpacePeer(
	ctx context.Context,
	spaceID string,
	peerID string,
	role sobject.SOParticipantRole,
) (bool, error) {
	if c == nil {
		return false, errors.New("session client is required")
	}
	if spaceID == "" {
		return false, errors.New("space id is required")
	}
	if peerID == "" {
		return false, errors.New("peer id is required")
	}
	if c.priv == nil {
		return false, errors.New("session private key not available")
	}
	if c.peerID == "" {
		return false, errors.New("session peer id not available")
	}
	targetPub, err := session.ExtractPublicKeyFromPeerID(peerID)
	if err != nil {
		return false, errors.Wrap(err, "extract pubkey")
	}
	grant, err := c.addStandalonePeerParticipant(ctx, spaceID, peerID, targetPub, role)
	if err != nil {
		return false, err
	}
	return grant != nil, nil
}

// addStandaloneParticipant adds or updates a participant with a root grant.
// It keeps a higher existing role and fills a missing entity or username.
// Returns nil when the participant and its grant are already current.
func (c *SessionClient) addStandaloneParticipant(
	ctx context.Context,
	spaceID string,
	accountID string,
	username string,
	targetPeerID string,
	targetPub crypto.PubKey,
	role sobject.SOParticipantRole,
) (*sobject.SOGrant, error) {
	// Write the change against the latest config, retrying on a conflict.
	for attempt := range maxWriteRetries {
		// Load the latest config, root grants and key epochs.
		state, currentCfg, epochs, err := c.loadStandaloneConfigState(ctx, spaceID)
		if err != nil {
			return nil, err
		}

		// Find the participant and whether it lacks the role, entity or username.
		role = max(role, participantRoleForPeer(
			currentCfg,
			targetPeerID,
			sobject.SOParticipantRole_SOParticipantRole_UNKNOWN,
		))
		participantIdx := slices.IndexFunc(currentCfg.GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
			return p.GetPeerId() == targetPeerID
		})
		participantExists := participantIdx >= 0
		participantNeedsUpdate := false
		if participantExists {
			current := currentCfg.GetParticipants()[participantIdx]
			participantNeedsUpdate = current.GetRole() != role ||
				(current.GetEntityId() == "" && accountID != "") ||
				(current.GetUsername() == "" && username != "")
		}

		// Return early when the participant and its grant are current.
		epoch := currentEpochWithFallback(state, epochs)
		grantExists := soGrantSliceHasPeerID(state.GetRootGrants(), targetPeerID)
		if !grantExists && epoch != nil {
			grantExists = soGrantSliceHasPeerID(epoch.GetGrants(), targetPeerID)
		}
		if participantExists && !participantNeedsUpdate && grantExists {
			return nil, nil
		}

		// Decrypt the local grant to re-encrypt it for the target.
		localPeerIDStr := c.peerID.String()
		localGrant := findSOGrantByPeerID(state.GetRootGrants(), localPeerIDStr)
		if localGrant == nil && epoch != nil {
			localGrant = findSOGrantByPeerID(epoch.GetGrants(), localPeerIDStr)
		}
		if localGrant == nil {
			return nil, errors.New("local grant not found")
		}
		grantInner, err := localGrant.DecryptInnerData(c.priv, spaceID)
		if err != nil {
			return nil, errors.Wrap(err, "decrypt local grant")
		}

		var entry *sobject.SOConfigChange
		var entryData []byte
		if !participantExists || participantNeedsUpdate {
			nextCfg := currentCfg.CloneVT()
			nextParticipant := &sobject.SOParticipantConfig{
				PeerId:   targetPeerID,
				Role:     role,
				EntityId: accountID,
				Username: username,
			}
			if participantExists {
				currentParticipant := currentCfg.GetParticipants()[participantIdx]
				if nextParticipant.GetEntityId() == "" {
					nextParticipant.EntityId = currentParticipant.GetEntityId()
				}
				if nextParticipant.GetUsername() == "" {
					nextParticipant.Username = currentParticipant.GetUsername()
				}
				nextCfg.Participants[participantIdx] = nextParticipant
			} else {
				nextCfg.Participants = append(nextCfg.Participants, nextParticipant)
			}
			entry, err = sobject.BuildSOConfigChange(
				spaceID,
				currentCfg,
				nextCfg,
				sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
				c.priv,
				nil,
			)
			if err != nil {
				return nil, errors.Wrap(err, "build config change")
			}
			entryData, err = entry.MarshalVT()
			if err != nil {
				return nil, errors.Wrap(err, "marshal config change")
			}
		}

		var grant *sobject.SOGrant
		if !grantExists {
			grant, err = sobject.EncryptSOGrant(c.priv, targetPub, spaceID, grantInner)
			if err != nil {
				return nil, errors.Wrap(err, "encrypt grant for target peer")
			}
			if epoch == nil {
				epoch = &sobject.SOKeyEpoch{
					Epoch:      sobject.CurrentEpochNumber(epochs),
					SeqnoStart: state.GetRoot().GetInnerSeqno(),
					Grants:     slices.Clone(state.GetRootGrants()),
				}
			}
			if epoch.GetSeqnoStart() == 0 {
				epoch.SeqnoStart = state.GetRoot().GetInnerSeqno()
			}
			epoch.Grants = append(epoch.GetGrants(), grant)
		}

		var postedEpoch *sobject.SOKeyEpoch
		if !grantExists {
			postedEpoch = epoch
		}

		recoveryCfg, err := recoveryConfigSnapshot(currentCfg, entry)
		if err != nil {
			return nil, errors.Wrap(err, "build recovery config snapshot")
		}
		recoveryKeyEpoch := sobject.CurrentEpochNumber(epochs)
		if postedEpoch != nil {
			recoveryKeyEpoch = postedEpoch.GetEpoch()
		}
		recoveryEnvelopes, err := buildSORecoveryEnvelopes(
			ctx,
			c,
			spaceID,
			recoveryCfg,
			recoveryKeyEpoch,
			grantInner,
		)
		if err != nil {
			var missingErr *missingRecoveryKeypairsError
			if !errors.As(err, &missingErr) || missingErr.entityID != accountID ||
				sobject.CanReadState(readableParticipantRoleForEntity(currentCfg, accountID)) {
				return nil, err
			}
			recoveryEnvelopes, err = buildSORecoveryEnvelopes(
				ctx,
				c,
				spaceID,
				recoveryConfigWithoutEntity(recoveryCfg, accountID),
				recoveryKeyEpoch,
				grantInner,
			)
			if err != nil {
				return nil, err
			}
		}

		if entryData != nil {
			if err := c.PostConfigState(
				ctx,
				spaceID,
				entryData,
				nil,
				postedEpoch,
				recoveryEnvelopes,
			); err != nil {
				var ce *cloudError
				if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
					return nil, err
				}
				continue
			}
		} else if postedEpoch != nil {
			if err := c.PostKeyEpoch(ctx, spaceID, epoch, recoveryEnvelopes); err != nil {
				var ce *cloudError
				if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
					return nil, err
				}
				continue
			}
		}

		return grant, nil
	}

	return nil, errors.New("add participant failed after max retries due to config conflicts")
}

func (c *SessionClient) addStandalonePeerParticipant(
	ctx context.Context,
	spaceID string,
	targetPeerID string,
	targetPub crypto.PubKey,
	role sobject.SOParticipantRole,
) (*sobject.SOGrant, error) {
	if err := sobject.ValidateSOParticipantRole(role, false); err != nil {
		return nil, err
	}
	for attempt := range maxWriteRetries {
		state, currentCfg, epochs, err := c.loadStandaloneConfigState(ctx, spaceID)
		if err != nil {
			return nil, err
		}
		var (
			participantExists bool
			participantIdx    int
			participantCount  int
		)
		for i, p := range currentCfg.GetParticipants() {
			if p.GetPeerId() != targetPeerID {
				continue
			}
			if !participantExists {
				participantExists = true
				participantIdx = i
			}
			participantCount++
		}
		epoch := currentEpochWithFallback(state, epochs)
		grantExists := soGrantSliceHasPeerID(state.GetRootGrants(), targetPeerID)
		if !grantExists && epoch != nil {
			grantExists = soGrantSliceHasPeerID(epoch.GetGrants(), targetPeerID)
		}
		participantNeedsUpdate := !participantExists ||
			participantCount != 1 ||
			currentCfg.GetParticipants()[participantIdx].GetRole() != role ||
			currentCfg.GetParticipants()[participantIdx].GetEntityId() != ""
		if participantExists && !participantNeedsUpdate && grantExists {
			return nil, nil
		}

		localPeerIDStr := c.peerID.String()
		localGrant := findSOGrantByPeerID(state.GetRootGrants(), localPeerIDStr)
		if localGrant == nil && epoch != nil {
			localGrant = findSOGrantByPeerID(epoch.GetGrants(), localPeerIDStr)
		}
		if localGrant == nil {
			return nil, errors.New("local grant not found")
		}
		grantInner, err := localGrant.DecryptInnerData(c.priv, spaceID)
		if err != nil {
			return nil, errors.Wrap(err, "decrypt local grant")
		}

		var entry *sobject.SOConfigChange
		var entryData []byte
		if !participantExists || participantNeedsUpdate {
			nextCfg := currentCfg.CloneVT()
			nextParticipant := &sobject.SOParticipantConfig{
				PeerId: targetPeerID,
				Role:   role,
			}
			if participantExists {
				nextCfg.Participants = removeParticipantPeerID(nextCfg.GetParticipants(), targetPeerID)
			}
			nextCfg.Participants = append(nextCfg.Participants, nextParticipant)
			entry, err = sobject.BuildSOConfigChange(
				spaceID,
				currentCfg,
				nextCfg,
				sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
				c.priv,
				nil,
			)
			if err != nil {
				return nil, errors.Wrap(err, "build config change")
			}
			entryData, err = entry.MarshalVT()
			if err != nil {
				return nil, errors.Wrap(err, "marshal config change")
			}
		}

		var grant *sobject.SOGrant
		if !grantExists {
			grant, err = sobject.EncryptSOGrant(c.priv, targetPub, spaceID, grantInner)
			if err != nil {
				return nil, errors.Wrap(err, "encrypt grant for target peer")
			}
			if epoch == nil {
				epoch = &sobject.SOKeyEpoch{
					Epoch:      sobject.CurrentEpochNumber(epochs),
					SeqnoStart: state.GetRoot().GetInnerSeqno(),
					Grants:     slices.Clone(state.GetRootGrants()),
				}
			}
			if epoch.GetSeqnoStart() == 0 {
				epoch.SeqnoStart = state.GetRoot().GetInnerSeqno()
			}
			epoch.Grants = append(epoch.GetGrants(), grant)
		}

		var postedEpoch *sobject.SOKeyEpoch
		if !grantExists {
			postedEpoch = epoch
		}
		recoveryCfg, err := recoveryConfigSnapshot(currentCfg, entry)
		if err != nil {
			return nil, errors.Wrap(err, "build recovery config snapshot")
		}
		recoveryKeyEpoch := sobject.CurrentEpochNumber(epochs)
		if postedEpoch != nil {
			recoveryKeyEpoch = postedEpoch.GetEpoch()
		}
		recoveryEnvelopes, err := buildSORecoveryEnvelopes(
			ctx,
			c,
			spaceID,
			recoveryCfg,
			recoveryKeyEpoch,
			grantInner,
		)
		if err != nil {
			return nil, err
		}

		if entryData != nil {
			if err := c.PostConfigState(
				ctx,
				spaceID,
				entryData,
				nil,
				postedEpoch,
				recoveryEnvelopes,
			); err != nil {
				var ce *cloudError
				if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
					return nil, err
				}
				continue
			}
		} else if postedEpoch != nil {
			if err := c.PostKeyEpoch(ctx, spaceID, epoch, recoveryEnvelopes); err != nil {
				var ce *cloudError
				if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
					return nil, err
				}
				continue
			}
		}

		return grant, nil
	}

	return nil, errors.New("add peer participant failed after max retries due to config conflicts")
}

func removeParticipantPeerID(
	participants []*sobject.SOParticipantConfig,
	peerID string,
) []*sobject.SOParticipantConfig {
	next := participants[:0]
	for _, participant := range participants {
		if participant.GetPeerId() != peerID {
			next = append(next, participant)
		}
	}
	return next
}

func (c *SessionClient) loadStandaloneConfigState(
	ctx context.Context,
	spaceID string,
) (*sobject.SOState, *sobject.SharedObjectConfig, []*sobject.SOKeyEpoch, error) {
	stateData, err := c.GetSOState(ctx, spaceID, 0, SeedReasonReconnect)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "get so state")
	}
	state, _, chain, err := decodeSOStateResponse(stateData)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "decode so state")
	}
	if state == nil {
		return nil, nil, nil, errors.New("missing so state snapshot")
	}
	currentCfg := &sobject.SharedObjectConfig{}
	if cfg := state.GetConfig(); cfg != nil {
		currentCfg = cfg.CloneVT()
	}

	if chain == nil {
		chainData, err := c.GetConfigChain(ctx, spaceID)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "get config chain")
		}
		chain = &sobject.SOConfigChainResponse{}
		if err := chain.UnmarshalVT(chainData); err != nil {
			return nil, nil, nil, errors.Wrap(err, "unmarshal config chain")
		}
	}
	return state, currentCfg, cloneVTSlice(chain.GetKeyEpochs()), nil
}
