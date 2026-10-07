package s4wave_secret

import (
	"bytes"
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
	// SecretKindProviderCredential is the kind for provider credential blobs
	// such as an auth.json document for an LLM provider.
	SecretKindProviderCredential = "provider_credential" // #nosec G101 -- this identifies a secret kind, not a credential value.
	// ProviderCredentialContentType is the content type for provider credential payloads.
	ProviderCredentialContentType = "application/json"
	// SecretKindStorageCredential is the kind for storage backend access keys.
	// The payload is a block_store_s3.Credentials message.
	SecretKindStorageCredential = "storage_credential" // #nosec G101 -- this identifies a secret kind, not a credential value.
	// StorageCredentialContentType is the content type for storage credential payloads.
	StorageCredentialContentType = "application/x-protobuf"
	// SecretKindSSHPrivateKey is the kind for SSH private-key credentials.
	SecretKindSSHPrivateKey = "ssh_private_key" // #nosec G101 -- this identifies a secret kind, not a credential value.
	// SecretKindSSHPassword is the kind for SSH password credentials.
	SecretKindSSHPassword = "ssh_password" // #nosec G101 -- this identifies a secret kind, not a credential value.
	// SecretKindSSHPassphrase is the kind for SSH private-key passphrases.
	SecretKindSSHPassphrase = "ssh_passphrase" // #nosec G101 -- this identifies a secret kind, not a credential value.
	// SecretKindAPIToken is the kind for bearer tokens that an AppConnector
	// presents to an application's admin API.
	SecretKindAPIToken = "api_token" // #nosec G101 -- this identifies a secret kind, not a credential value.
	// APITokenContentType is the content type for API token payloads.
	APITokenContentType = "text/plain; charset=utf-8" // #nosec G101 -- this is a MIME content type, not a credential value.
	// SSHPrivateKeyContentType is the content type for SSH private-key payloads.
	SSHPrivateKeyContentType = "application/x-pem-file"
	// SSHTextCredentialContentType is the content type for text SSH credentials.
	SSHTextCredentialContentType = "text/plain; charset=utf-8" // #nosec G101 -- this is a MIME content type, not a credential value.
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

// NewProviderCredentialPayload constructs a provider credential payload.
func NewProviderCredentialPayload(credential []byte, ts time.Time) *SecretPayload {
	return newSecretPayload(credential, ProviderCredentialContentType, ts)
}

// NewSSHPrivateKeyPayload constructs an SSH private-key payload.
func NewSSHPrivateKeyPayload(privateKey []byte, ts time.Time) *SecretPayload {
	return newSecretPayload(privateKey, SSHPrivateKeyContentType, ts)
}

// NewSSHPasswordPayload constructs an SSH password payload.
func NewSSHPasswordPayload(password string, ts time.Time) *SecretPayload {
	return newSecretPayload([]byte(password), SSHTextCredentialContentType, ts)
}

// NewSSHPassphrasePayload constructs an SSH private-key passphrase payload.
func NewSSHPassphrasePayload(passphrase string, ts time.Time) *SecretPayload {
	return newSecretPayload([]byte(passphrase), SSHTextCredentialContentType, ts)
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
	// Read the Secret record served by this state watch.
	ctx := strm.Context()
	secret, err := r.readSecret(ctx)
	if err != nil {
		return err
	}
	if secret.GetRef() == nil {
		return ErrMissingSecretRef
	}

	// Mount the nested SharedObject for the lifetime of the watch.
	so, soRef, err := sobject.ExMountSharedObject(ctx, r.b, secret.GetRef(), false, nil)
	if err != nil {
		return err
	}
	defer soRef.Release()

	// Subscribe to the nested SharedObject's changing state.
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
	// Require the parent World object key before creating the nested payload.
	if opts.ObjectKey == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "object_key")
	}

	// Create the nested SharedObject and its payload.
	secret, err := CreateSecretObject(ctx, b, soProvider, opts)
	if err != nil {
		return nil, err
	}

	// Write the redacted Secret metadata as the parent World object.
	wtx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	createdObject, _, err := world.CreateWorldObject(ctx, wtx, opts.ObjectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(secret, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
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

// CreateSecretObject creates the nested SharedObject holding a Secret payload
// and returns the redacted Secret metadata. It writes no World object, so the
// caller records the metadata wherever the Secret is used. opts.ObjectKey is
// ignored.
func CreateSecretObject(
	ctx context.Context,
	b bus.Bus,
	soProvider sobject.SharedObjectProvider,
	opts CreateSecretOptions,
) (*Secret, error) {
	// Fill the defaults for the timestamp, content type, and nested id.
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

	// Create the nested SharedObject and store the first payload version.
	nestedRef, err := soProvider.CreateSharedObject(ctx, nestedID, NewSharedObjectMeta(), "", "")
	if err != nil {
		return nil, errors.Wrap(err, "create nested shared object")
	}
	payload := &SecretPayload{
		Value:       bytes.Clone(opts.Value),
		ContentType: opts.ContentType,
		Version:     1,
		UpdatedAt:   timestamppb.New(opts.Timestamp),
	}
	if err := StoreSecretPayload(ctx, b, nestedRef, payload); err != nil {
		return nil, errors.Wrap(err, "store secret payload")
	}

	return &Secret{
		DisplayName:          opts.DisplayName,
		Kind:                 opts.Kind,
		NestedSharedObjectId: nestedRef.GetProviderResourceRef().GetId(),
		Ref:                  nestedRef.CloneVT(),
		CreatedAt:            timestamppb.New(opts.Timestamp),
		UpdatedAt:            timestamppb.New(opts.Timestamp),
	}, nil
}

// StoreSecretPayload replaces the payload in the nested SharedObject.
func StoreSecretPayload(ctx context.Context, b bus.Bus, ref *sobject.SharedObjectRef, payload *SecretPayload) error {
	// Encode the payload.
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

	// Write it as an operation on the nested object.
	so, soRef, err := sobject.ExMountSharedObject(ctx, b, ref, false, nil)
	if err != nil {
		return err
	}
	defer soRef.Release()
	_, err = sobject.WriteOperation(ctx, so, data, replaceSecretPayload)
	return err
}

// ReadSecretPayload reads the nested SharedObject payload for a granted caller.
func ReadSecretPayload(ctx context.Context, b bus.Bus, secret *Secret) (*SecretPayload, error) {
	// Mount the granted Secret payload and release its SharedObject reference.
	if secret == nil || secret.GetRef() == nil {
		return nil, ErrMissingSecretRef
	}
	so, soRef, err := sobject.ExMountSharedObject(ctx, b, secret.GetRef(), false, nil)
	if err != nil {
		return nil, err
	}
	defer soRef.Release()

	// Read the nested object's current payload snapshot.
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	return ReadSecretPayloadFromSnapshot(ctx, snap)
}

// ReadSecretPayloadFromSnapshot decodes payload bytes from a granted snapshot.
func ReadSecretPayloadFromSnapshot(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (*SecretPayload, error) {
	// Fold the operations over the checkpoint and decode the payload.
	if snap == nil {
		return nil, ErrPayloadAccessDenied
	}
	folded, err := sobject.Fold(ctx, snap, replaceSecretPayload)
	if err != nil {
		return nil, ErrPayloadAccessDenied
	}
	payload := &SecretPayload{}
	if data := folded.StateData; len(data) != 0 {
		if err := payload.UnmarshalVT(data); err != nil {
			return nil, err
		}
	}
	return payload, nil
}

// ReadProviderCredentialPayload reads a provider credential Secret payload after checking its kind.
func ReadProviderCredentialPayload(ctx context.Context, b bus.Bus, secret *Secret) ([]byte, error) {
	if secret == nil || secret.GetKind() != SecretKindProviderCredential {
		return nil, ErrSecretKindMismatch
	}
	payload, err := ReadSecretPayload(ctx, b, secret)
	if err != nil {
		return nil, err
	}
	return bytes.Clone(payload.GetValue()), nil
}

// ReadSSHCredentialPayload reads an SSH Secret payload after checking its kind.
func ReadSSHCredentialPayload(ctx context.Context, b bus.Bus, secret *Secret, expectedKind string) ([]byte, error) {
	if secret == nil || secret.GetKind() != expectedKind {
		return nil, ErrSecretKindMismatch
	}
	payload, err := ReadSecretPayload(ctx, b, secret)
	if err != nil {
		return nil, err
	}
	return bytes.Clone(payload.GetValue()), nil
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
	// Mount the Secret's nested SharedObject as an invite host.
	so, soRef, err := mountSecretInviteHost(ctx, b, secret)
	if err != nil {
		return nil, err
	}
	defer soRef()

	// Add the peer under the local key. A nested Secret records no username.
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
		"",
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
	// Mount the invite host before revoking the peer's nested access.
	so, soRef, err := mountSecretInviteHost(ctx, b, secret)
	if err != nil {
		return false, err
	}
	defer soRef()

	// Revoke the peer's grant through the nested invite host.
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
	// Read the parent World object and release its state after decoding.
	objState, found, err := r.ws.GetObject(ctx, r.objKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}

	// Decode the Secret root from the retained object state.
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
	// Build the browser-safe Secret status from the snapshot.
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
		nextStateData = bytes.Clone(op.GetOpData())
		opResults = append(opResults, sobject.BuildSOOperationResult(
			op.GetPeerId(),
			op.GetNonce(),
			true,
			nil,
		))
	}
	return &nextStateData, opResults, nil
}

func mountSecretInviteHost(
	ctx context.Context,
	b bus.Bus,
	secret *Secret,
) (sobject.SharedObject, func(), error) {
	// Require the Secret to reference its nested SharedObject.
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

func newSecretPayload(value []byte, contentType string, ts time.Time) *SecretPayload {
	if ts.IsZero() {
		ts = time.Now()
	}
	return &SecretPayload{
		Value:       bytes.Clone(value),
		ContentType: contentType,
		Version:     1,
		UpdatedAt:   timestamppb.New(ts),
	}
}

var _ SRPCSecretResourceServiceServer = (*SecretResource)(nil)
