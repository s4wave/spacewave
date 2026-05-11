package resource_space

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
)

func TestSpaceResourceCreateSecretCreatesGrantedSecret(t *testing.T) {
	ctx := t.Context()
	tb, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	readerPriv, readerPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	readerPubPEM, err := keypem.MarshalPubKeyPem(readerPub)
	if err != nil {
		t.Fatal(err)
	}
	readerPeerID, err := peer.IDFromPrivateKey(readerPriv)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{
		ObjectKey:          "secrets/matrix/access-token",
		DisplayName:        "Matrix access token",
		Kind:               s4wave_secret.SecretKindMatrixAccessToken,
		ContentType:        s4wave_secret.MatrixAccessTokenContentType,
		Value:              []byte("matrix-token"),
		ReaderPublicKeyPem: readerPubPEM,
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if resp.GetSecret().GetRef() == nil {
		t.Fatal("expected nested SharedObjectRef")
	}

	secretResource := s4wave_secret.NewSecretResource(tb.Logger, tb.Bus, tb.WorldState, "secrets/matrix/access-token")
	begin, err := secretResource.BeginReadPayload(ctx, &s4wave_secret.BeginReadPayloadRequest{
		ReaderPeerId: readerPeerID.String(),
		ExpectedKind: s4wave_secret.SecretKindMatrixAccessToken,
	})
	if err != nil {
		t.Fatalf("BeginReadPayload: %v", err)
	}
	signature, err := peer.NewSignature(
		s4wave_secret.ReadPayloadChallengeSignatureContext,
		readerPriv,
		hash.HashType_HashType_BLAKE3,
		begin.GetChallenge(),
		true,
	)
	if err != nil {
		t.Fatalf("NewSignature: %v", err)
	}
	read, err := secretResource.ReadPayload(ctx, &s4wave_secret.ReadPayloadRequest{
		ChallengeId: begin.GetChallengeId(),
		Signature:   signature,
	})
	if err != nil {
		t.Fatalf("ReadPayload: %v", err)
	}
	if got := string(read.GetPayload().GetValue()); got != "matrix-token" {
		t.Fatalf("payload mismatch: %q", got)
	}
}

type secretSpaceBody struct {
	ref    *sobject.SharedObjectRef
	engine world.Engine
}

func (b *secretSpaceBody) GetWorldEngine() world.Engine {
	return b.engine
}

func (b *secretSpaceBody) GetWorldEngineID() string {
	return "test-secret-space-engine"
}

func (b *secretSpaceBody) GetWorldEngineBucketID() string {
	return "test-secret-space-bucket"
}

func (b *secretSpaceBody) GetSharedObjectRef() *sobject.SharedObjectRef {
	return b.ref
}

func (b *secretSpaceBody) GetSharedObject() sobject.SharedObject {
	return nil
}

func setupSecretSpaceResourceTest(
	ctx context.Context,
	t *testing.T,
) (*testbed.Testbed, *SpaceResource, func()) {
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	providerID := "local"
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     tb.Volume.GetPeerID().String(),
	}), nil)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}

	accountID := "test-account-" + sobject.NewSOOperationLocalID()
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, accountID, false, nil)
	if err != nil {
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	spaceMeta, err := space.NewSharedObjectMeta("Secret Test Space")
	if err != nil {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}
	spaceRef, err := soProvider.CreateSharedObject(ctx, "space-"+sobject.NewSOOperationLocalID(), spaceMeta, "", "")
	if err != nil {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	resource := &SpaceResource{
		le:    tb.Logger,
		b:     tb.Bus,
		space: &secretSpaceBody{ref: spaceRef, engine: tb.BusEngine},
	}
	return tb, resource, func() {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
	}
}
