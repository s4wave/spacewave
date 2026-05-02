package provider_spacewave

import (
	"context"
	"crypto/rand"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
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

	state, chain, err := c.loadStandaloneInitState(ctx, spaceID)
	if err != nil {
		return false, err
	}

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

	if len(chain.GetConfigChanges()) != 0 {
		return false, errors.New("local grant missing on initialized space with config history")
	}
	for _, keyEpoch := range chain.GetKeyEpochs() {
		if len(keyEpoch.GetGrants()) != 0 {
			return false, errors.New("local grant missing on initialized space with existing key grants")
		}
	}
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
	stateData, err := c.GetSOState(ctx, spaceID, 0, SeedReasonColdSeed)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get so state")
	}
	state, _, err := decodeSOStateResponse(stateData)
	if err != nil {
		return nil, nil, errors.Wrap(err, "decode so state")
	}
	if state == nil {
		return nil, nil, errors.New("missing so state snapshot")
	}
	chainData, err := c.GetConfigChain(ctx, spaceID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get config chain")
	}
	chain := &sobject.SOConfigChainResponse{}
	if err := chain.UnmarshalVT(chainData); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal config chain")
	}
	return state, chain, nil
}

func buildStandaloneSpaceInitStepFactorySet() *block_transform.StepFactorySet {
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_s2.NewStepFactory())
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
	localPeerID, err := peer.IDFromPrivateKey(localPriv)
	if err != nil {
		return err
	}

	_, soTransform, grantInner, err := buildInitialSpaceTransform(le, sfs)
	if err != nil {
		return err
	}

	genesisConfig := &sobject.SharedObjectConfig{
		Participants: []*sobject.SOParticipantConfig{{
			PeerId:   localPeerID.String(),
			Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
			EntityId: accountID,
		}},
	}
	genesisEntry, err := sobject.BuildSOConfigChange(
		&sobject.SharedObjectConfig{},
		genesisConfig,
		sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		localPriv,
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "build signed genesis config")
	}
	genesisData, err := genesisEntry.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal signed genesis config")
	}

	localPub, err := localPeerID.ExtractPublicKey()
	if err != nil {
		return errors.Wrap(err, "extract local public key")
	}
	grant, err := sobject.EncryptSOGrant(localPriv, localPub, sharedObjectID, grantInner)
	if err != nil {
		return errors.Wrap(err, "encrypt grant")
	}

	epoch := &sobject.SOKeyEpoch{
		Epoch:      0,
		SeqnoStart: 1,
		Grants:     []*sobject.SOGrant{grant},
	}
	genesisHash, err := sobject.HashSOConfigChange(genesisEntry)
	if err != nil {
		return errors.Wrap(err, "hash signed genesis config")
	}
	genesisConfig = genesisConfig.CloneVT()
	genesisConfig.ConfigChainSeqno = genesisEntry.GetConfigSeqno()
	genesisConfig.ConfigChainHash = genesisHash
	recoveryEnvelopes, err := buildSORecoveryEnvelopes(
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
			return errors.Wrap(err, "build recovery envelopes")
		}
		recoveryEnvelopes = nil
	}
	if err := cli.PostConfigState(
		ctx,
		sharedObjectID,
		genesisData,
		nil,
		epoch,
		recoveryEnvelopes,
	); err != nil {
		return errors.Wrap(err, "post signed genesis config")
	}

	stateData, err := buildInitialWorldStateData(seedWorldHead)
	if err != nil {
		return err
	}
	ninner := &sobject.SORootInner{
		Seqno:     1,
		StateData: stateData,
	}
	innerDataDec, err := ninner.MarshalVT()
	if err != nil {
		return err
	}
	innerDataEnc, err := soTransform.EncodeBlock(innerDataDec)
	if err != nil {
		return errors.Wrap(err, "encrypt root inner")
	}

	nroot := &sobject.SORoot{InnerSeqno: 1, Inner: innerDataEnc}
	if err := nroot.SignInnerData(localPriv, sharedObjectID, nroot.GetInnerSeqno(), hash.RecommendedHashType); err != nil {
		return errors.Wrap(err, "sign root")
	}

	rootData, err := nroot.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal root")
	}
	if err := cli.PostInitState(ctx, sharedObjectID, rootData); err != nil {
		return err
	}

	return nil
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

	recoveryCfg := state.GetConfig()
	if recoveryCfg == nil {
		recoveryCfg = &sobject.SharedObjectConfig{}
	} else {
		recoveryCfg = recoveryCfg.CloneVT()
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
			BlockEnc: hydra_blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
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
