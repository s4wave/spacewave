package sobject

import (
	"bytes"
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
	"github.com/sirupsen/logrus"
)

// ValidateSOParticipantRole ensures the enum value is within the expected set.
// If allowUnknown is true, it will allow SOParticipantRole_UNKNOWN as a valid role.
func ValidateSOParticipantRole(role SOParticipantRole, allowUnknown bool) error {
	switch role {
	case SOParticipantRole_SOParticipantRole_READER,
		SOParticipantRole_SOParticipantRole_WRITER,
		SOParticipantRole_SOParticipantRole_VALIDATOR,
		SOParticipantRole_SOParticipantRole_OWNER:
		return nil
	case SOParticipantRole_SOParticipantRole_UNKNOWN:
		if allowUnknown {
			return nil
		}
		fallthrough
	default:
		return ErrInvalidSOParticipantRole
	}
}

// Validate performs cursory checks on the SOParticipant.
func (p *SOParticipantConfig) Validate() error {
	if _, err := p.ParsePeerID(); err != nil {
		return err
	}
	if err := ValidateSOParticipantRole(p.GetRole(), false); err != nil {
		return err
	}
	return nil
}

// ParsePeerID parses the participant peer ID.
func (p *SOParticipantConfig) ParsePeerID() (peer.ID, error) {
	if len(p.GetPeerId()) == 0 {
		return "", peer.ErrEmptyPeerID
	}
	return confparse.ParsePeerID(p.GetPeerId())
}

// CanReadState checks if the given role has read access to the state.
func CanReadState(role SOParticipantRole) bool {
	switch role {
	case SOParticipantRole_SOParticipantRole_READER,
		SOParticipantRole_SOParticipantRole_WRITER,
		SOParticipantRole_SOParticipantRole_VALIDATOR,
		SOParticipantRole_SOParticipantRole_OWNER:
		return true
	default:
		return false
	}
}

// CanWriteOps checks if the given role has access to write ops.
func CanWriteOps(role SOParticipantRole) bool {
	switch role {
	case SOParticipantRole_SOParticipantRole_WRITER,
		SOParticipantRole_SOParticipantRole_VALIDATOR,
		SOParticipantRole_SOParticipantRole_OWNER:
		return true
	default:
		return false
	}
}

// IsOwner checks if the given role is the OWNER role.
func IsOwner(role SOParticipantRole) bool {
	return role == SOParticipantRole_SOParticipantRole_OWNER
}

// IsValidatorOrOwner checks if the given role is VALIDATOR or OWNER.
func IsValidatorOrOwner(role SOParticipantRole) bool {
	switch role {
	case SOParticipantRole_SOParticipantRole_VALIDATOR,
		SOParticipantRole_SOParticipantRole_OWNER:
		return true
	default:
		return false
	}
}

// SOStateParticipantHandle implements SharedObjectStateSnapshot backed by a SOState plus a few extra functions.
type SOStateParticipantHandle struct {
	le             *logrus.Entry
	sfs            *block_transform.StepFactorySet
	sharedObjectID string
	state          *SOState
	privKey        crypto.PrivKey
	peerID         peer.ID
	peerIDStr      string
}

// NewSOStateParticipantHandle constructs a SOStateParticipantHandle from a SOState and private key.
func NewSOStateParticipantHandle(
	le *logrus.Entry,
	sfs *block_transform.StepFactorySet,
	sharedObjectID string,
	state *SOState,
	privKey crypto.PrivKey,
	localPeerID peer.ID,
) *SOStateParticipantHandle {
	return &SOStateParticipantHandle{
		le:             le,
		sfs:            sfs,
		sharedObjectID: sharedObjectID,
		state:          state,
		privKey:        privKey,
		peerID:         localPeerID,
		peerIDStr:      localPeerID.String(),
	}
}

