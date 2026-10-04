package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

// fillParticipantAccounts records the account of each participant whose
// config entry lacks one, when this session owns the Space. The local peer
// belongs to this account; an admitted peer takes the account of its accepted
// mailbox entry. Peers with neither stay blank.
func (t *sobjectTracker) fillParticipantAccounts(
	ctx context.Context,
	so *SharedObject,
	cli *SessionClient,
) error {
	// Only the owner may record accounts.
	if !t.a.canAccessOwnerMailbox() {
		return nil
	}
	state, err := so.GetSOHost().GetHostState(ctx)
	if err != nil {
		return err
	}
	cfg := state.GetConfig()
	localPeerID := so.localPid.String()
	local := participantConfigForPeer(cfg, localPeerID)
	if local.GetRole() != sobject.SOParticipantRole_SOParticipantRole_OWNER {
		return nil
	}

	// Collect the participants without an account.
	blank := make(map[string]struct{})
	for _, participant := range cfg.GetParticipants() {
		if participant.GetEntityId() == "" {
			blank[participant.GetPeerId()] = struct{}{}
		}
	}
	if len(blank) == 0 {
		return nil
	}

	// Match the local peer to this account and the rest to accepted entries.
	accounts := make(map[string]string, len(blank))
	if _, ok := blank[localPeerID]; ok {
		accounts[localPeerID] = t.a.accountID
		delete(blank, localPeerID)
	}
	if len(blank) != 0 {
		resp, err := cli.GetAcceptedMailboxEntries(ctx, so.GetSharedObjectID())
		if err != nil {
			return err
		}
		for _, entry := range resp.GetEntries() {
			if _, ok := blank[entry.GetPeerId()]; ok && entry.GetAccountId() != "" {
				accounts[entry.GetPeerId()] = entry.GetAccountId()
			}
		}
	}
	if len(accounts) == 0 {
		return nil
	}
	return so.FillParticipantAccounts(ctx, accounts)
}

// FillParticipantAccounts sets the account of each participant named in
// accounts, keyed by peer ID, in one signed config change. It leaves a
// participant that already has an account unchanged.
func (s *SharedObject) FillParticipantAccounts(
	ctx context.Context,
	accounts map[string]string,
) error {
	// Serialize writes to this object and take a write session.
	relLock, err := s.host.writeMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer relLock()
	cli, err := s.getReadyWriteSessionClient(ctx)
	if err != nil {
		return err
	}

	// Write the change against the latest config, retrying on a conflict.
	for attempt := range maxWriteRetries {
		// Fill the blank accounts in the latest config.
		state, currentCfg, epochs, err := s.loadLatestConfigState(ctx)
		if err != nil {
			return err
		}
		nextCfg := currentCfg.CloneVT()
		filled := make(map[string]struct{})
		for _, participant := range nextCfg.GetParticipants() {
			accountID := accounts[participant.GetPeerId()]
			if participant.GetEntityId() == "" && accountID != "" {
				participant.EntityId = accountID
				filled[accountID] = struct{}{}
			}
		}
		if len(filled) == 0 {
			return nil
		}

		// Sign the change.
		entry, err := sobject.BuildSOConfigChange(
			s.GetSharedObjectID(),
			currentCfg,
			nextCfg,
			sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
			s.privKey,
			nil,
		)
		if err != nil {
			return errors.Wrap(err, "build config change")
		}
		entryData, err := entry.MarshalVT()
		if err != nil {
			return errors.Wrap(err, "marshal config change")
		}

		// Seal recovery envelopes for the new config's entities.
		epoch := currentEpochWithFallback(state, epochs)
		if epoch == nil {
			return errSharedObjectCurrentKeyEpochMissing
		}
		recoveryCfg, err := configWithConfigChangeHash(entry)
		if err != nil {
			return errors.Wrap(err, "build recovery config snapshot")
		}
		recoveryEnvelopes, err := s.buildFilledRecoveryEnvelopes(
			ctx,
			cli,
			epoch,
			currentCfg,
			recoveryCfg,
			filled,
		)
		if err != nil {
			return err
		}

		// Post the change, retrying a config conflict.
		err = cli.PostConfigState(
			ctx,
			s.GetSharedObjectID(),
			entryData,
			nil,
			nil,
			recoveryEnvelopes,
		)
		var ce *cloudError
		if errors.As(err, &ce) && ce.StatusCode == 409 && attempt+1 < maxWriteRetries {
			continue
		}
		return err
	}
	return errors.New("fill participant accounts failed after max retries due to config conflicts")
}

// buildFilledRecoveryEnvelopes seals the current grant for the entities of
// recoveryCfg. A filled account that was not readable before and has no
// recovery keypairs gets no envelope, as when the participant was added.
func (s *SharedObject) buildFilledRecoveryEnvelopes(
	ctx context.Context,
	cli *SessionClient,
	epoch *sobject.SOKeyEpoch,
	currentCfg *sobject.SharedObjectConfig,
	recoveryCfg *sobject.SharedObjectConfig,
	filled map[string]struct{},
) ([]*sobject.SOEntityRecoveryEnvelope, error) {
	for {
		// Seal envelopes for every entity left in the config.
		recoveryEnvelopes, err := s.buildRecoveryEnvelopesForConfig(
			ctx,
			cli,
			epoch,
			recoveryCfg,
			epoch.GetEpoch(),
		)
		var missingErr *missingRecoveryKeypairsError
		if !errors.As(err, &missingErr) {
			return recoveryEnvelopes, err
		}

		// Skip a newly filled entity without keypairs, once.
		entityID := missingErr.entityID
		if _, ok := filled[entityID]; !ok ||
			sobject.CanReadState(readableParticipantRoleForEntity(currentCfg, entityID)) {
			return nil, err
		}
		delete(filled, entityID)
		recoveryCfg = recoveryConfigWithoutEntity(recoveryCfg, entityID)
	}
}
