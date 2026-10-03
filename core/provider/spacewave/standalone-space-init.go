package provider_spacewave

import (
	"context"
	"crypto/rand"
	"slices"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	hydra_blockenc "github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// InitEmptyStandaloneSpace bootstraps the initial owner config, key epoch and
// genesis checkpoint for an existing empty cloud shared object. username is
// the owner account's username.
//
// Returns true when initialization wrote the initial state, or false when the
// shared object was already initialized.
func (c *SessionClient) InitEmptyStandaloneSpace(
	ctx context.Context,
	le *logrus.Entry,
	accountID string,
	username string,
	spaceID string,
) (bool, error) {
	// Validate the authenticated client and initialization inputs.
	if c == nil {
		return false, errors.New("session client is required")
	}
	if accountID == "" {
		return false, errors.New("account id is required")
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
	if le == nil {
		le = logrus.New().WithField("component", "standalone-space-init")
	}

	// Load the current shared-object state and config chain.
	state, chain, err := c.loadStandaloneInitState(ctx, spaceID)
	if err != nil {
		return false, err
	}

	// Verify local owner participation and grant state.
	localPeerID := c.peerID.String()
	localParticipant := participantConfigForPeer(state.GetConfig(), localPeerID)
	if localParticipant == nil {
		return false, errors.New("local participant missing on empty space")
	}
	if localParticipant.GetRole() != sobject.SOParticipantRole_SOParticipantRole_OWNER {
		return false, errors.New("local participant is not owner on empty space")
	}
	epoch := currentEpochWithFallback(state, chain.GetKeyEpochs())
	if epoch.FindGrant(localPeerID) != nil {
		return false, nil
	}

	// Initialize a shared object without a checkpoint. An initialized one
	// without the local grant is unreadable by this session.
	if state.GetCheckpoint() != nil {
		return false, errors.New("local grant missing on initialized space")
	}
	if err := initializeCloudSharedObjectState(
		ctx,
		c,
		le,
		accountID,
		username,
		spaceID,
		c.priv,
		buildStandaloneSpaceInitStepFactorySet(),
		false,
	); err != nil {
		return false, err
	}
	return true, nil
}

func (c *SessionClient) loadStandaloneInitState(
	ctx context.Context,
	spaceID string,
) (*sobject.SOState, *sobject.SOConfigChainResponse, error) {
	// Fetch and decode the cloud state snapshot.
	stateData, err := c.GetSOState(ctx, spaceID, 0, SeedReasonColdSeed)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get so state")
	}
	state, _, chain, err := decodeSOStateResponse(stateData)
	if err != nil {
		return nil, nil, errors.Wrap(err, "decode so state")
	}
	if state == nil {
		return nil, nil, errors.New("missing so state snapshot")
	}

	// Load the config chain when the state response omitted it.
	if chain == nil {
		chainData, err := c.GetConfigChain(ctx, spaceID)
		if err != nil {
			return nil, nil, errors.Wrap(err, "get config chain")
		}
		chain = &sobject.SOConfigChainResponse{}
		if err := chain.UnmarshalVT(chainData); err != nil {
			return nil, nil, errors.Wrap(err, "unmarshal config chain")
		}
	}
	return state, chain, nil
}

func buildStandaloneSpaceInitStepFactorySet() *block_transform.StepFactorySet {
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_gzip.NewStepFactory())
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	return sfs
}

// buildInitialWorldStateData returns the encoded World state of a new Space: an
// initialized World when seedWorldHead is set, or nil.
func buildInitialWorldStateData(seedWorldHead bool) ([]byte, error) {
	// Encode an initialized World, unless asked for none.
	if !seedWorldHead {
		return nil, nil
	}
	initOp, err := sobject_world_engine.NewInitWorldOp(nil)
	if err != nil {
		return nil, err
	}
	state, err := sobject_world_engine.BuildInitialInnerState(initOp)
	if err != nil {
		return nil, err
	}
	data, err := state.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal initial world state")
	}
	return data, nil
}

func initializeCloudSharedObjectState(
	ctx context.Context,
	cli *SessionClient,
	le *logrus.Entry,
	accountID string,
	username string,
	sharedObjectID string,
	localPriv crypto.PrivKey,
	sfs *block_transform.StepFactorySet,
	seedWorldHead bool,
) error {
	// Build the signed standalone initialization state.
	state, err := buildStandaloneSpaceInitState(
		ctx,
		cli,
		le,
		accountID,
		username,
		sharedObjectID,
		localPriv,
		sfs,
		seedWorldHead,
		nil,
	)
	if err != nil {
		return err
	}

	// Publish the signed config state.
	if err := cli.PostConfigState(
		ctx,
		sharedObjectID,
		state.configData,
		nil,
		state.keyEpoch,
		state.recoveryEnvelopes,
	); err != nil {
		return errors.Wrap(err, "post signed genesis config")
	}

	// Publish the genesis checkpoint.
	if err := cli.PostCheckpoint(ctx, sharedObjectID, state.checkpoint); err != nil {
		return errors.Wrap(err, "post genesis checkpoint")
	}
	return nil
}