// GetParticipantConfig returns the participant record for our participant.
// uses the peer identity from the SharedObject.
// returns ErrNotParticipant if the local peer is not a participant.
func (s *SOStateParticipantHandle) GetParticipantConfig(ctx context.Context) (*SOParticipantConfig, error) {
	for _, participantConfig := range s.state.GetConfig().GetParticipants() {
		if participantConfig.GetPeerId() == s.peerIDStr {
			return participantConfig, nil
		}
	}

	return nil, ErrNotParticipant
}

// GetOpQueue returns the operation queue for our participant.
// uses the peer identity from the SharedObject.
func (s *SOStateParticipantHandle) GetOpQueue(ctx context.Context) ([]*SOOperation, []*QueuedSOOperation, error) {
	ops := s.state.GetOps()
	var opq []*SOOperation
	for _, op := range ops {
		pubKey, err := op.GetSignature().ParsePubKey()
		if err != nil {
			return nil, nil, err
		}

		peerID, err := peer.IDFromPublicKey(pubKey)
		if err != nil {
			return nil, nil, err
		}

		if peerID.String() == s.peerIDStr {
			opq = append(opq, op)
		}
	}

	return opq, nil, nil
}

// GetRootInner attempts to decode the current SORootInner and return it.
// uses the peer identity from the SharedObject to decode.
func (s *SOStateParticipantHandle) GetRootInner(ctx context.Context) (*SORootInner, error) {
	// blank state
	stateRoot := s.state.GetRoot()
	if stateRoot.GetInnerSeqno() == 0 {
		return nil, nil
	}

	rootInnerData := bytes.Clone(stateRoot.GetInner())
	if len(rootInnerData) == 0 {
		return nil, ErrEmptyInnerData
	}
	defer scrub.Scrub(rootInnerData)

	grants := s.state.GetRootGrants()
	localGrantIdx := slices.IndexFunc(grants, func(g *SOGrant) bool {
		return g.GetPeerId() == s.peerIDStr
	})
	if localGrantIdx == -1 {
		return nil, ErrCannotDecode
	}

	localGrant := grants[localGrantIdx]
	innerDataObj, err := localGrant.DecryptInnerData(s.privKey, s.sharedObjectID)
	if err != nil {
		return nil, errors.Wrap(err, "so grant: decode inner data")
	}

	transformConf := innerDataObj.GetTransformConf()
	if err := transformConf.Validate(); err != nil {
		return nil, errors.Wrap(err, "so grant: validate transform config")
	}

	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: s.le}, s.sfs, transformConf)
	if err != nil {
		return nil, err
	}

	rootInnerDataDec, err := xfrm.DecodeBlock(rootInnerData)
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(rootInnerDataDec)

	rootInnerObj := &SORootInner{}
	if err := rootInnerObj.UnmarshalVT(rootInnerDataDec); err != nil {
		return nil, err
	}
	if rootInnerObj.GetSeqno() != stateRoot.GetInnerSeqno() {
		return nil, errors.Wrapf(
			ErrInvalidSeqno,
			"root had %d but inner had %d",
			s.state.GetRoot().GetInnerSeqno(),
			rootInnerObj.GetSeqno(),
		)
	}

	return rootInnerObj, nil
}

