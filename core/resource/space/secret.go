package resource_space

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// CreateSecret creates or reconciles a Secret object and grants an optional reader key.
func (r *SpaceResource) CreateSecret(
	ctx context.Context,
	req *s4wave_space.CreateSecretRequest,
) (*s4wave_space.CreateSecretResponse, error) {
	if err := r.checkSecretMutationAuthority(ctx); err != nil {
		return nil, err
	}
	if req.GetObjectKey() == "" {
		return nil, errors.New("object_key cannot be empty")
	}
	if req.GetKind() == "" {
		return nil, errors.New("kind cannot be empty")
	}
	if req.GetReconcileExisting() {
		if len(req.GetCreationToken()) < 16 {
			return nil, errors.New("creation_token must contain at least 16 unpredictable bytes")
		}
		if len(req.GetPayloadIdentity()) < 16 {
			return nil, errors.New("payload_identity must contain at least 16 unpredictable bytes")
		}
		if req.GetTimestamp() == nil {
			return nil, errors.New("timestamp is required for reconciliation")
		}
		if err := req.GetTimestamp().Validate(false); err != nil {
			return nil, errors.Wrap(err, "validate timestamp")
		}
	}

	ref := r.space.GetSharedObjectRef()
	if ref == nil || ref.GetProviderResourceRef() == nil {
		return nil, errors.New("space shared object ref is missing")
	}

	var readerPeerID string
	var readerPub crypto.PubKey
	if len(req.GetReaderPublicKeyPem()) != 0 {
		pub, err := keypem.ParsePubKeyPem(req.GetReaderPublicKeyPem())
		if err != nil {
			return nil, errors.Wrap(err, "parse reader public key PEM")
		}
		if pub == nil {
			return nil, errors.New("reader public key PEM did not contain a public key")
		}
		peerID, err := peer.IDFromPublicKey(pub)
		if err != nil {
			return nil, errors.Wrap(err, "derive reader peer id")
		}
		readerPeerID = peerID.String()
		readerPub = pub
	}

	nestedID := req.GetNestedSharedObjectId()
	if req.GetReconcileExisting() {
		desiredNestedID := secretNestedSharedObjectID(req.GetCreationToken(), req.GetObjectKey())
		if nestedID != "" && nestedID != desiredNestedID {
			return nil, errors.New("nested_shared_object_id does not match the creation identity")
		}
		nestedID = desiredNestedID
	}

	provRef := ref.GetProviderResourceRef()
	provAcc, relProvAcc, err := provider.ExAccessProviderAccount(
		ctx,
		r.b,
		provRef.GetProviderId(),
		provRef.GetProviderAccountId(),
		false,
		nil,
	)
	if err != nil {
		return nil, err
	}
	if provAcc == nil {
		return nil, errors.Errorf("provider account %s/%s not found", provRef.GetProviderId(), provRef.GetProviderAccountId())
	}
	defer relProvAcc.Release()

	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, err
	}
	ts := time.Now()
	if req.GetTimestamp() != nil {
		ts = req.GetTimestamp().AsTime()
	}
	secret, err := s4wave_secret.CreateSecret(ctx, r.b, soProvider, r.space.GetWorldEngine(), s4wave_secret.CreateSecretOptions{
		ObjectKey:            req.GetObjectKey(),
		DisplayName:          req.GetDisplayName(),
		Kind:                 req.GetKind(),
		ContentType:          req.GetContentType(),
		Value:                req.GetValue(),
		Timestamp:            ts,
		NestedSharedObjectId: nestedID,
		CreationToken:        req.GetCreationToken(),
		PayloadIdentity:      req.GetPayloadIdentity(),
	})
	if err != nil {
		return nil, err
	}
	if readerPeerID != "" {
		if _, err := s4wave_secret.AddSecretParticipant(
			ctx,
			r.b,
			secret,
			readerPeerID,
			readerPub,
			sobject.SOParticipantRole_SOParticipantRole_READER,
			"",
		); err != nil {
			return nil, errors.Wrap(err, "grant reader")
		}
	}
	return &s4wave_space.CreateSecretResponse{Secret: secret}, nil
}

func secretNestedSharedObjectID(creationToken []byte, objectKey string) string {
	h := sha256.New()
	_, _ = h.Write(creationToken)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(objectKey))
	return "secret-" + hex.EncodeToString(h.Sum(nil)[:28])
}

