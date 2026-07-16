package sobject

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// ReencryptSOState decrypts source root state with sourcePrivKey and returns a
// clean destination state encrypted under fresh transform material. The
// destination signing key must belong to a readable validator or owner in
// destinationParticipants; grants are created for every readable participant.
// Only the decoded root state data is carried across the destination boundary.
func ReencryptSOState(
	ctx context.Context,
	le *logrus.Entry,
	sfs *block_transform.StepFactorySet,
	sourceSharedObjectID string,
	sourceState *SOState,
	sourcePrivKey crypto.PrivKey,
	destinationSharedObjectID string,
	destinationPrivKey crypto.PrivKey,
	destinationParticipants []*SOParticipantConfig,
) (*SOState, error) {
	if sourceSharedObjectID == "" || destinationSharedObjectID == "" {
		return nil, ErrEmptySharedObjectID
	}
	if sfs == nil {
		return nil, errors.New("step factory set is required")
	}
	if sourceState == nil {
		return nil, errors.New("source state is required")
	}
	if sourcePrivKey == nil {
		return nil, errors.New("source private key is required")
	}
	if destinationPrivKey == nil {
		return nil, errors.New("destination private key is required")
	}
	if len(destinationParticipants) == 0 {
		return nil, ErrEmptyParticipants
	}

	participants := make([]*SOParticipantConfig, len(destinationParticipants))
	for i, participant := range destinationParticipants {
		if participant == nil {
			return nil, errors.Errorf("destination participant[%d] is required", i)
		}
		if err := participant.Validate(); err != nil {
			return nil, errors.Wrapf(err, "destination participant[%d]", i)
		}
		participants[i] = participant.CloneVT()
	}

	sourceConfig := sourceState.GetConfig()
	if sourceConfig == nil {
		return nil, errors.New("source config is required")
	}
	if err := sourceConfig.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate source config")
	}
	sourceRoot := sourceState.GetRoot()
	if sourceRoot == nil {
		return nil, errors.New("source root is required")
	}
	if sourceRoot.GetInnerSeqno() == 0 {
		return nil, errors.Wrap(ErrInvalidSeqno, "source root has no readable state")
	}
	if err := sourceRoot.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate source root")
	}
	validSigs, err := sourceRoot.ValidateSignatures(
		sourceSharedObjectID,
		sourceConfig.GetParticipants(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "validate source root signatures")
	}
	if err := CheckConsensusAcceptance(sourceConfig.GetConsensusMode(), validSigs); err != nil {
		return nil, errors.Wrap(err, "validate source root consensus")
	}
	seenGrantPeerIDs := make(map[string]struct{}, len(sourceState.GetRootGrants()))
	for i, grant := range sourceState.GetRootGrants() {
		if grant == nil {
			return nil, errors.Errorf("source root grant[%d] is required", i)
		}
		if err := grant.Validate(); err != nil {
			return nil, errors.Wrapf(err, "validate source root grant[%d]", i)
		}
		grantPeerID := grant.GetPeerId()
		if _, ok := seenGrantPeerIDs[grantPeerID]; ok {
			return nil, errors.Errorf("source root grant[%d]: duplicate peer id %s", i, grantPeerID)
		}
		seenGrantPeerIDs[grantPeerID] = struct{}{}
		var grantParticipant *SOParticipantConfig
		for _, participant := range sourceConfig.GetParticipants() {
			if participant.GetPeerId() == grantPeerID {
				grantParticipant = participant
				break
			}
		}
		if grantParticipant == nil || !CanReadState(grantParticipant.GetRole()) {
			return nil, errors.Errorf("source root grant[%d]: peer %s has no read access", i, grantPeerID)
		}
		if err := grant.ValidateSignature(sourceSharedObjectID, sourceConfig.GetParticipants()); err != nil {
			return nil, errors.Wrapf(err, "validate source root grant[%d] signature", i)
		}
	}

	sourcePeerID, err := peer.IDFromPrivateKey(sourcePrivKey)
	if err != nil {
		return nil, errors.Wrap(err, "source peer id")
	}
	sourceHandle := NewSOStateParticipantHandle(
		le,
		sfs,
		sourceSharedObjectID,
		sourceState,
		sourcePrivKey,
		sourcePeerID,
	)
	sourceParticipant, err := sourceHandle.GetParticipantConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "source participant")
	}
	if !CanReadState(sourceParticipant.GetRole()) {
		return nil, errors.New("source peer does not have read access")
	}

	rootInner, err := sourceHandle.GetRootInner(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt source root inner")
	}
	if rootInner == nil {
		return nil, errors.New("source root inner is required")
	}
	stateData := rootInner.GetStateData()
	defer scrub.Scrub(stateData)
	if err := rootInner.Validate(); err != nil {
		return nil, errors.Wrap(err, "validate source root inner")
	}

	destinationPeerID, err := peer.IDFromPrivateKey(destinationPrivKey)
	if err != nil {
		return nil, errors.Wrap(err, "destination peer id")
	}
	var destinationParticipant *SOParticipantConfig
	for _, participant := range participants {
		if participant.GetPeerId() == destinationPeerID.String() {
			destinationParticipant = participant
			break
		}
	}
	if destinationParticipant == nil || !CanReadState(destinationParticipant.GetRole()) {
		return nil, errors.Wrap(ErrNotParticipant, "destination signing peer must retain read access")
	}
	if !IsValidatorOrOwner(destinationParticipant.GetRole()) {
		return nil, errors.New("destination signing peer must be a validator or owner")
	}

	transformConf, grants, _, err := RotateTransformKey(
		destinationPrivKey,
		destinationSharedObjectID,
		participants,
		0,
		0,
	)
	if err != nil {
		return nil, errors.Wrap(err, "generate destination transform material")
	}
	transform, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: le},
		sfs,
		transformConf,
	)
	if err != nil {
		return nil, errors.Wrap(err, "build destination transformer")
	}

	newRootInner := &SORootInner{
		Seqno:     1,
		StateData: stateData,
	}
	innerData, err := newRootInner.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal destination root inner")
	}
	defer scrub.Scrub(innerData)
	innerDataEnc, err := transform.EncodeBlock(innerData)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt destination root inner")
	}

	newRoot := &SORoot{
		Inner:      innerDataEnc,
		InnerSeqno: 1,
	}
	if err := newRoot.SignInnerData(
		destinationPrivKey,
		destinationSharedObjectID,
		1,
		hash.RecommendedHashType,
	); err != nil {
		return nil, errors.Wrap(err, "sign destination root")
	}

	newState := &SOState{
		Config:     &SharedObjectConfig{Participants: participants},
		Root:       newRoot,
		RootGrants: grants,
	}
	if err := newState.Validate(destinationSharedObjectID); err != nil {
		return nil, errors.Wrap(err, "validate re-encrypted state")
	}
	return newState, nil
}
