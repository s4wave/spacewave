package cdn_sharedobject

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/peer"
)

// CdnDisplayName is the human-readable name surfaced for the CDN Space mount.
const CdnDisplayName = "Spacewave CDN"

// ErrCdnReadOnly is returned from any CdnSharedObject write path.
var ErrCdnReadOnly = errors.New("cdn shared object is read-only")

// CdnSharedObject is a read-only sobject.SharedObject backed by the anonymous
// CDN block store. It exposes the checkpoint from the cached CdnRootPointer so
// callers can build a WorldState against the CDN's world without going
// through the normal SO-world-engine controller path.
type CdnSharedObject struct {
	spaceID string
	bus     bus.Bus
	peerID  peer.ID
	bs      cdn_bstore.RootBlockStore

	meta   *sobject.SharedObjectMeta
	snap   *cdnStateSnapshot
	watch  *ccontainer.CContainer[sobject.SharedObjectStateSnapshot]
	health *ccontainer.CContainer[*sobject.SharedObjectHealth]
}

// CdnSharedObjectOptions configure a CdnSharedObject.
type CdnSharedObjectOptions struct {
	// SpaceID is the CDN Space ULID this SharedObject represents.
	SpaceID string
	// Bus is the controllerbus used by session consumers of GetBus().
	Bus bus.Bus
	// PeerID is the local peer id. An empty value is allowed for anonymous
	// mounts on local-only bootstraps where no session identity exists yet.
	PeerID peer.ID
	// BlockStore reads the CDN Space and tracks its root pointer. It is
	// either the transport-owning CdnBlockStore or a SuppliedBlockStore reading
	// through a process that already owns that transport.
	BlockStore cdn_bstore.RootBlockStore
}

// NewCdnSharedObject constructs a new CdnSharedObject. The caller is expected
// to refresh the block store pointer before the first read so GetCheckpoint
// returns the current published checkpoint.
func NewCdnSharedObject(opts CdnSharedObjectOptions) (*CdnSharedObject, error) {
	// Validate the CDN Space identity and block-store dependency.
	if opts.SpaceID == "" {
		return nil, errors.New("cdn shared object: SpaceID required")
	}
	if opts.BlockStore == nil {
		return nil, errors.New("cdn shared object: BlockStore required")
	}

	// Initialize the CDN mount and its state and health watches.
	so := &CdnSharedObject{
		spaceID: opts.SpaceID,
		bus:     opts.Bus,
		peerID:  opts.PeerID,
		bs:      opts.BlockStore,
		meta: &sobject.SharedObjectMeta{
			BodyType: CdnBodyType,
		},
		watch:  ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](nil),
		health: ccontainer.NewCContainer[*sobject.SharedObjectHealth](nil),
	}
	so.snap = newCdnStateSnapshot(so)
	so.watch.SetValue(so.snap)
	so.setHealth(nil)
	return so, nil
}

// CdnBodyType is the body_type recorded in the synthetic SharedObjectMeta that
// CdnSharedObject returns. It lets UI code identify a CDN-mount source without
// depending on the well-known Space ID string.
const CdnBodyType = "cdn.spacewave"

// GetBus returns the session bus. Returns nil for anonymous local-only mounts
// that were constructed without a bus.
func (s *CdnSharedObject) GetBus() bus.Bus {
	return s.bus
}

// GetPeerID returns the local peer id recorded at construction time. May be
// empty for anonymous mounts.
func (s *CdnSharedObject) GetPeerID() peer.ID {
	return s.peerID
}

// GetSharedObjectID returns the CDN Space ULID.
func (s *CdnSharedObject) GetSharedObjectID() string {
	return s.spaceID
}

// GetBlockStore returns the anonymous CDN-backed block store.
func (s *CdnSharedObject) GetBlockStore() bstore.BlockStore {
	return s.bs
}

// GetMeta returns the synthetic metadata used for display surfaces.
// Display_name is CdnDisplayName; public_read is implicitly true because the
// mount is anonymous and served from the public CDN.
func (s *CdnSharedObject) GetMeta() *sobject.SharedObjectMeta {
	return s.meta
}