// DeleteSecret removes an exact token-owned nested payload before its parent.
func (r *SpaceResource) DeleteSecret(
	ctx context.Context,
	req *s4wave_space.DeleteSecretRequest,
) (*s4wave_space.DeleteSecretResponse, error) {
	if req.GetObjectKey() == "" || len(req.GetCreationToken()) < 16 {
		return nil, errors.New("object_key and creation_token are required")
	}
	nestedID := secretNestedSharedObjectID(req.GetCreationToken(), req.GetObjectKey())
	if req.GetNestedSharedObjectId() != "" && req.GetNestedSharedObjectId() != nestedID {
		return nil, errors.New("nested_shared_object_id does not match the creation identity")
	}
	if err := r.checkSecretMutationAuthority(ctx); err != nil {
		return nil, err
	}

	readTx, err := r.space.GetWorldEngine().NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	observed, err := verifySecretDeletionTarget(ctx, readTx, req.GetObjectKey(), nestedID, req.GetCreationToken())
	readTx.Discard()
	if err != nil {
		return nil, err
	}

	writeTx, err := r.space.GetWorldEngine().NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer writeTx.Discard()
	current, err := verifySecretDeletionTarget(ctx, writeTx, req.GetObjectKey(), nestedID, req.GetCreationToken())
	if err != nil {
		return nil, err
	}
	if (observed == nil) != (current == nil) || observed != nil && !observed.EqualVT(current) {
		return nil, errors.New("secret object identity changed before deletion")
	}
	deleted, err := writeTx.DeleteObject(ctx, req.GetObjectKey())
	if err != nil {
		return nil, err
	}

	spaceRef := r.space.GetSharedObjectRef()
	if spaceRef == nil || spaceRef.GetProviderResourceRef() == nil {
		return nil, errors.New("space shared object ref is missing")
	}
	ref := spaceRef.GetProviderResourceRef()
	provAcc, rel, err := provider.ExAccessProviderAccount(ctx, r.b, ref.GetProviderId(), ref.GetProviderAccountId(), false, nil)
	if err != nil {
		return nil, err
	}
	defer rel.Release()
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, err
	}
	nestedFound, nestedMatches, err := secretSharedObjectMatchesToken(ctx, soProvider, nestedID, req.GetCreationToken())
	if err != nil {
		return nil, err
	}
	if nestedFound && !nestedMatches {
		return nil, errors.New("nested secret ownership identity does not match")
	}
	if observed == nil && !nestedFound {
		return &s4wave_space.DeleteSecretResponse{}, nil
	}
	if nestedFound {
		if err := soProvider.DeleteSharedObject(ctx, nestedID); err != nil && !errors.Is(err, sobject.ErrSharedObjectNotFound) {
			return nil, errors.Wrap(err, "delete nested secret")
		}
	}
	if err := writeTx.Commit(ctx); err != nil {
		return nil, err
	}
	return &s4wave_space.DeleteSecretResponse{Deleted: deleted}, nil
}

func verifySecretDeletionTarget(
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	nestedID string,
	creationToken []byte,
) (*s4wave_secret.Secret, error) {
	secret, objRef, err := world.LookupObject[*s4wave_secret.Secret](ctx, ws, objectKey, s4wave_secret.NewSecretBlock)
	world.ReleaseObjectState(objRef)
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := world_types.CheckObjectType(ctx, ws, objectKey, s4wave_secret.SecretTypeID); err != nil {
		return nil, err
	}
	if !bytes.Equal(secret.GetCreationToken(), creationToken) || secret.GetNestedSharedObjectId() != nestedID {
		return nil, errors.New("secret ownership identity does not match")
	}
	return secret, nil
}

func secretSharedObjectMatchesToken(
	ctx context.Context,
	provider sobject.SharedObjectProvider,
	nestedID string,
	token []byte,
) (bool, bool, error) {
	ctr, release, err := provider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		return false, false, err
	}
	defer release()
	list, err := ctr.WaitValue(ctx, nil)
	if err != nil {
		return false, false, err
	}
	for _, entry := range list.GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == nestedID {
			return true, entry.GetMeta().GetBodyType() == s4wave_secret.SecretBodyType &&
				bytes.Equal(entry.GetMeta().GetBodyMeta(), token), nil
		}
	}
	return false, false, nil
}

func (r *SpaceResource) checkSecretMutationAuthority(ctx context.Context) error {
	so := r.space.GetSharedObject()
	if so == nil || r.sessionPeerID == "" || so.GetPeerID().String() != r.sessionPeerID {
		return errors.New("space mutation authority is unavailable")
	}
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	participant, err := snap.GetParticipantConfig(ctx)
	if err != nil {
		return err
	}
	if participant.GetRole() < sobject.SOParticipantRole_SOParticipantRole_WRITER {
		return errors.New("space mutation requires writer authority")
	}
	return nil
}

// ReadSecretPayload reads a Secret payload under the mounted session authority.
func (r *SpaceResource) ReadSecretPayload(
	ctx context.Context,
	req *s4wave_space.ReadSecretPayloadRequest,
) (*s4wave_space.ReadSecretPayloadResponse, error) {
	if req.GetObjectKey() == "" {
		return nil, errors.New("object_key cannot be empty")
	}
	if r.sessionPeerID == "" {
		return nil, errors.New("space session peer id is unavailable")
	}

	wtx, err := r.space.GetWorldEngine().NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer wtx.Discard()

	if err := world_types.CheckObjectType(ctx, wtx, req.GetObjectKey(), s4wave_secret.SecretTypeID); err != nil {
		return nil, err
	}
	secret, objRef, err := world.LookupObject[*s4wave_secret.Secret](ctx, wtx, req.GetObjectKey(), s4wave_secret.NewSecretBlock)
	world.ReleaseObjectState(objRef)
	if err != nil {
		return nil, err
	}
	payload, err := s4wave_secret.ReadSecretPayloadForPeer(
		ctx,
		r.b,
		secret,
		req.GetExpectedKind(),
		r.sessionPeerID,
	)
	if err != nil {
		return nil, err
	}
	return &s4wave_space.ReadSecretPayloadResponse{
		Secret:  secret.CloneVT(),
		Payload: payload,
	}, nil
}
