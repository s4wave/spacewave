package s4wave_secret

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

const (
	// SecretTypeID is the ObjectType id for Secret objects.
	SecretTypeID = "spacewave/secret"
	// SecretBodyType is the nested SharedObject body type for Secret payloads.
	SecretBodyType = "secret"
	// SecretKindMatrixAccessToken is the kind for Matrix access tokens.
	SecretKindMatrixAccessToken = "matrix_access_token"
	// MatrixAccessTokenContentType is the content type for Matrix access tokens.
	MatrixAccessTokenContentType = "text/plain; charset=utf-8"
)

// SecretResource implements the SecretResourceService SRPC interface.
type SecretResource struct {
	le           *logrus.Entry
	b            bus.Bus
	ws           world.WorldState
	objKey       string
	mux          srpc.Mux
	challengeMu  sync.Mutex
	challenges   map[string]*payloadReadChallenge
	challengeTTL time.Duration
}

// CreateSecretOptions configures CreateSecret.
type CreateSecretOptions struct {
	// ObjectKey is the parent World object key.
	ObjectKey string
	// DisplayName is the parent Secret display name.
	DisplayName string
	// Kind is the Secret kind.
	Kind string
	// ContentType is the payload content type.
	ContentType string
	// Value is the raw payload stored in the nested SharedObject.
	Value []byte
	// Timestamp is used for created_at and updated_at.
	Timestamp time.Time
	// NestedSharedObjectId optionally fixes the nested SharedObject id.
	NestedSharedObjectId string
}

// NewSecretResource creates a new SecretResource.
func NewSecretResource(le *logrus.Entry, b bus.Bus, ws world.WorldState, objKey string) *SecretResource {
	r := &SecretResource{
		le:           le,
		b:            b,
		ws:           ws,
		objKey:       objKey,
		challengeTTL: time.Minute,
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterSecretResourceService(mux, r)
	})
	return r
}

// NewSecretBlock constructs a new Secret block.
func NewSecretBlock() block.Block {
	return &Secret{}
}

// NewSecretPayloadBlock constructs a new SecretPayload block.
func NewSecretPayloadBlock() block.Block {
	return &SecretPayload{}
}

// NewSharedObjectMeta constructs Secret nested SharedObject metadata.
func NewSharedObjectMeta() *sobject.SharedObjectMeta {
	return &sobject.SharedObjectMeta{
		BodyType: SecretBodyType,
	}
}

// NewMatrixAccessTokenPayload constructs a Matrix access token payload.
func NewMatrixAccessTokenPayload(token string, ts time.Time) *SecretPayload {
	if ts.IsZero() {
		ts = time.Now()
	}
	return &SecretPayload{
		Value:       []byte(token),
		ContentType: MatrixAccessTokenContentType,
		Version:     1,
		UpdatedAt:   timestamppb.New(ts),
	}
}

// GetMux returns the srpc mux for this resource.
func (r *SecretResource) GetMux() srpc.Mux {
	return r.mux
}

// WatchState streams browser-safe Secret state.
func (r *SecretResource) WatchState(
	_ *WatchStateRequest,
	strm SRPCSecretResourceService_WatchStateStream,
) error {
	ctx := strm.Context()
	secret, err := r.readSecret(ctx)
	if err != nil {
		return err
	}
	if secret.GetRef() == nil {
		return ErrMissingSecretRef
	}

	so, soRef, err := sobject.ExMountSharedObject(ctx, r.b, secret.GetRef(), false, nil)
	if err != nil {
		return err
	}
	defer soRef.Release()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer relStateCtr()

	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			state := buildSecretStateFromSnapshot(ctx, secret, so, snap)
			return strm.Send(&WatchStateResponse{State: state})
		},
		nil,
	)
}