// standaloneSpaceInitState is the signed initial state of a new shared object.
type standaloneSpaceInitState struct {
	// configData is the encoded genesis config change.
	configData []byte
	// keyEpoch is key epoch 0 with a grant for each participant.
	keyEpoch *sobject.SOKeyEpoch
	// recoveryEnvelopes carry the grant material to each entity's recovery keys.
	recoveryEnvelopes []*sobject.SOEntityRecoveryEnvelope
	// checkpoint is the owner-signed genesis checkpoint.
	checkpoint *sobject.SOCheckpoint
}

// buildStandaloneGenesisParticipants builds the genesis participant list. A
// single-owner Space names its owner with username; a friend DM takes each
// account's username from friendAccounts.
func buildStandaloneGenesisParticipants(
	localPeerID peer.ID,
	accountID string,
	username string,
	friendAccounts []*api.FriendDmAccount,
) ([]*sobject.SOParticipantConfig, error) {
	// A single-owner Space holds only the local session.
	if len(friendAccounts) == 0 {
		return []*sobject.SOParticipantConfig{{
			PeerId:   localPeerID.String(),
			Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
			EntityId: accountID,
			Username: username,
		}}, nil
	}

	// Add every session of each friend DM account, with the local account as
	// owner and the friend as writer.
	participants := make([]*sobject.SOParticipantConfig, 0)
	seenAccounts := make(map[string]struct{}, len(friendAccounts))
	seenPeers := make(map[string]struct{})
	for _, account := range friendAccounts {
		// Require a distinct account with active sessions.
		if account.GetAccountId() == "" {
			return nil, errors.New("friend dm account id is required")
		}
		if _, ok := seenAccounts[account.GetAccountId()]; ok {
			return nil, errors.New("friend dm account is duplicated")
		}
		seenAccounts[account.GetAccountId()] = struct{}{}
		if len(account.Sessions) == 0 {
			return nil, errors.Errorf(
				"friend dm account %s has no active sessions",
				account.GetAccountId(),
			)
		}

		// Add each session peer once under the account's role and username.
		role := sobject.SOParticipantRole_SOParticipantRole_WRITER
		if account.GetAccountId() == accountID {
			role = sobject.SOParticipantRole_SOParticipantRole_OWNER
		}
		for _, session := range account.Sessions {
			if session.GetPeerId() == "" {
				return nil, errors.New("friend dm session peer is required")
			}
			if _, ok := seenPeers[session.GetPeerId()]; ok {
				return nil, errors.Errorf(
					"friend dm peer %s appears more than once",
					session.GetPeerId(),
				)
			}
			seenPeers[session.GetPeerId()] = struct{}{}
			participants = append(participants, &sobject.SOParticipantConfig{
				PeerId:   session.GetPeerId(),
				Role:     role,
				EntityId: account.GetAccountId(),
				Username: account.GetEntityId(),
			})
		}
	}

	// Require two accounts, one of which is the local owner.
	if len(seenAccounts) != 2 {
		return nil, errors.New("friend dm requires two accounts")
	}
	if _, ok := seenAccounts[accountID]; !ok {
		return nil, errors.New("friend dm owner account is not present")
	}

	// Require the local session among the participants as an owner.
	localIdx := slices.IndexFunc(participants, func(p *sobject.SOParticipantConfig) bool {
		return p.GetPeerId() == localPeerID.String()
	})
	if localIdx < 0 {
		return nil, errors.New("friend dm owner session is not present")
	}
	local := participants[localIdx]
	if local.GetEntityId() != accountID || !sobject.IsOwner(local.GetRole()) {
		return nil, errors.New("friend dm owner session has non-owner role")
	}
	return participants, nil
}

