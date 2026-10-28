package resource_space

import (
	"context"

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

// CreateSecret creates a Secret object and grants an optional reader key.
func (r *SpaceResource) CreateSecret(
	ctx context.Context,
	req *s4wave_space.CreateSecretRequest,
) (*s4wave_space.CreateSecretResponse, error) {
	// Validate the Secret identity, kind, and mounted provider reference.
	if req.GetObjectKey() == "" {
		return nil, errors.New("object_key cannot be empty")
	}
	if req.GetKind() == "" {
		return nil, errors.New("kind cannot be empty")
	}
	ref := r.space.GetSharedObjectRef()
	if ref == nil || ref.GetProviderResourceRef() == nil {
		return nil, errors.New("space shared object ref is missing")
	}

	// Resolve the Secret participant role and optional reader public key.
	role := req.GetParticipantRole()
	if role == sobject.SOParticipantRole_SOParticipantRole_UNKNOWN {
		role = sobject.SOParticipantRole_SOParticipantRole_READER
	}
	if role != sobject.SOParticipantRole_SOParticipantRole_READER && role != sobject.SOParticipantRole_SOParticipantRole_WRITER {
		return nil, errors.New("Secret participant role must be reader or writer")
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

	// Acquire the provider account that will store the Secret.
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

	// Create the Secret through the provider shared object feature.
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		return nil, err
	}
	secret, err := s4wave_secret.CreateSecret(ctx, r.b, soProvider, r.space.GetWorldEngine(), s4wave_secret.CreateSecretOptions{
		ObjectKey:   req.GetObjectKey(),
		DisplayName: req.GetDisplayName(),
		Kind:        req.GetKind(),
		ContentType: req.GetContentType(),
		Value:       req.GetValue(),
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
			role,
			"",
		); err != nil {
			return nil, errors.Wrap(err, "grant reader")
		}
	}
	return &s4wave_space.CreateSecretResponse{Secret: secret}, nil
}

// ReadSecretPayload reads a Secret payload under the mounted session authority.
func (r *SpaceResource) ReadSecretPayload(
	ctx context.Context,
	req *s4wave_space.ReadSecretPayloadRequest,
) (*s4wave_space.ReadSecretPayloadResponse, error) {
	// Require the Secret identity and mounted Session authority.
	if req.GetObjectKey() == "" {
		return nil, errors.New("object_key cannot be empty")
	}
	if r.sessionPeerID == "" {
		return nil, errors.New("space session peer id is unavailable")
	}

	// Open a World snapshot for the Secret identity.
	wtx, err := r.space.GetWorldEngine().NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer wtx.Discard()

	// Read the typed Secret and its payload under the Session grant.
	if err := world_types.CheckObjectType(ctx, wtx, req.GetObjectKey(), s4wave_secret.SecretTypeID); err != nil {
		return nil, err
	}
	secret, err := world.LookupObjectBody[s4wave_secret.Secret](ctx, wtx, req.GetObjectKey())
	if err != nil {
		return nil, err
	}
	bound, err := r.bindSecret(ctx, secret)
	if err != nil {
		return nil, err
	}
	payload, err := s4wave_secret.ReadSecretPayloadForPeer(
		ctx,
		r.b,
		bound,
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

// WriteSecretPayload replaces a Secret payload under the mounted session writer grant.
func (r *SpaceResource) WriteSecretPayload(ctx context.Context, req *s4wave_space.WriteSecretPayloadRequest) (*s4wave_space.WriteSecretPayloadResponse, error) {
	// Read the existing Secret identity from the parent World.
	if req.GetObjectKey() == "" {
		return nil, errors.New("object_key cannot be empty")
	}
	tx, err := r.space.GetWorldEngine().NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	if err := world_types.CheckObjectType(ctx, tx, req.GetObjectKey(), s4wave_secret.SecretTypeID); err != nil {
		return nil, err
	}
	secret, err := world.LookupObjectBody[s4wave_secret.Secret](ctx, tx, req.GetObjectKey())
	if err != nil {
		return nil, err
	}

	// Delegate payload authority and persistence to the Secret component.
	bound, err := r.bindSecret(ctx, secret)
	if err != nil {
		return nil, err
	}
	if err := s4wave_secret.WriteSecretPayloadForPeer(ctx, r.b, bound, req.GetExpectedKind(), r.sessionPeerID, req.GetContentType(), req.GetValue()); err != nil {
		return nil, err
	}
	return &s4wave_space.WriteSecretPayloadResponse{}, nil
}

// AddSecretParticipant grants a peer access to an existing Secret, signed by
// the mounted session.
func (r *SpaceResource) AddSecretParticipant(ctx context.Context, req *s4wave_space.AddSecretParticipantRequest) (*s4wave_space.AddSecretParticipantResponse, error) {
	// Validate the Secret identity, target peer, and role.
	if req.GetObjectKey() == "" {
		return nil, errors.New("object_key cannot be empty")
	}
	peerID, err := peer.IDB58Decode(req.GetPeerId())
	if err != nil {
		return nil, errors.Wrap(err, "parse peer id")
	}
	pub, err := peerID.ExtractPublicKey()
	if err != nil {
		return nil, errors.Wrap(err, "extract peer public key")
	}
	role := req.GetParticipantRole()
	if role == sobject.SOParticipantRole_SOParticipantRole_UNKNOWN {
		role = sobject.SOParticipantRole_SOParticipantRole_READER
	}
	if role != sobject.SOParticipantRole_SOParticipantRole_READER && role != sobject.SOParticipantRole_SOParticipantRole_WRITER {
		return nil, errors.New("Secret participant role must be reader or writer")
	}

	// Read the existing Secret identity from the parent World.
	tx, err := r.space.GetWorldEngine().NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	if err := world_types.CheckObjectType(ctx, tx, req.GetObjectKey(), s4wave_secret.SecretTypeID); err != nil {
		return nil, err
	}
	secret, err := world.LookupObjectBody[s4wave_secret.Secret](ctx, tx, req.GetObjectKey())
	if err != nil {
		return nil, err
	}

	// Grant the peer on the Secret's nested SharedObject. A peer that is
	// already a participant keeps its role, so report it instead of success.
	bound, err := r.bindSecret(ctx, secret)
	if err != nil {
		return nil, err
	}
	grant, err := s4wave_secret.AddSecretParticipant(ctx, r.b, bound, peerID.String(), pub, role, "")
	if err != nil {
		return nil, errors.Wrap(err, "grant peer")
	}
	if grant == nil {
		return nil, errors.New("peer is already a Secret participant; its role is unchanged")
	}
	return &s4wave_space.AddSecretParticipantResponse{}, nil
}

// bindSecret returns secret with its nested ref in the account that holds the
// Space. A Secret another account created names that account, which this
// Session's provider does not hold, so the account enrolls into the nested
// object through the Space first. A ref from another provider is unchanged.
func (r *SpaceResource) bindSecret(ctx context.Context, secret *s4wave_secret.Secret) (*s4wave_secret.Secret, error) {
	// Keep a ref that already names the Space's account.
	spaceRef := r.space.GetSharedObjectRef().GetProviderResourceRef()
	nestedRef := secret.GetRef().GetProviderResourceRef()
	if spaceRef == nil || nestedRef == nil || nestedRef.GetProviderId() != spaceRef.GetProviderId() || nestedRef.GetProviderAccountId() == spaceRef.GetProviderAccountId() {
		return secret, nil
	}

	// Acquire the provider account that holds the Space.
	provAcc, relProvAcc, err := provider.ExAccessProviderAccount(ctx, r.b, spaceRef.GetProviderId(), spaceRef.GetProviderAccountId(), false, nil)
	if err != nil {
		return nil, err
	}
	if provAcc == nil {
		return nil, errors.Errorf("provider account %s/%s not found", spaceRef.GetProviderId(), spaceRef.GetProviderAccountId())
	}
	defer relProvAcc.Release()
	enroller, ok := provAcc.(sobject.ParticipantObjectEnroller)
	if !ok {
		return secret, nil
	}

	// Enroll into the nested object and read it under this account.
	ref, err := enroller.EnrollParticipantObject(ctx, nestedRef.GetId(), s4wave_secret.NewSharedObjectMeta(), spaceRef.GetId())
	if err != nil {
		return nil, errors.Wrap(err, "enroll in the Secret")
	}
	bound := secret.CloneVT()
	bound.Ref = ref
	return bound, nil
}
