package provider_transfer

import (
	"context"
	"crypto/rand"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// SOStateRewriter rewrites an SO state during transfer.
// Used to re-key state from source peer to target peer.
type SOStateRewriter func(ctx context.Context, soID string, state *sobject.SOState) (*sobject.SOState, error)

// RekeySOState re-keys an SO state from the source peer to the target peer.
// Decrypts the root inner using the source key, re-encrypts with a fresh key
// for the target peer, and re-signs the root.
func RekeySOState(
	ctx context.Context,
	le *logrus.Entry,
	sfs *block_transform.StepFactorySet,
	sourceState *sobject.SOState,
	sourcePrivKey crypto.PrivKey,
	targetPrivKey crypto.PrivKey,
	sharedObjectID string,
) (*sobject.SOState, error) {
	// Decrypt the root inner using the source key.
	sourcePeerID, err := peer.IDFromPrivateKey(sourcePrivKey)
	if err != nil {
		return nil, errors.Wrap(err, "source peer id")
	}
	sourceHandle := sobject.NewSOStateParticipantHandle(
		le, sfs, sharedObjectID, sourceState, sourcePrivKey, sourcePeerID,
	)
	rootInner, err := sourceHandle.GetRootInner(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt source root inner")
	}

	// Get target peer identity.
	targetPeerID, err := peer.IDFromPrivateKey(targetPrivKey)
	if err != nil {
		return nil, errors.Wrap(err, "target peer id")
	}
	targetPeerIDStr := targetPeerID.String()

	// Build new participants config with the target peer.
	newConfig := &sobject.SharedObjectConfig{
		Participants: []*sobject.SOParticipantConfig{{
			PeerId: targetPeerIDStr,
			Role:   sobject.SOParticipantRole_SOParticipantRole_VALIDATOR,
		}},
	}

	// Generate a fresh encryption key.
	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		return nil, err
	}

	soTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "build transform config")
	}

	soTransform, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le}, sfs, soTransformConf,
	)
	if err != nil {
		return nil, errors.Wrap(err, "build transformer")
	}

	// Preserve the seqno and state data from the source root inner.
	newSeqno := uint64(1)
	var stateData []byte
	if rootInner != nil {
		newSeqno = rootInner.GetSeqno()
		stateData = rootInner.GetStateData()
	}

	newRootInner := &sobject.SORootInner{
		Seqno:     newSeqno,
		StateData: stateData,
	}
	innerData, err := newRootInner.MarshalVT()
	if err != nil {
		return nil, err
	}
	innerDataEnc, err := soTransform.EncodeBlock(innerData)
	if err != nil {
		return nil, errors.Wrap(err, "encode root inner")
	}

	// Create and sign the new root with the target key.
	newRoot := &sobject.SORoot{
		Inner:      innerDataEnc,
		InnerSeqno: newSeqno,
	}
	if err := newRoot.SignInnerData(
		targetPrivKey, sharedObjectID, newSeqno, hash.RecommendedHashType,
	); err != nil {
		return nil, errors.Wrap(err, "sign root")
	}

	// Create a grant encrypted to the target peer.
	targetPub := targetPrivKey.GetPublic()
	nextGrantInner := &sobject.SOGrantInner{TransformConf: soTransformConf}
	grant, err := sobject.EncryptSOGrant(
		targetPrivKey, targetPub, sharedObjectID, nextGrantInner,
	)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt grant")
	}

	newState := &sobject.SOState{
		Config:     newConfig,
		Root:       newRoot,
		RootGrants: []*sobject.SOGrant{grant},
	}
	if err := newState.Validate(sharedObjectID); err != nil {
		return nil, errors.Wrap(err, "validate re-keyed state")
	}

	return newState, nil
}