func buildFriendDmRecoveryEnvelopes(
	accounts []*api.FriendDmAccount,
	cfg *sobject.SharedObjectConfig,
	keyEpoch uint64,
	grantInner *sobject.SOGrantInner,
) ([]*sobject.SOEntityRecoveryEnvelope, error) {
	if cfg == nil {
		return nil, errors.New("friend dm recovery config is required")
	}
	if grantInner == nil {
		return nil, errors.New("friend dm recovery grant material is required")
	}
	entityRoles := listReadableEntityRoles(cfg)
	if len(entityRoles) != len(accounts) {
		return nil, errors.New("friend dm recovery entities do not match accounts")
	}
	envelopes := make([]*sobject.SOEntityRecoveryEnvelope, 0, len(accounts))
	for _, account := range accounts {
		role, ok := entityRoles[account.GetAccountId()]
		if !ok {
			return nil, errors.Errorf(
				"friend dm recovery entity %s is not readable",
				account.GetAccountId(),
			)
		}
		if len(account.RecoveryKeypairs) == 0 {
			return nil, errors.Errorf(
				"friend dm account %s has no recovery keypairs",
				account.GetAccountId(),
			)
		}
		recipientPubs := make([]crypto.PubKey, 0, len(account.RecoveryKeypairs))
		seenRecoveryPeers := make(map[string]struct{}, len(account.RecoveryKeypairs))
		for _, recoveryKeypair := range account.RecoveryKeypairs {
			if _, ok := seenRecoveryPeers[recoveryKeypair.GetPeerId()]; ok {
				return nil, errors.Errorf(
					"friend dm recovery peer %s appears more than once",
					recoveryKeypair.GetPeerId(),
				)
			}
			seenRecoveryPeers[recoveryKeypair.GetPeerId()] = struct{}{}
			pub, err := session.ExtractPublicKeyFromPeerID(recoveryKeypair.GetPeerId())
			if err != nil {
				return nil, errors.Wrapf(
					err,
					"extract friend dm recovery pubkey %s",
					recoveryKeypair.GetPeerId(),
				)
			}
			recipientPubs = append(recipientPubs, pub)
		}
		env, err := sobject.BuildSOEntityRecoveryEnvelope(
			account.GetAccountId(),
			keyEpoch,
			cfg,
			&sobject.SOEntityRecoveryMaterial{
				EntityId:   account.GetAccountId(),
				Role:       role,
				GrantInner: grantInner.CloneVT(),
			},
			recipientPubs,
		)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"build friend dm recovery envelope %s",
				account.GetAccountId(),
			)
		}
		envelopes = append(envelopes, env)
	}
	return envelopes, nil
}

func marshalFriendDmInitialState(
	state *standaloneSpaceInitState,
) ([]byte, []byte, error) {
	// Encode the config and checkpoint request wrappers.
	if state == nil || state.keyEpoch == nil || state.checkpoint == nil {
		return nil, nil, errors.New("friend dm initial state is incomplete")
	}
	configData, err := (&api.PostConfigStateRequest{
		ConfigChange:      state.configData,
		KeyEpoch:          state.keyEpoch,
		RecoveryEnvelopes: state.recoveryEnvelopes,
	}).MarshalVT()
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal friend dm config state")
	}
	checkpointData, err := (&api.PostCheckpointRequest{Checkpoint: state.checkpoint}).MarshalVT()
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal friend dm checkpoint state")
	}
	return configData, checkpointData, nil
}