// GetDisplayName returns the fixed CDN display label.
func (s *CdnSharedObject) GetDisplayName() string {
	return CdnDisplayName
}

// IsPublicRead reports the CDN mount's public_read flag, which is always
// true. Exposed as a method so call sites do not hard-code the value.
func (s *CdnSharedObject) IsPublicRead() bool {
	return true
}

// GetCheckpoint returns the body of the checkpoint in the most recent
// CdnRootPointer. Returns nil, nil before the first published checkpoint. CDN
// Spaces publish plain (unencrypted) state data because the data is public.
func (s *CdnSharedObject) GetCheckpoint() (*sobject.SOCheckpointInner, error) {
	// Decode the published checkpoint.
	checkpoint := s.bs.Pointer().GetCheckpoint()
	if checkpoint == nil {
		return nil, nil
	}
	inner, err := checkpoint.UnmarshalInner()
	if err != nil {
		return nil, errors.Wrap(err, "decode cdn checkpoint")
	}
	return inner, nil
}

// RefreshSnapshot forces the CDN block store to re-fetch the root pointer
// and emits a fresh cdnStateSnapshot on the watch container. Callers that
// observe cdn-root-changed signals invoke this so snapshot consumers such as
// SpaceSharedObjectBody see the new head ref.
func (s *CdnSharedObject) RefreshSnapshot(ctx context.Context) error {
	if _, err := s.bs.Refresh(ctx); err != nil {
		s.health.SetValue(sobject.BuildSharedObjectHealthFromError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
			err,
		))
		return errors.Wrap(err, "refresh cdn root pointer")
	}
	s.watch.SetValue(newCdnStateSnapshot(s))
	s.setHealth(nil)
	return nil
}

// GetHeadInnerState decodes the World of the published checkpoint. Returns
// nil, nil before the first published checkpoint.
func (s *CdnSharedObject) GetHeadInnerState() (*sobject_world_engine.InnerState, error) {
	// Decode the World state of the checkpoint.
	checkpoint, err := s.GetCheckpoint()
	if err != nil || checkpoint == nil {
		return nil, err
	}
	inner := &sobject_world_engine.InnerState{}
	if err := inner.UnmarshalVT(checkpoint.GetStateData()); err != nil {
		return nil, errors.Wrap(err, "decode InnerState")
	}
	return inner, nil
}

// AccessLocalStateStore is not supported on a read-only CDN mount.
func (s *CdnSharedObject) AccessLocalStateStore(_ context.Context, _ string, _ func()) (kvtx.Store, func(), error) {
	return nil, nil, ErrCdnReadOnly
}

// GetSharedObjectState returns a snapshot that exposes the published CDN
// checkpoint. Participant and transform methods return errors because the CDN
// mount has no participants and no transformer.
func (s *CdnSharedObject) GetSharedObjectState(_ context.Context) (sobject.SharedObjectStateSnapshot, error) {
	return s.snap, nil
}

// AccessSharedObjectState returns a watchable state container. Callers
// observe refreshed CDN checkpoints by waiting on value changes; the session
// layer invokes RefreshSnapshot after cdn-root-changed WS frames so a
// fresh cdnStateSnapshot is emitted here.
func (s *CdnSharedObject) AccessSharedObjectState(_ context.Context, _ func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return s.watch, func() {}, nil
}

// AccessSharedObjectHealth returns a watchable health container for the CDN mount.
func (s *CdnSharedObject) AccessSharedObjectHealth(_ context.Context, _ func()) (ccontainer.Watchable[*sobject.SharedObjectHealth], func(), error) {
	return s.health, func() {}, nil
}

// QueueOperation is not supported on a read-only CDN mount.
func (s *CdnSharedObject) QueueOperation(_ context.Context, _ []byte) (string, error) {
	return "", ErrCdnReadOnly
}

// cdnStateSnapshot is a minimal sobject.SharedObjectStateSnapshot tied to a
// CdnSharedObject. Methods that require grants or a local participant return
// errors because anonymous CDN mounts have neither.
type cdnStateSnapshot struct {
	so *CdnSharedObject
}

