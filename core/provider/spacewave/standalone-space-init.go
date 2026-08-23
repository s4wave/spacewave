package provider_spacewave

import (
	"context"
	"crypto/rand"
	"path"

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
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// InitEmptyStandaloneSpace bootstraps the initial owner config/root/grant for
// an existing empty cloud shared object.
//
// Returns true when initialization wrote the initial config/root state, or
// false when the shared object was already initialized.
func (c *SessionClient) InitEmptyStandaloneSpace(
	ctx context.Context,
	le *logrus.Entry,
	accountID string,
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
	if soGrantSliceHasPeerID(state.GetRootGrants(), localPeerID) ||
		(epoch != nil && soGrantSliceHasPeerID(epoch.GetGrants(), localPeerID)) {
		return false, nil
	}

	// Initialize an unrooted shared object when needed.
	root := state.GetRoot()
	if root == nil || root.GetInnerSeqno() == 0 {
		if err := initializeCloudSharedObjectState(
			ctx,
			c,
			le,
			accountID,
			spaceID,
			c.priv,
			buildStandaloneSpaceInitStepFactorySet(),
			false,
		); err != nil {
			return false, err
		}
		return true, nil
	}

	// Reject inconsistent config history before repair.
	if len(chain.GetConfigChanges()) != 0 {
		return false, errors.New("local grant missing on initialized space with config history")
	}
	for _, keyEpoch := range chain.GetKeyEpochs() {
		if len(keyEpoch.GetGrants()) != 0 {
			return false, errors.New("local grant missing on initialized space with existing key grants")
		}
	}

	// Repair an initialized shared object missing the local grant.
	if err := repairGrantlessStandaloneSpace(
		ctx,
		c,
		le,
		accountID,
		spaceID,
		c.priv,
		state,
		chain.GetKeyEpochs(),
		buildStandaloneSpaceInitStepFactorySet(),
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

func buildInitialWorldStateData(seedWorldHead bool) ([]byte, error) {
	if !seedWorldHead {
		return nil, nil
	}
	state, err := sobject_world_engine.BuildInitialInnerState(nil)
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

	// Marshal and publish the initial root state.
	rootData, err := state.root.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal root")
	}
	if err := cli.PostInitState(ctx, sharedObjectID, rootData); err != nil {
		return err
	}
	return nil
}

type standaloneSpaceInitState struct {
	configData        []byte
	keyEpoch          *sobject.SOKeyEpoch
	recoveryEnvelopes []*sobject.SOEntityRecoveryEnvelope
	root              *sobject.SORoot
}

func buildStandaloneGenesisParticipants(
	localPeerID peer.ID,
	accountID string,
	friendAccounts []*api.FriendDmAccount,
) ([]*sobject.SOParticipantConfig, error) {
	if len(friendAccounts) == 0 {
		return []*sobject.SOParticipantConfig{{
			PeerId:   localPeerID.String(),
			Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
			EntityId: accountID,
		}}, nil
	}
	participants := make([]*sobject.SOParticipantConfig, 0)
	seenAccounts := make(map[string]struct{}, len(friendAccounts))
	seenPeers := make(map[string]struct{})
	ownerAccountFound := false
	for _, account := range friendAccounts {
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
		role := sobject.SOParticipantRole_SOParticipantRole_WRITER
		if account.GetAccountId() == accountID {
			role = sobject.SOParticipantRole_SOParticipantRole_OWNER
			ownerAccountFound = true
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
			})
		}
	}
	if len(seenAccounts) != 2 {
		return nil, errors.New("friend dm requires two accounts")
	}
	if !ownerAccountFound {
		return nil, errors.New("friend dm owner account is not present")
	}
	localPeerFound := false
	localOwnerFound := false
	for _, participant := range participants {
		if participant.GetPeerId() != localPeerID.String() {
			continue
		}
		localPeerFound = true
		if participant.GetEntityId() == accountID &&
			sobject.IsOwner(participant.GetRole()) {
			localOwnerFound = true
		}
		break
	}
	if !localPeerFound {
		return nil, errors.New("friend dm owner session is not present")
	}
	if !localOwnerFound {
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
	if state == nil || state.keyEpoch == nil || state.root == nil {
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
	rootData, err := (&api.PostRootRequest{Root: state.root}).MarshalVT()
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal friend dm root state")
	}
	return configData, rootData, nil
}

func buildStandaloneSpaceInitState(
	ctx context.Context,
	cli *SessionClient,
	le *logrus.Entry,
	accountID string,
	sharedObjectID string,
	localPriv crypto.PrivKey,
	sfs *block_transform.StepFactorySet,
	seedWorldHead bool,
	friendAccounts []*api.FriendDmAccount,
) (*standaloneSpaceInitState, error) {
	localPeerID, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		return nil, err
	}
	_, soTransform, grantInner, err := buildInitialSpaceTransform(le, sfs)
	if err != nil {
		return nil, err
	}

	participants, err := buildStandaloneGenesisParticipants(
		localPeerID,
		accountID,
		friendAccounts,
	)
	if err != nil {
		return nil, err
	}
	genesisConfig := &sobject.SharedObjectConfig{
		Participants: participants,
	}
	genesisEntry, err := sobject.BuildSOConfigChange(
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
	epoch := &sobject.SOKeyEpoch{
		Epoch:      0,
		SeqnoStart: 1,
		Grants:     grants,
	}
	genesisHash, err := sobject.HashSOConfigChange(genesisEntry)
	if err != nil {
		return nil, errors.Wrap(err, "hash signed genesis config")
	}
	genesisConfig = genesisConfig.CloneVT()
	genesisConfig.ConfigChainSeqno = genesisEntry.GetConfigSeqno()
	genesisConfig.ConfigChainHash = genesisHash
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

	stateData, err := buildInitialWorldStateData(seedWorldHead)
	if err != nil {
		return nil, err
	}
	ninner := &sobject.SORootInner{
		Seqno:     1,
		StateData: stateData,
	}
	innerDataDec, err := ninner.MarshalVT()
	if err != nil {
		return nil, err
	}
	innerDataEnc, err := soTransform.EncodeBlock(innerDataDec)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt root inner")
	}
	root := &sobject.SORoot{InnerSeqno: 1, Inner: innerDataEnc}
	if err := root.SignInnerData(localPriv, sharedObjectID, root.GetInnerSeqno(), hash.RecommendedHashType); err != nil {
		return nil, errors.Wrap(err, "sign root")
	}
	return &standaloneSpaceInitState{
		configData:        genesisData,
		keyEpoch:          epoch,
		recoveryEnvelopes: recoveryEnvelopes,
		root:              root,
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
	rootState []byte,
) (*api.CreateWithStateRequest, error) {
	if displayName == "" || objectType == "" || ownerType == "" || ownerID == "" {
		return nil, errors.New("space metadata is required")
	}
	if len(configState) == 0 || len(rootState) == 0 {
		return nil, errors.New("space initial state is required")
	}
	return &api.CreateWithStateRequest{
		DisplayName:    displayName,
		ObjectType:     objectType,
		OwnerType:      ownerType,
		OwnerId:        ownerID,
		AccountPrivate: accountPrivate,
		ConfigState:    configState,
		RootState:      rootState,
	}, nil
}

// CreateSpaceWithState atomically creates a private shared object with signed
// config and root state. A 200 response is an idempotent existing-state match;
// a 201 response creates the canonical state.
func (c *SessionClient) CreateSpaceWithState(
	ctx context.Context,
	spaceID string,
	displayName string,
	objectType string,
	ownerType string,
	ownerID string,
	accountPrivate bool,
	configState []byte,
	rootState []byte,
) error {
	if c == nil {
		return errors.New("session client is required")
	}
	if spaceID == "" {
		return errors.New("space id is required")
	}
	req, err := buildCreateWithStateRequest(
		displayName,
		objectType,
		ownerType,
		ownerID,
		accountPrivate,
		configState,
		rootState,
	)
	if err != nil {
		return err
	}
	body, err := req.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal create request")
	}
	_, err = c.doPostBinary(
		ctx,
		path.Join("/api/sobject", spaceID, "create-with-state"),
		body,
		nil,
		SeedReasonMutation,
	)
	return errors.Wrap(err, "create space with state")
}