func buildStandaloneSpaceInitState(
	ctx context.Context,
	cli *SessionClient,
	le *logrus.Entry,
	accountID string,
	username string,
	sharedObjectID string,
	localPriv crypto.PrivKey,
	sfs *block_transform.StepFactorySet,
	seedWorldHead bool,
	friendAccounts []*api.FriendDmAccount,
) (*standaloneSpaceInitState, error) {
	// Create the object's encryption transform and grant secret.
	localPeerID, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		return nil, err
	}
	_, soTransform, grantInner, err := buildInitialSpaceTransform(le, sfs)
	if err != nil {
		return nil, err
	}

	// Sign the genesis config naming the participants.
	participants, err := buildStandaloneGenesisParticipants(
		localPeerID,
		accountID,
		username,
		friendAccounts,
	)
	if err != nil {
		return nil, err
	}
	genesisConfig := &sobject.SharedObjectConfig{
		Participants: participants,
	}
	genesisEntry, err := sobject.BuildSOConfigChange(
		sharedObjectID,
		&sobject.SharedObjectConfig{},
		genesisConfig,
		sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		localPriv,
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "build signed genesis config")
	}
	genesisData, err := genesisEntry.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal signed genesis config")
	}

	// Encrypt a grant for each participant in the first key epoch.
	grants := make([]*sobject.SOGrant, 0, len(participants))
	for _, participant := range participants {
		targetPeer, peerErr := participant.ParsePeerID()
		if peerErr != nil {
			return nil, errors.Wrap(peerErr, "parse genesis participant peer")
		}
		targetPub, pubErr := targetPeer.ExtractPublicKey()
		if pubErr != nil {
			return nil, errors.Wrap(pubErr, "extract genesis participant public key")
		}
		grant, grantErr := sobject.EncryptSOGrant(
			localPriv,
			targetPub,
			sharedObjectID,
			grantInner,
		)
		if grantErr != nil {
			return nil, errors.Wrap(grantErr, "encrypt genesis grant")
		}
		grants = append(grants, grant)
	}
	epoch := &sobject.SOKeyEpoch{Grants: grants}

	// Advance the config to the genesis head for recovery.
	genesisHash, err := sobject.HashSOConfigChange(genesisEntry)
	if err != nil {
		return nil, errors.Wrap(err, "hash signed genesis config")
	}
	genesisConfig = genesisConfig.CloneVT()
	genesisConfig.ConfigChainSeqno = genesisEntry.GetConfigSeqno()
	genesisConfig.ConfigChainHash = genesisHash

	// Build recovery envelopes, skipping an owner that has no recovery keypairs.
	var recoveryEnvelopes []*sobject.SOEntityRecoveryEnvelope
	if len(friendAccounts) > 0 {
		recoveryEnvelopes, err = buildFriendDmRecoveryEnvelopes(
			friendAccounts,
			genesisConfig,
			epoch.GetEpoch(),
			grantInner,
		)
	} else {
		recoveryEnvelopes, err = buildSORecoveryEnvelopes(
			ctx,
			cli,
			sharedObjectID,
			genesisConfig,
			epoch.GetEpoch(),
			grantInner,
		)
		if err != nil {
			var missingErr *missingRecoveryKeypairsError
			if !errors.As(err, &missingErr) || missingErr.entityID != accountID {
				return nil, errors.Wrap(err, "build recovery envelopes")
			}
			recoveryEnvelopes = nil
			err = nil
		}
	}
	if err != nil {
		return nil, errors.Wrap(err, "build friend dm recovery envelopes")
	}

	// Sign the genesis checkpoint holding the encrypted initial World state.
	stateData, err := buildInitialWorldStateData(seedWorldHead)
	if err != nil {
		return nil, err
	}
	if len(stateData) != 0 {
		stateData, err = soTransform.EncodeBlock(stateData)
		if err != nil {
			return nil, errors.Wrap(err, "encrypt initial state")
		}
	}
	checkpoint, err := sobject.BuildGenesisSOCheckpoint(localPriv, sharedObjectID, genesisHash, stateData)
	if err != nil {
		return nil, errors.Wrap(err, "sign genesis checkpoint")
	}
	return &standaloneSpaceInitState{
		configData:        genesisData,
		keyEpoch:          epoch,
		recoveryEnvelopes: recoveryEnvelopes,
		checkpoint:        checkpoint,
	}, nil
}

// buildCreateWithStateRequest validates and builds the create-with-state
// request for the Cloud shared-object route.
func buildCreateWithStateRequest(
	displayName string,
	objectType string,
	ownerType string,
	ownerID string,
	accountPrivate bool,
	configState []byte,
	checkpointState []byte,
) (*api.CreateWithStateRequest, error) {
	if (displayName == "" && objectType == "space") || objectType == "" || ownerType == "" || ownerID == "" {
		return nil, errors.New("space metadata is required")
	}
	if len(configState) == 0 || len(checkpointState) == 0 {
		return nil, errors.New("space initial state is required")
	}
	return &api.CreateWithStateRequest{
		DisplayName:     displayName,
		ObjectType:      objectType,
		OwnerType:       ownerType,
		OwnerId:         ownerID,
		AccountPrivate:  accountPrivate,
		ConfigState:     configState,
		CheckpointState: checkpointState,
	}, nil
}

func buildInitialSpaceTransform(
	le *logrus.Entry,
	sfs *block_transform.StepFactorySet,
) (*block_transform.Config, *block_transform.Transformer, *sobject.SOGrantInner, error) {
	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		return nil, nil, nil, errors.Wrap(err, "generate encryption key")
	}
	soTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: hydra_blockenc.DefaultBlockEnc,
			Key:      encKey,
		},
	})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "build transform config")
	}
	soTransform, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		sfs,
		soTransformConf,
	)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "build transformer")
	}
	grantInner := &sobject.SOGrantInner{TransformConf: soTransformConf}
	return soTransformConf, soTransform, grantInner, nil
}
