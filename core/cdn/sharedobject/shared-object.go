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
// CDN block store. It exposes the decoded SORoot from the cached CdnRootPointer
// so callers can build a WorldState against the CDN's world without going
// through the normal SO-world-engine controller path.
type CdnSharedObject struct {
	spaceID string
	bus     bus.Bus
	peerID  peer.ID
	bs      *cdn_bstore.CdnBlockStore

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
	// BlockStore is the CDN-backed block store produced by NewCdnBlockStore.
	BlockStore *cdn_bstore.CdnBlockStore
}

// NewCdnSharedObject constructs a new CdnSharedObject. The caller is expected
// to refresh the block store pointer before the first read so GetSORoot
// returns the current published root.
func NewCdnSharedObject(opts CdnSharedObjectOptions) (*CdnSharedObject, error) {
	if opts.SpaceID == "" {
		return nil, errors.New("cdn shared object: SpaceID required")
	}
	if opts.BlockStore == nil {
		return nil, errors.New("cdn shared object: BlockStore required")
	}
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

// GetSORoot returns the signed SORoot decoded from the most recent
// CdnRootPointer. Returns nil if the CDN Space has no published root yet.
func (s *CdnSharedObject) GetSORoot() *sobject.SORoot {
	ptr := s.bs.Pointer()
	if ptr == nil {
		return nil
	}
	return ptr.GetRoot()
}

// GetPlainRootInner decodes the CDN-published SORootInner. Initialized empty
// CDN Spaces can have a normal shared-object root before any world packs are
// published; treat that as "no CDN world head yet" only while the pointer has
// no packs.
func (s *CdnSharedObject) GetPlainRootInner() (*sobject.SORootInner, error) {
	ptr := s.bs.Pointer()
	if ptr == nil || ptr.GetRoot() == nil || len(ptr.GetRoot().GetInner()) == 0 {
		return nil, nil
	}
	inner := &sobject.SORootInner{}
	if err := inner.UnmarshalVT(ptr.GetRoot().GetInner()); err != nil {
		if len(ptr.GetPacks()) == 0 {
			return nil, nil
		}
		return nil, errors.Wrap(err, "decode SORootInner")
	}
	return inner, nil
}

// RefreshSnapshot forces the CDN block store to re-fetch the root pointer
// and emits a fresh cdnStateSnapshot on the watch container. Callers that
// observe cdn-root-changed signals invoke this so downstream consumers
// (engine refresh goroutine, SpaceSharedObjectBody) see the new head ref.
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

// GetHeadInnerState decodes SORoot.Inner as a SORootInner, then unmarshals its
// StateData as the sobject_world_engine.InnerState. CDN Spaces publish Inner
// as plain (unencrypted) protobuf because the data is public; the admin CLI
// (runPostRoot) produces the same shape.
// Returns nil, nil when there is no published root yet.
func (s *CdnSharedObject) GetHeadInnerState() (*sobject_world_engine.InnerState, error) {
	sori, err := s.GetPlainRootInner()
	if err != nil {
		return nil, err
	}
	if sori == nil {
		return nil, nil
	}
	inner := &sobject_world_engine.InnerState{}
	if len(sori.GetStateData()) == 0 {
		return inner, nil
	}
	if err := inner.UnmarshalVT(sori.GetStateData()); err != nil {
		return nil, errors.Wrap(err, "decode InnerState")
	}
	return inner, nil
}

// AccessLocalStateStore is not supported on a read-only CDN mount.
func (s *CdnSharedObject) AccessLocalStateStore(_ context.Context, _ string, _ func()) (kvtx.Store, func(), error) {
	return nil, nil, ErrCdnReadOnly
}

// GetSharedObjectState returns a snapshot that exposes the decoded CDN root.
// Mutation-oriented snapshot methods (ProcessOperations, GetParticipantConfig)
// return errors because the CDN mount has no participants and no transformer.
func (s *CdnSharedObject) GetSharedObjectState(_ context.Context) (sobject.SharedObjectStateSnapshot, error) {
	return s.snap, nil
}

// AccessSharedObjectState returns a watchable state container. Callers
// observe refreshed CDN roots by waiting on value changes; the session
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

// WaitOperation is not supported on a read-only CDN mount.
func (s *CdnSharedObject) WaitOperation(_ context.Context, _ string) (uint64, bool, error) {
	return 0, false, ErrCdnReadOnly
}

// ClearOperationResult is not supported on a read-only CDN mount.
func (s *CdnSharedObject) ClearOperationResult(_ context.Context, _ string) error {
	return ErrCdnReadOnly
}

// ProcessOperations is not supported on a read-only CDN mount.
func (s *CdnSharedObject) ProcessOperations(_ context.Context, _ bool, _ sobject.ProcessOpsFunc) error {
	return ErrCdnReadOnly
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
	if err != nil {
		s.health.SetValue(sobject.BuildSharedObjectHealthFromError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
			err,
		))
		return
	}
	if s.bs.Pointer() == nil {
		s.health.SetValue(sobject.NewSharedObjectLoadingHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		))
		return
	}
	s.health.SetValue(sobject.NewSharedObjectReadyHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
	))
}

// GetParticipantConfig returns ErrNotParticipant because the CDN mount has no
// local participant entry (anonymous access only).
func (s *cdnStateSnapshot) GetParticipantConfig(_ context.Context) (*sobject.SOParticipantConfig, error) {
	return nil, sobject.ErrNotParticipant
}

// GetTransformer is not available on a CDN mount because the mount has no
// grants to decrypt a transform config from. CDN Spaces publish their state
// as plain SORootInner, so callers should use GetRootInner directly instead.
func (s *cdnStateSnapshot) GetTransformer(_ context.Context) (*block_transform.Transformer, error) {
	return nil, ErrCdnReadOnly
}

// GetTransformInfo is not available on a CDN mount; see GetTransformer.
func (s *cdnStateSnapshot) GetTransformInfo(_ context.Context) (*sobject.TransformInfo, error) {
	return nil, ErrCdnReadOnly
}

// GetOpQueue returns empty queues because CDN mounts do not submit operations.
func (s *cdnStateSnapshot) GetOpQueue(_ context.Context) ([]*sobject.SOOperation, []*sobject.QueuedSOOperation, error) {
	return nil, nil, nil
}

// GetRootInner decodes the plain-encoded SORootInner that the CDN admin CLI
// emits for public_read Spaces (see runPostRoot). Returns nil, nil when no
// root has been published yet.
func (s *cdnStateSnapshot) GetRootInner(_ context.Context) (*sobject.SORootInner, error) {
	return s.so.GetPlainRootInner()
}

// ProcessOperations is not supported on a read-only CDN mount.
func (s *cdnStateSnapshot) ProcessOperations(
	_ context.Context,
	_ []*sobject.SOOperation,
	_ sobject.SnapshotProcessOpsFunc,
) (
	*sobject.SORoot,
	[]*sobject.SOOperationRejection,
	[]*sobject.SOOperation,
	error,
) {
	return nil, nil, nil, ErrCdnReadOnly
}

// _ is a type assertion.
var (
	_ sobject.SharedObject               = (*CdnSharedObject)(nil)
	_ sobject.SharedObjectHealthAccessor = (*CdnSharedObject)(nil)
	_ sobject.SharedObjectStateSnapshot  = (*cdnStateSnapshot)(nil)
)
