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
	secret, err := s4wave_secret.CreateSecret(ctx, r.b, soProvider, r.space.GetWorldEngine(), s4wave_secret.CreateSecretOptions{
		ObjectKey:       req.GetObjectKey(),
		DisplayName:     req.GetDisplayName(),
		Kind:            req.GetKind(),
		ContentType:     req.GetContentType(),
		Value:           req.GetValue(),
		ReaderPeerID:    readerPeerID,
		ReaderPublicKey: readerPub,
	})
	if err != nil {
		return nil, err
	}
	return &s4wave_space.CreateSecretResponse{Secret: secret}, nil
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
	secret, err := world.LookupObjectBody[*s4wave_secret.Secret](ctx, wtx, req.GetObjectKey(), s4wave_secret.NewSecretBlock)
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
