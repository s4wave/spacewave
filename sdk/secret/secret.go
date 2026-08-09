package s4wave_secret

import (
	"bytes"
	"context"
	"crypto/rand"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
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
	SecretKindMatrixAccessToken = "matrix_access_token" // #nosec G101 -- this identifies a secret kind, not a credential value.
	// SecretKindSSHPrivateKey is the kind for SSH private-key credentials.
	SecretKindSSHPrivateKey = "ssh_private_key" // #nosec G101 -- this identifies a secret kind, not a credential value.
	// SecretKindSSHPassword is the kind for SSH password credentials.
	SecretKindSSHPassword = "ssh_password" // #nosec G101 -- this identifies a secret kind, not a credential value.
	// SecretKindSSHPassphrase is the kind for SSH private-key passphrases.
	SecretKindSSHPassphrase = "ssh_passphrase" // #nosec G101 -- this identifies a secret kind, not a credential value.
	// MatrixAccessTokenContentType is the content type for Matrix access tokens.
	MatrixAccessTokenContentType = "text/plain; charset=utf-8" // #nosec G101 -- this is a MIME content type, not a credential value.
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
	// CreationToken is the opaque identity of the authorized creation request.
	CreationToken []byte
	// PayloadIdentity is an opaque identity shared by the parent and exact payload.
	PayloadIdentity []byte
}

type secretPayloadStoreOperation struct {
	so         sobject.SharedObject
	stateCtr   ccontainer.Watchable[sobject.SharedObjectStateSnapshot]
	expected   *SecretPayload
	processor  *routine.RoutineContainer
	state      *routine.RoutineContainer
	waiter     *routine.RoutineContainer
	bcast      broadcast.Broadcast
	localID    string
	stored     bool
	waitDone   bool
	initialize bool
	err        error
	processErr error
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
	return newSecretPayload([]byte(token), MatrixAccessTokenContentType, ts)
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

// CreateSecret creates or reconciles a parent World object and its stable nested payload.
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
	if opts.Kind == "" {
		return nil, errors.New("kind cannot be empty")
	}
	if opts.ContentType == "" {
		opts.ContentType = "application/octet-stream"
	}
	if len(opts.PayloadIdentity) == 0 {
		opts.PayloadIdentity = make([]byte, 32)
		if _, err := rand.Read(opts.PayloadIdentity); err != nil {
			return nil, errors.Wrap(err, "generate payload identity")
		}
	}
	nestedID := opts.NestedSharedObjectId
	if nestedID == "" {
		nestedID = "secret-" + sobject.NewSOOperationLocalID()
	}
	meta := NewSharedObjectMeta()
	meta.BodyMeta = append([]byte(nil), opts.CreationToken...)
	nestedRef, err := soProvider.CreateSharedObject(ctx, nestedID, meta, "", "")
	createdNested := err == nil
	if errors.Is(err, sobject.ErrSharedObjectExists) {
		nestedRef, err = lookupSecretSharedObjectRef(ctx, soProvider, nestedID, opts.CreationToken)
	}
	if err != nil {
		return nil, errors.Wrap(err, "create or adopt nested shared object")
	}

	payload := &SecretPayload{
		Value:           append([]byte(nil), opts.Value...),
		ContentType:     opts.ContentType,
		Version:         1,
		UpdatedAt:       timestamppb.New(opts.Timestamp),
		PayloadIdentity: append([]byte(nil), opts.PayloadIdentity...),
	}
	if createdNested {
		if err := storeOrVerifySecretPayload(ctx, b, nestedRef, payload); err != nil {
			return nil, errors.Wrap(err, "store secret payload")
		}
	} else {
		existingPayload, err := ReadSecretPayload(ctx, b, &Secret{Ref: nestedRef})
		if err != nil {
			return nil, errors.Wrap(err, "read adopted secret payload")
		}
		if existingPayload.EqualVT(&SecretPayload{}) {
			if err := storeOrVerifySecretPayload(ctx, b, nestedRef, payload); err != nil {
				return nil, errors.Wrap(err, "store adopted secret payload")
			}
		} else if !existingPayload.EqualVT(payload) {
			return nil, errors.New("existing nested secret payload does not match creation request")
		}
	}

	secret := &Secret{
		DisplayName:          opts.DisplayName,
		Kind:                 opts.Kind,
		NestedSharedObjectId: nestedID,
		Ref:                  nestedRef.CloneVT(),
		CreatedAt:            timestamppb.New(opts.Timestamp),
		UpdatedAt:            timestamppb.New(opts.Timestamp),
		CreationToken:        append([]byte(nil), opts.CreationToken...),
		PayloadIdentity:      append([]byte(nil), opts.PayloadIdentity...),
	}
	wtx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer wtx.Discard()
	createdState, _, err := world.CreateWorldObject(ctx, wtx, opts.ObjectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(secret, true)
		return nil
	})
	world.ReleaseObjectState(createdState)
	if err == nil {
		err = world_types.SetObjectType(ctx, wtx, opts.ObjectKey, SecretTypeID)
	}
	if err == nil {
		err = wtx.Commit(ctx)
	} else {
		wtx.Discard()
	}
	if err == nil {
		return secret, nil
	}
	if !errors.Is(err, world.ErrObjectExists) {
		return nil, err
	}

	// A concurrent identical retry may have committed while this call prepared
	// the same stable nested object. Read and verify the winner.
	readTx, txErr := engine.NewTransaction(ctx, false)
	if txErr != nil {
		return nil, txErr
	}
	defer readTx.Discard()
	existing, objRef, txErr := world.LookupObject[*Secret](ctx, readTx, opts.ObjectKey, NewSecretBlock)
	world.ReleaseObjectState(objRef)
	if txErr != nil {
		return nil, txErr
	}
	if txErr = world_types.CheckObjectType(ctx, readTx, opts.ObjectKey, SecretTypeID); txErr != nil {
		return nil, txErr
	}
	if !existing.EqualVT(secret) {
		return nil, world.ErrObjectExists
	}
	return existing, nil
}

