package sobject

import (
	"bytes"
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
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

// MaxSOParticipantRole returns the higher-privilege participant role.
func MaxSOParticipantRole(a, b SOParticipantRole) SOParticipantRole {
	if b > a {
		return b
	}
	return a
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

// SOStateParticipantHandle implements SharedObjectStateSnapshot over a SOState
// as one participant.
type SOStateParticipantHandle struct {
	le             *logrus.Entry
	sfs            *block_transform.StepFactorySet
	sharedObjectID string
	state          *SOState
	privKey        crypto.PrivKey
	peerID         peer.ID
	peerIDStr      string
	// configEntry reads a retained config change by hash, or is nil.
	configEntry func(context.Context, []byte) (*SOConfigChange, error)

	// mtx guards the fields below.
	mtx sync.Mutex
	// transformers caches the transformer of each decoded key epoch.
	transformers map[uint64]*block_transform.Transformer
	// opSet caches the verified operation set, or is nil.
	opSet *SOOperationSet
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
		transformers:   make(map[uint64]*block_transform.Transformer),
	}
}

// WithConfigHistory resolves configs other than the current one through read,
// which returns the retained config change with a hash, or nil. Call it before
// sharing the handle.
func (s *SOStateParticipantHandle) WithConfigHistory(read func(context.Context, []byte) (*SOConfigChange, error)) *SOStateParticipantHandle {
	s.configEntry = read
	return s
}

// GetParticipantConfig returns the participant record for our participant.
// uses the peer identity from the SharedObject.
// returns ErrNotParticipant if the local peer is not a participant.
func (s *SOStateParticipantHandle) GetParticipantConfig(ctx context.Context) (*SOParticipantConfig, error) {
	return s.GetParticipantConfigForPeer(ctx, s.peerIDStr)
}

// GetParticipantConfigForPeer returns a participant from this accepted config.
func (s *SOStateParticipantHandle) GetParticipantConfigForPeer(_ context.Context, peerID string) (*SOParticipantConfig, error) {
	for _, participant := range s.state.GetConfig().GetParticipants() {
		if participant.GetPeerId() == peerID {
			return participant, nil
		}
	}
	return nil, ErrNotParticipant
}

// GetConfig returns the accepted config.
func (s *SOStateParticipantHandle) GetConfig(context.Context) (*SharedObjectConfig, error) {
	return s.state.GetConfig(), nil
}

// GetConfigByHash returns the current config or a retained earlier one.
func (s *SOStateParticipantHandle) GetConfigByHash(ctx context.Context, hash []byte) (*SharedObjectConfig, error) {
	// The current config needs no history.
	current := s.state.GetConfig()
	if bytes.Equal(current.GetConfigChainHash(), hash) {
		return current, nil
	}

	// Read the retained change that produced the config.
	if s.configEntry == nil || len(hash) == 0 {
		return nil, ErrConfigHistoryUnavailable
	}
	entry, err := s.configEntry(ctx, hash)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, ErrConfigHistoryUnavailable
	}

	// The change must hash to the requested config.
	entryHash, err := HashSOConfigChange(entry)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(entryHash, hash) {
		return nil, ErrConfigHistoryUnavailable
	}
	cfg := entry.GetConfig().CloneVT()
	cfg.ConfigChainHash = hash
	return cfg, nil
}

// GetCheckpoint returns the checkpoint body with its state data decoded.
func (s *SOStateParticipantHandle) GetCheckpoint(ctx context.Context) (*SOCheckpointInner, error) {
	// A state without a checkpoint has no readable state.
	inner, err := s.state.GetCheckpointInner()
	if err != nil || inner == nil {
		return nil, err
	}
	return s.DecodeCheckpoint(inner)
}

// DecodeCheckpoint returns a copy of a checkpoint body with its state data
// decoded with the key of its epoch.
func (s *SOStateParticipantHandle) DecodeCheckpoint(inner *SOCheckpointInner) (*SOCheckpointInner, error) {
	inner = inner.CloneVT()
	if len(inner.GetStateData()) != 0 {
		xfrm, err := s.epochTransformer(inner.GetKeyEpoch())
		if err != nil {
			return nil, err
		}
		inner.StateData, err = xfrm.DecodeBlock(inner.GetStateData())
		if err != nil {
			return nil, errors.Wrap(err, "decode checkpoint state")
		}
	}
	return inner, nil
}

