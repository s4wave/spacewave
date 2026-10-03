package sobject

import (
	"crypto/rand"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// BuildGenesisSOState returns the state of a new shared object owned solely by
// privKey, with the signed genesis config change its config history starts
// from. The checkpoint holds stateData encrypted with a fresh key of epoch 0,
// granted to the owner.
func BuildGenesisSOState(
	le *logrus.Entry,
	sfs *block_transform.StepFactorySet,
	sharedObjectID string,
	privKey crypto.PrivKey,
	stateData []byte,
) (*SOState, *SOConfigChange, error) {
	// Sign the genesis config naming the owner.
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, nil, err
	}
	initial := &SharedObjectConfig{
		Participants: []*SOParticipantConfig{{
			PeerId: peerID.String(),
			Role:   SOParticipantRole_SOParticipantRole_OWNER,
		}},
	}
	genesis, err := BuildSOConfigChange(sharedObjectID, initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, privKey, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build genesis config")
	}
	cfg, err := VerifyConfigChange(sharedObjectID, initial, genesis)
	if err != nil {
		return nil, nil, err
	}

	// Grant a fresh key of epoch 0 to the owner.
	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		return nil, nil, errors.Wrap(err, "generate encryption key")
	}
	defer scrub.Scrub(encKey)
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.DefaultBlockEnc,
			Key:      encKey,
		},
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "build transform config")
	}
	grant, err := EncryptSOGrant(privKey, privKey.GetPublic(), sharedObjectID, &SOGrantInner{TransformConf: transformConf})
	if err != nil {
		return nil, nil, errors.Wrap(err, "encrypt genesis grant")
	}

	// Sign the checkpoint holding the encrypted initial state.
	var stateDataEnc []byte
	if len(stateData) != 0 {
		xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, sfs, transformConf)
		if err != nil {
			return nil, nil, errors.Wrap(err, "build transformer")
		}
		stateDataEnc, err = xfrm.EncodeBlock(stateData)
		if err != nil {
			return nil, nil, errors.Wrap(err, "encrypt initial state")
		}
	}
	checkpoint, err := BuildGenesisSOCheckpoint(privKey, sharedObjectID, cfg.GetConfigChainHash(), stateDataEnc)
	if err != nil {
		return nil, nil, err
	}

	// Assemble and validate the genesis state.
	state := &SOState{
		Config:     cfg,
		Checkpoint: checkpoint,
		KeyEpochs:  []*SOKeyEpoch{{Grants: []*SOGrant{grant}}},
	}
	if err := state.Validate(sharedObjectID); err != nil {
		return nil, nil, err
	}
	return state, genesis, nil
}