// CreateSecret creates a parent World object plus nested SharedObject payload.
func CreateSecret(
	ctx context.Context,
	b bus.Bus,
	soProvider sobject.SharedObjectProvider,
	engine world.Engine,
	opts CreateSecretOptions,
) (*Secret, error) {
	if opts.ObjectKey == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "object_key")
	}
	if opts.Timestamp.IsZero() {
		opts.Timestamp = time.Now()
	}
	if opts.ContentType == "" {
		opts.ContentType = "application/octet-stream"
	}
	nestedID := opts.NestedSharedObjectId
	if nestedID == "" {
		nestedID = "secret-" + sobject.NewSOOperationLocalID()
	}

	nestedRef, err := soProvider.CreateSharedObject(ctx, nestedID, NewSharedObjectMeta(), "", "")
	if err != nil {
		return nil, errors.Wrap(err, "create nested shared object")
	}
	payload := &SecretPayload{
		Value:       append([]byte(nil), opts.Value...),
		ContentType: opts.ContentType,
		Version:     1,
		UpdatedAt:   timestamppb.New(opts.Timestamp),
	}
	if err := StoreSecretPayload(ctx, b, nestedRef, payload); err != nil {
		return nil, errors.Wrap(err, "store secret payload")
	}

	secret := &Secret{
		DisplayName:          opts.DisplayName,
		Kind:                 opts.Kind,
		NestedSharedObjectId: nestedRef.GetProviderResourceRef().GetId(),
		Ref:                  nestedRef.CloneVT(),
		CreatedAt:            timestamppb.New(opts.Timestamp),
		UpdatedAt:            timestamppb.New(opts.Timestamp),
	}

	wtx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	if _, _, err := world.CreateWorldObject(ctx, wtx, opts.ObjectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(secret, true)
		return nil
	}); err != nil {
		wtx.Discard()
		return nil, err
	}
	if err := world_types.SetObjectType(ctx, wtx, opts.ObjectKey, SecretTypeID); err != nil {
		wtx.Discard()
		return nil, err
	}
	if err := wtx.Commit(ctx); err != nil {
		return nil, err
	}
	return secret, nil
}

// StoreSecretPayload replaces the payload in the nested SharedObject.
func StoreSecretPayload(ctx context.Context, b bus.Bus, ref *sobject.SharedObjectRef, payload *SecretPayload) error {
	if ref == nil {
		return ErrMissingSecretRef
	}
	if payload == nil {
		payload = &SecretPayload{}
	}
	data, err := payload.MarshalVT()
	if err != nil {
		return err
	}

	so, soRef, err := sobject.ExMountSharedObject(ctx, b, ref, false, nil)
	if err != nil {
		return err
	}
	defer soRef.Release()

	processCtx, cancelProcess := context.WithCancel(ctx)
	processErr := make(chan error, 1)
	go func() {
		err := so.ProcessOperations(processCtx, true, replaceSecretPayload)
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		processErr <- err
	}()
	defer cancelProcess()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer relStateCtr()

	if _, err := so.QueueOperation(ctx, data); err != nil {
		return err
	}

	err = waitSecretPayload(ctx, stateCtr, payload)
	cancelProcess()
	perr := <-processErr
	if err == nil {
		err = perr
	}
	if err != nil {
		return err
	}
	return nil
}

// ReadSecretPayload reads the nested SharedObject payload for a granted caller.
func ReadSecretPayload(ctx context.Context, b bus.Bus, secret *Secret) (*SecretPayload, error) {
	if secret == nil || secret.GetRef() == nil {
		return nil, ErrMissingSecretRef
	}
	so, soRef, err := sobject.ExMountSharedObject(ctx, b, secret.GetRef(), false, nil)
	if err != nil {
		return nil, err
	}
	defer soRef.Release()

	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	return ReadSecretPayloadFromSnapshot(ctx, snap)
}