func repairGrantlessStandaloneSpace(
	ctx context.Context,
	cli *SessionClient,
	le *logrus.Entry,
	accountID string,
	sharedObjectID string,
	localPriv crypto.PrivKey,
	state *sobject.SOState,
	epochs []*sobject.SOKeyEpoch,
	sfs *block_transform.StepFactorySet,
) error {
	root := state.GetRoot()
	if root == nil || root.GetInnerSeqno() == 0 {
		return errors.New("grantless space repair requires an initialized root")
	}
	currentSeqno := root.GetInnerSeqno()

	_, soTransform, grantInner, err := buildInitialSpaceTransform(
		le,
		sfs,
	)
	if err != nil {
		return err
	}

	localPeerID, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		return err
	}
	localPub, err := localPeerID.ExtractPublicKey()
	if err != nil {
		return errors.Wrap(err, "extract local public key")
	}
	grant, err := sobject.EncryptSOGrant(localPriv, localPub, sharedObjectID, grantInner)
	if err != nil {
		return errors.Wrap(err, "encrypt local repair grant")
	}

	nextEpoch := sobject.CurrentEpochNumber(epochs) + 1
	keyEpoch := &sobject.SOKeyEpoch{
		Epoch:      nextEpoch,
		SeqnoStart: currentSeqno + 1,
		Grants:     []*sobject.SOGrant{grant},
	}

	recoveryCfg := &sobject.SharedObjectConfig{}
	if cfg := state.GetConfig(); cfg != nil {
		recoveryCfg = cfg.CloneVT()
	}
	recoveryEnvelopes, err := buildSORecoveryEnvelopes(
		ctx,
		cli,
		sharedObjectID,
		recoveryCfg,
		keyEpoch.GetEpoch(),
		grantInner,
	)
	if err != nil {
		var missingErr *missingRecoveryKeypairsError
		if !errors.As(err, &missingErr) || missingErr.entityID != accountID {
			return errors.Wrap(err, "build recovery envelopes")
		}
		recoveryEnvelopes = nil
	}
	if err := cli.PostKeyEpoch(ctx, sharedObjectID, keyEpoch, recoveryEnvelopes); err != nil {
		return err
	}

	ninner := &sobject.SORootInner{Seqno: currentSeqno + 1}
	innerDataDec, err := ninner.MarshalVT()
	if err != nil {
		return err
	}
	innerDataEnc, err := soTransform.EncodeBlock(innerDataDec)
	if err != nil {
		return errors.Wrap(err, "encrypt repaired root inner")
	}

	nroot := &sobject.SORoot{InnerSeqno: currentSeqno + 1, Inner: innerDataEnc}
	if err := nroot.SignInnerData(localPriv, sharedObjectID, nroot.GetInnerSeqno(), hash.RecommendedHashType); err != nil {
		return errors.Wrap(err, "sign repaired root")
	}
	return cli.PostRoot(ctx, sharedObjectID, nroot, nil)
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