func lookupSecretSharedObjectRef(
	ctx context.Context,
	provider sobject.SharedObjectProvider,
	nestedID string,
	creationToken []byte,
) (*sobject.SharedObjectRef, error) {
	ctr, release, err := provider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer release()
	list, err := ctr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, entry := range list.GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() != nestedID {
			continue
		}
		if entry.GetMeta().GetBodyType() != SecretBodyType ||
			!bytes.Equal(entry.GetMeta().GetBodyMeta(), creationToken) {
			return nil, sobject.ErrSharedObjectExists
		}
		return entry.GetRef().CloneVT(), nil
	}
	return nil, sobject.ErrSharedObjectExists
}

func storeOrVerifySecretPayload(
	ctx context.Context,
	b bus.Bus,
	ref *sobject.SharedObjectRef,
	payload *SecretPayload,
) error {
	var storeErr error
	for range 3 {
		storeErr = initializeSecretPayload(ctx, b, ref, payload)
		if storeErr == nil {
			return nil
		}
		existing, readErr := ReadSecretPayload(ctx, b, &Secret{Ref: ref})
		if readErr == nil && existing.EqualVT(payload) {
			return nil
		}
	}
	return storeErr
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

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer relStateCtr()

	op := newSecretPayloadStoreOperation(so, stateCtr, payload)
	return op.Store(ctx, data)
}

func initializeSecretPayload(ctx context.Context, b bus.Bus, ref *sobject.SharedObjectRef, payload *SecretPayload) error {
	if ref == nil {
		return ErrMissingSecretRef
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
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer relStateCtr()
	op := newSecretPayloadStoreOperation(so, stateCtr, payload)
	op.initialize = true
	return op.Store(ctx, data)
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

// ReadSSHCredentialPayload reads an SSH Secret payload after checking its kind.
func ReadSSHCredentialPayload(ctx context.Context, b bus.Bus, secret *Secret, expectedKind string) ([]byte, error) {
	if secret == nil || secret.GetKind() != expectedKind {
		return nil, ErrSecretKindMismatch
	}
	payload, err := ReadSecretPayload(ctx, b, secret)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), payload.GetValue()...), nil
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
			op.GetPeerId(), op.GetNonce(), true, nil,
		))
	}
	return &nextStateData, opResults, nil
}

func initializeSecretPayloadState(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	currentStateData []byte,
	ops []*sobject.SOOperationInner,
) (*[]byte, []*sobject.SOOperationResult, error) {
	nextStateData := currentStateData
	changed := false
	opResults := make([]*sobject.SOOperationResult, 0, len(ops))
	for _, op := range ops {
		payload := &SecretPayload{}
		if err := payload.UnmarshalVT(op.GetOpData()); err != nil {
			return nil, nil, err
		}
		if len(payload.GetPayloadIdentity()) < 16 {
			return nil, nil, errors.New("payload identity is missing")
		}
		if len(nextStateData) == 0 {
			nextStateData = append([]byte(nil), op.GetOpData()...)
			changed = true
			opResults = append(opResults, sobject.BuildSOOperationResult(
				op.GetPeerId(), op.GetNonce(), true, nil,
			))
			continue
		}
		existing := &SecretPayload{}
		if err := existing.UnmarshalVT(nextStateData); err != nil {
			return nil, nil, err
		}
		if existing.EqualVT(payload) {
			opResults = append(opResults, sobject.BuildSOOperationResult(
				op.GetPeerId(), op.GetNonce(), true, nil,
			))
			continue
		}
		opResults = append(opResults, sobject.BuildSOOperationResult(
			op.GetPeerId(), op.GetNonce(), false,
			&sobject.SOOperationRejectionErrorDetails{
				ErrorMsg: "secret payload already initialized by a different request",
			},
		))
	}
	if !changed {
		return nil, opResults, nil
	}
	return &nextStateData, opResults, nil
}