// ReadSecretPayloadFromSnapshot decodes payload bytes from a granted snapshot.
func ReadSecretPayloadFromSnapshot(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (*SecretPayload, error) {
	if snap == nil {
		return nil, ErrPayloadAccessDenied
	}
	root, err := snap.GetRootInner(ctx)
	if err != nil {
		return nil, ErrPayloadAccessDenied
	}
	payload := &SecretPayload{}
	if data := root.GetStateData(); len(data) != 0 {
		if err := payload.UnmarshalVT(data); err != nil {
			return nil, err
		}
	}
	return payload, nil
}

// ReadMatrixAccessToken reads a Matrix access token Secret payload.
func ReadMatrixAccessToken(ctx context.Context, b bus.Bus, secret *Secret) (string, error) {
	payload, err := ReadSecretPayload(ctx, b, secret)
	if err != nil {
		return "", err
	}
	return string(payload.GetValue()), nil
}

// AddSecretParticipant grants nested SharedObject access to a peer.
func AddSecretParticipant(
	ctx context.Context,
	b bus.Bus,
	secret *Secret,
	targetPeerIDStr string,
	targetPub crypto.PubKey,
	role sobject.SOParticipantRole,
	entityID string,
) (*sobject.SOGrant, error) {
	so, soRef, err := mountSecretInviteHost(ctx, b, secret)
	if err != nil {
		return nil, err
	}
	defer soRef()

	ih := so.(sobject.InviteHost)
	return sobject.AddSOParticipant(
		ctx,
		ih.GetSOHost(),
		so.GetSharedObjectID(),
		ih.GetPrivKey(),
		so.GetPeerID().String(),
		targetPeerIDStr,
		targetPub,
		role,
		entityID,
	)
}

// RemoveSecretParticipant revokes nested SharedObject access from a peer.
func RemoveSecretParticipant(
	ctx context.Context,
	b bus.Bus,
	secret *Secret,
	targetPeerIDStr string,
	revInfo *sobject.SORevocationInfo,
) (bool, error) {
	so, soRef, err := mountSecretInviteHost(ctx, b, secret)
	if err != nil {
		return false, err
	}
	defer soRef()

	ih := so.(sobject.InviteHost)
	return sobject.RemoveSOParticipant(
		ctx,
		ih.GetSOHost(),
		targetPeerIDStr,
		ih.GetPrivKey(),
		revInfo,
	)
}

// UnmarshalSecret unmarshals a Secret from a cursor.
func UnmarshalSecret(ctx context.Context, bcs *block.Cursor) (*Secret, error) {
	return block.UnmarshalBlock[*Secret](ctx, bcs, NewSecretBlock)
}

// MarshalBlock marshals the Secret to binary.
func (s *Secret) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the Secret from binary.
func (s *Secret) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// MarshalBlock marshals the SecretPayload to binary.
func (p *SecretPayload) MarshalBlock() ([]byte, error) {
	return p.MarshalVT()
}

// UnmarshalBlock unmarshals the SecretPayload from binary.
func (p *SecretPayload) UnmarshalBlock(data []byte) error {
	return p.UnmarshalVT(data)
}

func (r *SecretResource) readSecret(ctx context.Context) (*Secret, error) {
	objState, found, err := r.ws.GetObject(ctx, r.objKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}
	var secret *Secret
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		secret, uerr = UnmarshalSecret(ctx, bcs)
		return uerr
	})
	if err != nil {
		return nil, err
	}
	if secret == nil {
		secret = &Secret{}
	}
	return secret, nil
}

func buildSecretStateFromSnapshot(
	ctx context.Context,
	secret *Secret,
	so sobject.SharedObject,
	snap sobject.SharedObjectStateSnapshot,
) *SecretState {
	state := &SecretState{
		Secret: secret.CloneVT(),
		GrantStatus: &SecretGrantStatus{
			PeerId: so.GetPeerID().String(),
		},
		Health: sobject.NewSharedObjectReadyHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		),
	}
	if snap == nil {
		state.Health = sobject.NewSharedObjectLoadingHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		)
		return state
	}
	if participant, err := snap.GetParticipantConfig(ctx); err == nil {
		state.GrantStatus.Participant = true
		state.GrantStatus.Role = participant.GetRole()
	}
	if info, err := snap.GetTransformInfo(ctx); err == nil {
		state.GrantStatus.Readable = true
		state.GrantStatus.GrantCount = info.GrantCount
	}
	return state
}

func replaceSecretPayload(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	currentStateData []byte,
	ops []*sobject.SOOperationInner,
) (*[]byte, []*sobject.SOOperationResult, error) {
	nextStateData := currentStateData
	opResults := make([]*sobject.SOOperationResult, 0, len(ops))
	for _, op := range ops {
		payload := &SecretPayload{}
		if err := payload.UnmarshalVT(op.GetOpData()); err != nil {
			return nil, nil, err
		}
		nextStateData = append([]byte(nil), op.GetOpData()...)
		opResults = append(opResults, sobject.BuildSOOperationResult(
			op.GetPeerId(),
			op.GetNonce(),
			true,
			nil,
		))
	}
	return &nextStateData, opResults, nil
}

func waitSecretPayload(
	ctx context.Context,
	stateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
	expected *SecretPayload,
) error {
	var current sobject.SharedObjectStateSnapshot
	for {
		next, err := stateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return err
		}
		current = next
		payload, err := ReadSecretPayloadFromSnapshot(ctx, next)
		if err == nil && payload.EqualVT(expected) {
			return nil
		}
	}
}

func mountSecretInviteHost(
	ctx context.Context,
	b bus.Bus,
	secret *Secret,
) (sobject.SharedObject, func(), error) {
	if secret == nil || secret.GetRef() == nil {
		return nil, nil, ErrMissingSecretRef
	}
	so, soRef, err := sobject.ExMountSharedObject(ctx, b, secret.GetRef(), false, nil)
	if err != nil {
		return nil, nil, err
	}
	if _, ok := so.(sobject.InviteHost); !ok {
		soRef.Release()
		return nil, nil, errors.New("secret shared object does not support participant mutation")
	}
	return so, soRef.Release, nil
}

// _ is a type assertion
var _ SRPCSecretResourceServiceServer = ((*SecretResource)(nil))