// ProcessOperations implements SharedObjectStateSnapshot.ProcessOperations
func (s *SOStateParticipantHandle) ProcessOperations(
	ctx context.Context,
	ops []*SOOperation,
	cb SnapshotProcessOpsFunc,
) (*SORoot, []*SOOperationRejection, []*SOOperation, error) {
	// Check if we are a validator
	participantConfig, err := s.GetParticipantConfig(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	if !IsValidatorOrOwner(participantConfig.GetRole()) {
		return nil, nil, nil, errors.New("local peer is not a validator or owner")
	}

	// Get transformer for decoding op data
	xfrm, err := s.GetTransformer(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	// Decode inner operations
	innerOps := make([]*SOOperationInner, 0, len(ops))
	var rejectedOps []*SOOperationRejection
	var acceptedOps []*SOOperation

	// Process each operation
	for _, op := range ops {
		inner, err := op.UnmarshalInner()
		if err == nil {
			err = inner.Validate()
		}
		if err != nil {
			return nil, nil, nil, err
		}

		innerPeerID, err := inner.ParsePeerID()
		if err != nil {
			return nil, nil, nil, err
		}

		// Decode operation data if present
		if len(inner.GetOpData()) > 0 {
			opDataDec, err := xfrm.DecodeBlock(inner.GetOpData())
			if err != nil {
				// Build rejection for failed decode / validate
				rejection, rerr := BuildSOOperationRejection(
					s.privKey,
					s.sharedObjectID,
					innerPeerID,
					inner.GetNonce(),
					inner.GetLocalId(),
					&SOOperationRejectionErrorDetails{
						ErrorMsg: errors.Wrap(err, "failed to decode operation data").Error(),
					},
				)
				if rerr != nil {
					return nil, nil, nil, rerr
				}
				rejectedOps = append(rejectedOps, rejection)
				continue
			}
			inner.OpData = opDataDec
		}
		innerOps = append(innerOps, inner)
	}

	// Get current root inner state to access current state data
	rootInner, err := s.GetRootInner(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	// Get current state data - may be nil if initial state
	var currentStateData []byte
	if rootInner != nil {
		currentStateData = rootInner.GetStateData()
	}
	currentRoot := s.state.GetRoot()

	// Call callback with current state data and valid operations
	rawNextStateData, opResults, err := cb(ctx, currentStateData, innerOps)
	if err != nil {
		return nil, nil, nil, err
	}

	// If nothing happened, return early
	if rawNextStateData == nil && len(opResults) == 0 {
		return currentRoot.CloneVT(), rejectedOps, nil, nil
	}

	// If no state changes, use current state
	if rawNextStateData == nil {
		rawNextStateData = &currentStateData
	}

	// Build next root state
	nextInner := &SORootInner{
		Seqno:     s.state.GetRoot().GetInnerSeqno() + 1,
		StateData: *rawNextStateData,
	}

	// Marshal and encode the inner state
	innerData, err := nextInner.MarshalVT()
	if err != nil {
		return nil, nil, nil, err
	}

	encodedInnerData, err := xfrm.EncodeBlock(innerData)
	if err != nil {
		return nil, nil, nil, err
	}

	// Build root state
	nextRoot := currentRoot.CloneVT()
	if nextRoot == nil {
		nextRoot = &SORoot{}
	}
	nextRoot.Inner = encodedInnerData
	nextRoot.InnerSeqno = nextInner.GetSeqno()
	nextRoot.ValidatorSignatures = nil

	// Process operation results
	for _, result := range opResults {
		opRef := result.GetOpRef()
		if opRef == nil {
			continue
		}

		// Validate the operation reference
		if err := opRef.Validate(); err != nil {
			return nil, nil, nil, errors.Wrap(err, "invalid operation reference")
		}

		// Parse the peer ID
		submitterPeerID, err := opRef.ParsePeerID()
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to parse submitter peer ID")
		}

		// Find matching operation
		var matchOp *SOOperation
		var matchLocalID string
		for _, op := range ops {
			inner, err := op.UnmarshalInner()
			if err != nil {
				return nil, nil, nil, err
			}
			if inner.GetPeerId() == opRef.GetPeerId() && inner.GetNonce() == opRef.GetNonce() {
				matchOp = op
				matchLocalID = inner.GetLocalId()
				break
			}
		}
		if matchOp == nil {
			continue
		}

		switch body := result.GetBody().(type) {
		case *SOOperationResult_ErrorDetails:
			rejection, err := BuildSOOperationRejection(
				s.privKey,
				s.sharedObjectID,
				submitterPeerID,
				opRef.GetNonce(),
				matchLocalID,
				body.ErrorDetails,
			)
			if err != nil {
				return nil, nil, nil, err
			}
			rejectedOps = append(rejectedOps, rejection)
		default:
			acceptedOps = append(acceptedOps, matchOp)
			nextRoot.updateAccountNonce(opRef.GetPeerId(), opRef.GetNonce())
		}
	}

	// Sign the root state
	if err := nextRoot.SignInnerData(
		s.privKey,
		s.sharedObjectID,
		nextRoot.GetInnerSeqno(),
		hash.RecommendedHashType,
	); err != nil {
		return nil, nil, nil, err
	}

	return nextRoot, rejectedOps, acceptedOps, nil
}

// GetTransformer implements SharedObjectStateSnapshot.GetTransformer
func (s *SOStateParticipantHandle) GetTransformer(ctx context.Context) (*block_transform.Transformer, error) {
	grants := s.state.GetRootGrants()
	localGrantIdx := slices.IndexFunc(grants, func(g *SOGrant) bool {
		return g.GetPeerId() == s.peerIDStr
	})
	if localGrantIdx == -1 {
		return nil, ErrCannotDecode
	}

	localGrant := grants[localGrantIdx]
	innerDataObj, err := localGrant.DecryptInnerData(s.privKey, s.sharedObjectID)
	if err != nil {
		return nil, errors.Wrap(err, "so grant: decode inner data")
	}

	transformConf := innerDataObj.GetTransformConf()
	if err := transformConf.Validate(); err != nil {
		return nil, errors.Wrap(err, "so grant: validate transform config")
	}

	return block_transform.NewTransformer(controller.ConstructOpts{Logger: s.le}, s.sfs, transformConf)
}

// GetTransformInfo implements SharedObjectStateSnapshot.GetTransformInfo.
func (s *SOStateParticipantHandle) GetTransformInfo(ctx context.Context) (*TransformInfo, error) {
	grants := s.state.GetRootGrants()
	info := &TransformInfo{
		GrantCount: uint32(len(grants)),
	}

	// Find and decrypt local grant to extract transform steps.
	localGrantIdx := slices.IndexFunc(grants, func(g *SOGrant) bool {
		return g.GetPeerId() == s.peerIDStr
	})
	if localGrantIdx == -1 {
		return info, nil
	}

	localGrant := grants[localGrantIdx]
	innerDataObj, err := localGrant.DecryptInnerData(s.privKey, s.sharedObjectID)
	if err != nil {
		return info, nil
	}

	transformConf := innerDataObj.GetTransformConf()
	info.Steps = RedactStepConfigs(transformConf.GetSteps())
	return info, nil
}

// GetOpRejections returns any operation rejections for our participant along with their decoded error details.
// uses the peer identity from the SharedObject.
// The error details slice corresponds 1:1 with the rejections slice.
// If a rejection has no error details, the corresponding entry will be nil.
func (s *SOStateParticipantHandle) GetOpRejections(ctx context.Context) ([]*SOOperationRejection, []*SOOperationRejectionErrorDetails, error) {
	// Find rejections for our peer ID
	for _, peerRejections := range s.state.GetOpRejections() {
		if peerRejections.GetPeerId() == s.peerIDStr {
			rejections := peerRejections.GetRejections()
			if len(rejections) == 0 {
				return nil, nil, nil
			}

			// Decode error details for each rejection
			errorDetails := make([]*SOOperationRejectionErrorDetails, len(rejections))
			for i, rejection := range rejections {
				// Get validator peer ID from signature
				validatorPubKey, err := rejection.GetSignature().ParsePubKey()
				if err != nil {
					return nil, nil, err
				}
				validatorPeerID, err := peer.IDFromPublicKey(validatorPubKey)
				if err != nil {
					return nil, nil, err
				}

				// Unmarshal inner data
				inner, err := rejection.UnmarshalInner()
				if err != nil {
					return nil, nil, err
				}

				// Decode error details if present
				details, err := inner.DecodeErrorDetails(s.privKey, s.sharedObjectID, validatorPeerID)
				if err != nil {
					return nil, nil, errors.Wrapf(err, "failed to decode error details for rejection %d", i)
				}
				errorDetails[i] = details
			}
			return rejections, errorDetails, nil
		}
	}
	return nil, nil, nil
}

// _ is a type assertion
var _ SharedObjectStateSnapshot = (*SOStateParticipantHandle)(nil)