func newSecretPayloadStoreOperation(
	so sobject.SharedObject,
	stateCtr ccontainer.Watchable[sobject.SharedObjectStateSnapshot],
	expected *SecretPayload,
) *secretPayloadStoreOperation {
	op := &secretPayloadStoreOperation{
		so:       so,
		stateCtr: stateCtr,
		expected: expected,
	}
	op.processor = routine.NewRoutineContainer()
	op.processor.SetRoutine(op.processOperations)
	op.state = routine.NewRoutineContainer()
	op.state.SetRoutine(op.watchPayload)
	op.waiter = routine.NewRoutineContainer()
	op.waiter.SetRoutine(op.waitOperation)
	return op
}

func (op *secretPayloadStoreOperation) Store(ctx context.Context, data []byte) error {
	op.processor.SetContext(ctx, false)
	op.state.SetContext(ctx, false)
	defer op.stop()

	localID, err := op.so.QueueOperation(ctx, data)
	if err != nil {
		return err
	}
	op.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		op.localID = localID
		broadcast()
	})
	op.waiter.SetContext(ctx, false)
	if err := op.waitStored(ctx); err != nil {
		return err
	}
	return op.getProcessErr()
}

func (op *secretPayloadStoreOperation) processOperations(ctx context.Context) (err error) {
	defer func() {
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		op.setProcessDone(err)
	}()
	processor := replaceSecretPayload
	if op.initialize {
		processor = initializeSecretPayloadState
	}
	return op.so.ProcessOperations(ctx, true, processor)
}

func (op *secretPayloadStoreOperation) waitOperation(ctx context.Context) (err error) {
	localID, err := op.waitLocalID(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil && !errors.Is(err, context.Canceled) {
			op.setErr(err)
			return
		}
		if err == nil {
			op.setWaitDone()
		}
	}()
	_, rejected, err := op.so.WaitOperation(ctx, localID)
	if rejected && err == nil {
		err = errors.New("secret payload operation rejected")
	}
	return err
}

func (op *secretPayloadStoreOperation) waitLocalID(ctx context.Context) (string, error) {
	var localID string
	err := op.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		localID = op.localID
		return localID != "", nil
	})
	return localID, err
}

func (op *secretPayloadStoreOperation) watchPayload(ctx context.Context) (err error) {
	defer func() {
		if err != nil && !errors.Is(err, context.Canceled) {
			op.setErr(err)
		}
	}()

	var current sobject.SharedObjectStateSnapshot
	for {
		next, err := op.stateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return err
		}
		current = next
		payload, err := ReadSecretPayloadFromSnapshot(ctx, next)
		if err != nil {
			continue
		}
		if payload.EqualVT(op.expected) {
			op.setStored()
			return nil
		}
		if op.initialize && len(payload.GetPayloadIdentity()) != 0 {
			return errors.New("secret payload already initialized by a different request")
		}
	}
}

func (op *secretPayloadStoreOperation) waitStored(ctx context.Context) error {
	return op.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		if op.stored && op.waitDone {
			return true, nil
		}
		if op.err != nil {
			return true, op.err
		}
		return false, nil
	})
}

func (op *secretPayloadStoreOperation) stop() {
	waitRoutineStopped(op.state)
	waitRoutineStopped(op.waiter)
	waitRoutineStopped(op.processor)
}

func (op *secretPayloadStoreOperation) setStored() {
	op.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		op.stored = true
		broadcast()
	})
}

func (op *secretPayloadStoreOperation) setProcessDone(err error) {
	op.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		op.processErr = err
		if err != nil {
			op.err = err
		}
		broadcast()
	})
}

func (op *secretPayloadStoreOperation) setWaitDone() {
	op.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		op.waitDone = true
		broadcast()
	})
}

func (op *secretPayloadStoreOperation) setErr(err error) {
	op.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if err != nil {
			op.err = err
			broadcast()
		}
	})
}

func (op *secretPayloadStoreOperation) getProcessErr() error {
	var err error
	op.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		err = op.processErr
	})
	return err
}

func waitRoutineStopped(rc *routine.RoutineContainer) {
	if rc == nil {
		return
	}
	waitCh, _ := rc.SetRoutine(nil)
	if waitCh != nil {
		<-waitCh
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

func newSecretPayload(value []byte, contentType string, ts time.Time) *SecretPayload {
	if ts.IsZero() {
		ts = time.Now()
	}
	return &SecretPayload{
		Value:       append([]byte(nil), value...),
		ContentType: contentType,
		Version:     1,
		UpdatedAt:   timestamppb.New(ts),
	}
}

// _ is a type assertion
var _ SRPCSecretResourceServiceServer = (*SecretResource)(nil)