// GetOperationSet returns the verified operations above the checkpoint. The
// handle verifies them once and shares the set, which callers must not modify.
func (s *SOStateParticipantHandle) GetOperationSet(context.Context) (*SOOperationSet, error) {
	// Verify the operations once, under the lock.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.opSet != nil {
		return s.opSet, nil
	}
	set, err := s.state.OperationSet(s.sharedObjectID)
	if err != nil {
		return nil, err
	}
	s.opSet = set
	return set, nil
}

// DecodeOperation decodes the operation data with the key of its epoch. An
// acknowledgment decodes to no data.
func (s *SOStateParticipantHandle) DecodeOperation(_ context.Context, inner *SOOperationInner) ([]byte, error) {
	if inner.IsAcknowledgment() {
		return nil, nil
	}
	xfrm, err := s.epochTransformer(inner.GetKeyEpoch())
	if err != nil {
		return nil, err
	}
	return xfrm.DecodeBlock(inner.GetOpData())
}

// GetTransformer returns the transformer of the current key epoch.
func (s *SOStateParticipantHandle) GetTransformer(context.Context) (*block_transform.Transformer, error) {
	current := s.state.CurrentKeyEpoch()
	if current == nil {
		return nil, ErrCannotDecode
	}
	return s.epochTransformer(current.GetEpoch())
}

// epochTransformer returns the transformer of one key epoch from the local
// peer's grant.
func (s *SOStateParticipantHandle) epochTransformer(epoch uint64) (*block_transform.Transformer, error) {
	// Reuse a transformer already built for the epoch.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if xfrm, ok := s.transformers[epoch]; ok {
		return xfrm, nil
	}

	// Decrypt the local peer's grant for the epoch.
	conf, err := s.epochTransformConf(epoch)
	if err != nil {
		return nil, err
	}
	if err := conf.Validate(); err != nil {
		return nil, NewSharedObjectHealthError(NewSharedObjectClosedHealth(
			SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
			SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_TRANSFORM_CONFIG_DECODE_FAILED,
			SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
			err.Error(),
		), err)
	}

	// Build and cache its transformer.
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: s.le}, s.sfs, conf)
	if err != nil {
		return nil, err
	}
	s.transformers[epoch] = xfrm
	return xfrm, nil
}

// epochTransformConf decrypts the local peer's grant for one key epoch.
func (s *SOStateParticipantHandle) epochTransformConf(epoch uint64) (*block_transform.Config, error) {
	// Decrypt the local peer's grant in the epoch.
	grant := s.localGrant(s.state.GetKeyEpoch(epoch))
	if grant == nil {
		return nil, errors.Wrapf(ErrKeyEpochUnavailable, "epoch %d", epoch)
	}
	inner, err := grant.DecryptInnerData(s.privKey, s.sharedObjectID)
	if err != nil {
		return nil, errors.Wrap(err, "so grant: decode inner data")
	}
	return inner.GetTransformConf(), nil
}

// localGrant returns the local peer's grant in epoch, or nil.
func (s *SOStateParticipantHandle) localGrant(epoch *SOKeyEpoch) *SOGrant {
	for _, grant := range epoch.GetGrants() {
		if grant.GetPeerId() == s.peerIDStr {
			return grant
		}
	}
	return nil
}

// GetTransformInfo implements SharedObjectStateSnapshot.GetTransformInfo.
func (s *SOStateParticipantHandle) GetTransformInfo(context.Context) (*TransformInfo, error) {
	// Count the current epoch's grants.
	current := s.state.CurrentKeyEpoch()
	info := &TransformInfo{
		GrantCount: uint32(len(current.GetGrants())), //nolint:gosec // grants is the bounded in-memory grant set.
	}

	// Redact the steps of the local peer's grant.
	grant := s.localGrant(current)
	if grant == nil {
		return info, nil
	}
	inner, err := grant.DecryptInnerData(s.privKey, s.sharedObjectID)
	if err != nil {
		return info, nil
	}
	info.Steps = RedactStepConfigs(inner.GetTransformConf().GetSteps())
	return info, nil
}

// _ is a type assertion
var _ SharedObjectStateSnapshot = (*SOStateParticipantHandle)(nil)