func newCdnStateSnapshot(so *CdnSharedObject) *cdnStateSnapshot {
	return &cdnStateSnapshot{so: so}
}

// setHealth updates the derived health snapshot from the current CDN root pointer.
func (s *CdnSharedObject) setHealth(err error) {
	// Publish a CDN refresh failure as the SharedObject health state.
	if err != nil {
		s.health.SetValue(sobject.BuildSharedObjectHealthFromError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
			err,
		))
		return
	}

	// Derive health from the current CDN checkpoint and World head.
	inner, err := s.GetHeadInnerState()
	if err != nil {
		s.health.SetValue(sobject.BuildSharedObjectHealthFromError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
			err,
		))
		return
	}

	// Keep the CDN mount loading until its World head is published.
	if inner == nil || inner.GetHeadRef() == nil {
		s.health.SetValue(sobject.NewSharedObjectLoadingHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		))
		return
	}

	// Mark the CDN SharedObject ready after its World head is available.
	s.health.SetValue(sobject.NewSharedObjectReadyHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
	))
}

// GetParticipantConfig returns ErrNotParticipant because the CDN mount has no
// local participant entry (anonymous access only).
func (s *cdnStateSnapshot) GetParticipantConfig(_ context.Context) (*sobject.SOParticipantConfig, error) {
	return nil, sobject.ErrNotParticipant
}

// GetParticipantConfigForPeer rejects signers on anonymous CDN mounts.
func (s *cdnStateSnapshot) GetParticipantConfigForPeer(_ context.Context, _ string) (*sobject.SOParticipantConfig, error) {
	return nil, sobject.ErrNotParticipant
}

// GetConfig returns an empty config because the CDN mount holds no config.
func (s *cdnStateSnapshot) GetConfig(_ context.Context) (*sobject.SharedObjectConfig, error) {
	return &sobject.SharedObjectConfig{}, nil
}

// GetConfigByHash returns ErrConfigHistoryUnavailable because the CDN mount
// holds no config history.
func (s *cdnStateSnapshot) GetConfigByHash(_ context.Context, _ []byte) (*sobject.SharedObjectConfig, error) {
	return nil, sobject.ErrConfigHistoryUnavailable
}

// GetTransformer is not available on a CDN mount because the mount has no
// grants to decrypt a transform config from. CDN Spaces publish plain state
// data, so callers read the checkpoint directly.
func (s *cdnStateSnapshot) GetTransformer(_ context.Context) (*block_transform.Transformer, error) {
	return nil, ErrCdnReadOnly
}

// GetTransformInfo is not available on a CDN mount; see GetTransformer.
func (s *cdnStateSnapshot) GetTransformInfo(_ context.Context) (*sobject.TransformInfo, error) {
	return nil, ErrCdnReadOnly
}

// GetCheckpoint returns the published checkpoint, whose state data is plain.
func (s *cdnStateSnapshot) GetCheckpoint(_ context.Context) (*sobject.SOCheckpointInner, error) {
	return s.so.GetCheckpoint()
}

// GetOperationSet returns an empty set because CDN Spaces publish only
// checkpoints.
func (s *cdnStateSnapshot) GetOperationSet(_ context.Context) (*sobject.SOOperationSet, error) {
	checkpoint, err := s.so.GetCheckpoint()
	if err != nil {
		return nil, err
	}
	return sobject.NewSOOperationSet(s.so.spaceID, checkpoint), nil
}

// DecodeOperation is not available because CDN Spaces hold no operations.
func (s *cdnStateSnapshot) DecodeOperation(_ context.Context, _ *sobject.SOOperationInner) ([]byte, error) {
	return nil, ErrCdnReadOnly
}

// _ is a type assertion.
var (
	_ sobject.SharedObject               = (*CdnSharedObject)(nil)
	_ sobject.SharedObjectHealthAccessor = (*CdnSharedObject)(nil)
	_ sobject.SharedObjectStateSnapshot  = (*cdnStateSnapshot)(nil)
)
