package s4wave_secret_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	spacewave_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	"github.com/s4wave/spacewave/testbed"
)

func TestCreateSecretStoresPayloadOnlyInNestedSharedObject(t *testing.T) {
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()

	token := "matrix-token-secret-value"
	secret, err := s4wave_secret.CreateSecret(ctx, tb.Bus, soProvider, tb.BusEngine, s4wave_secret.CreateSecretOptions{
		ObjectKey:   "secrets/matrix",
		DisplayName: "Matrix token",
		Kind:        s4wave_secret.SecretKindMatrixAccessToken,
		ContentType: s4wave_secret.MatrixAccessTokenContentType,
		Value:       []byte(token),
		Timestamp:   time.Unix(100, 0),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if secret.GetRef() == nil {
		t.Fatal("expected nested SharedObjectRef")
	}
	if secret.GetNestedSharedObjectId() == "" {
		t.Fatal("expected nested SharedObject id")
	}

	if err := world_types.CheckObjectType(ctx, tb.WorldState, "secrets/matrix", s4wave_secret.SecretTypeID); err != nil {
		t.Fatalf("CheckObjectType: %v", err)
	}

	parent := readParentSecret(ctx, t, tb.WorldState, "secrets/matrix")
	parentData, err := parent.MarshalVT()
	if err != nil {
		t.Fatalf("marshal parent: %v", err)
	}
	if bytes.Contains(parentData, []byte(token)) {
		t.Fatal("parent Secret block contains raw token bytes")
	}

	payload, err := s4wave_secret.ReadSecretPayload(ctx, tb.Bus, secret)
	if err != nil {
		t.Fatalf("ReadSecretPayload: %v", err)
	}
	if got := string(payload.GetValue()); got != token {
		t.Fatalf("payload value mismatch: %q", got)
	}
	readToken, err := s4wave_secret.ReadMatrixAccessToken(ctx, tb.Bus, secret)
	if err != nil {
		t.Fatalf("ReadMatrixAccessToken: %v", err)
	}
	if readToken != token {
		t.Fatalf("matrix token mismatch: %q", readToken)
	}
}

func TestSecretPayloadAccessUsesSharedObjectGrants(t *testing.T) {
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()

	value := []byte("grant-gated-secret")
	secret, err := s4wave_secret.CreateSecret(ctx, tb.Bus, soProvider, tb.BusEngine, s4wave_secret.CreateSecretOptions{
		ObjectKey:   "secrets/grants",
		DisplayName: "Grant gated",
		Kind:        "api_key",
		Value:       value,
		Timestamp:   time.Unix(200, 0),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	so, soRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, secret.GetRef(), false, nil)
	if err != nil {
		t.Fatalf("mount nested SO: %v", err)
	}
	ih, ok := so.(sobject.InviteHost)
	if !ok {
		t.Fatal("nested SO does not support participant mutation")
	}

	grantedPriv, grantedPub, grantedPeerID := makePeer(t)
	ungrantedPriv, _, ungrantedPeerID := makePeer(t)
	if _, err := s4wave_secret.AddSecretParticipant(
		ctx,
		tb.Bus,
		secret,
		grantedPeerID.String(),
		grantedPub,
		sobject.SOParticipantRole_SOParticipantRole_READER,
		"",
	); err != nil {
		t.Fatalf("AddSecretParticipant: %v", err)
	}
	soRef.Release()

	so, soRef, err = sobject.ExMountSharedObject(ctx, tb.Bus, secret.GetRef(), false, nil)
	if err != nil {
		t.Fatalf("remount nested SO after grant: %v", err)
	}
	ih, ok = so.(sobject.InviteHost)
	if !ok {
		t.Fatal("remounted nested SO does not support participant mutation")
	}

	state, err := ih.GetSOHost().GetHostState(ctx)
	if err != nil {
		t.Fatalf("GetHostState: %v", err)
	}
	grantedSnap := sobject.NewSOStateParticipantHandle(
		tb.Logger,
		tb.StepFactorySet,
		so.GetSharedObjectID(),
		state,
		grantedPriv,
		grantedPeerID,
	)
	grantedPayload, err := s4wave_secret.ReadSecretPayloadFromSnapshot(ctx, grantedSnap)
	if err != nil {
		t.Fatalf("granted ReadSecretPayloadFromSnapshot: %v", err)
	}
	if !bytes.Equal(grantedPayload.GetValue(), value) {
		t.Fatalf("granted payload mismatch: %q", grantedPayload.GetValue())
	}

	ungrantedSnap := sobject.NewSOStateParticipantHandle(
		tb.Logger,
		tb.StepFactorySet,
		so.GetSharedObjectID(),
		state,
		ungrantedPriv,
		ungrantedPeerID,
	)
	if _, err := s4wave_secret.ReadSecretPayloadFromSnapshot(ctx, ungrantedSnap); !errors.Is(err, s4wave_secret.ErrPayloadAccessDenied) {
		t.Fatalf("expected ungranted access denied, got %v", err)
	}

	removed, err := s4wave_secret.RemoveSecretParticipant(ctx, tb.Bus, secret, grantedPeerID.String(), nil)
	if err != nil {
		t.Fatalf("RemoveSecretParticipant: %v", err)
	}
	if !removed {
		t.Fatal("expected participant removal")
	}
	soRef.Release()

	so, soRef, err = sobject.ExMountSharedObject(ctx, tb.Bus, secret.GetRef(), false, nil)
	if err != nil {
		t.Fatalf("remount nested SO after revocation: %v", err)
	}
	defer soRef.Release()
	ih, ok = so.(sobject.InviteHost)
	if !ok {
		t.Fatal("revoked nested SO does not support participant mutation")
	}
	revokedState, err := ih.GetSOHost().GetHostState(ctx)
	if err != nil {
		t.Fatalf("GetHostState after removal: %v", err)
	}
	revokedSnap := sobject.NewSOStateParticipantHandle(
		tb.Logger,
		tb.StepFactorySet,
		so.GetSharedObjectID(),
		revokedState,
		grantedPriv,
		grantedPeerID,
	)
	if _, err := s4wave_secret.ReadSecretPayloadFromSnapshot(ctx, revokedSnap); !errors.Is(err, s4wave_secret.ErrPayloadAccessDenied) {
		t.Fatalf("expected revoked access denied, got %v", err)
	}
}

func TestSecretResourceReadPayloadRequiresSignedGrantedPeer(t *testing.T) {
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()

	value := []byte("resource-read-secret")
	secret, err := s4wave_secret.CreateSecret(ctx, tb.Bus, soProvider, tb.BusEngine, s4wave_secret.CreateSecretOptions{
		ObjectKey:   "secrets/resource-read",
		DisplayName: "Resource read",
		Kind:        "api_key",
		Value:       value,
		Timestamp:   time.Unix(300, 0),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	grantedPriv, grantedPub, grantedPeerID := makePeer(t)
	ungrantedPriv, _, ungrantedPeerID := makePeer(t)
	if _, err := s4wave_secret.AddSecretParticipant(
		ctx,
		tb.Bus,
		secret,
		grantedPeerID.String(),
		grantedPub,
		sobject.SOParticipantRole_SOParticipantRole_READER,
		"",
	); err != nil {
		t.Fatalf("AddSecretParticipant: %v", err)
	}

	res := s4wave_secret.NewSecretResource(tb.Logger, tb.Bus, tb.WorldState, "secrets/resource-read")
	if _, err := res.BeginReadPayload(ctx, &s4wave_secret.BeginReadPayloadRequest{
		ReaderPeerId: ungrantedPeerID.String(),
		ExpectedKind: "api_key",
	}); !errors.Is(err, s4wave_secret.ErrPayloadAccessDenied) {
		t.Fatalf("expected ungranted BeginReadPayload access denied, got %v", err)
	}
	if _, err := res.BeginReadPayload(ctx, &s4wave_secret.BeginReadPayloadRequest{
		ReaderPeerId: grantedPeerID.String(),
		ExpectedKind: "wrong-kind",
	}); !errors.Is(err, s4wave_secret.ErrSecretKindMismatch) {
		t.Fatalf("expected kind mismatch, got %v", err)
	}

	begin, err := res.BeginReadPayload(ctx, &s4wave_secret.BeginReadPayloadRequest{
		ReaderPeerId: grantedPeerID.String(),
		ExpectedKind: "api_key",
	})
	if err != nil {
		t.Fatalf("BeginReadPayload: %v", err)
	}
	sig, err := peer.NewSignature(
		s4wave_secret.ReadPayloadChallengeSignatureContext,
		grantedPriv,
		hash.HashType_HashType_BLAKE3,
		begin.GetChallenge(),
		true,
	)
	if err != nil {
		t.Fatalf("NewSignature: %v", err)
	}
	read, err := res.ReadPayload(ctx, &s4wave_secret.ReadPayloadRequest{
		ChallengeId: begin.GetChallengeId(),
		Signature:   sig,
	})
	if err != nil {
		t.Fatalf("ReadPayload: %v", err)
	}
	if !bytes.Equal(read.GetPayload().GetValue(), value) {
		t.Fatalf("payload mismatch: %q", read.GetPayload().GetValue())
	}
	if _, err := res.ReadPayload(ctx, &s4wave_secret.ReadPayloadRequest{
		ChallengeId: begin.GetChallengeId(),
		Signature:   sig,
	}); !errors.Is(err, s4wave_secret.ErrReadChallengeNotFound) {
		t.Fatalf("expected replay failure, got %v", err)
	}

	begin, err = res.BeginReadPayload(ctx, &s4wave_secret.BeginReadPayloadRequest{
		ReaderPeerId: grantedPeerID.String(),
		ExpectedKind: "api_key",
	})
	if err != nil {
		t.Fatalf("BeginReadPayload before revocation: %v", err)
	}
	sig, err = peer.NewSignature(
		s4wave_secret.ReadPayloadChallengeSignatureContext,
		grantedPriv,
		hash.HashType_HashType_BLAKE3,
		begin.GetChallenge(),
		true,
	)
	if err != nil {
		t.Fatalf("NewSignature before revocation: %v", err)
	}
	removed, err := s4wave_secret.RemoveSecretParticipant(ctx, tb.Bus, secret, grantedPeerID.String(), nil)
	if err != nil {
		t.Fatalf("RemoveSecretParticipant: %v", err)
	}
	if !removed {
		t.Fatal("expected participant removal")
	}
	if _, err := res.ReadPayload(ctx, &s4wave_secret.ReadPayloadRequest{
		ChallengeId: begin.GetChallengeId(),
		Signature:   sig,
	}); !errors.Is(err, s4wave_secret.ErrPayloadAccessDenied) {
		t.Fatalf("expected revoked read access denied, got %v", err)
	}

	begin, err = res.BeginReadPayload(ctx, &s4wave_secret.BeginReadPayloadRequest{
		ReaderPeerId: ungrantedPeerID.String(),
	})
	if !errors.Is(err, s4wave_secret.ErrPayloadAccessDenied) {
		t.Fatalf("expected ungranted access denied after revocation, got begin=%v err=%v", begin, err)
	}
	badSig, err := peer.NewSignature(
		s4wave_secret.ReadPayloadChallengeSignatureContext,
		ungrantedPriv,
		hash.HashType_HashType_BLAKE3,
		[]byte("not the issued challenge"),
		true,
	)
	if err != nil {
		t.Fatalf("bad NewSignature: %v", err)
	}
	if _, err := res.ReadPayload(ctx, &s4wave_secret.ReadPayloadRequest{
		ChallengeId: "missing",
		Signature:   badSig,
	}); !errors.Is(err, s4wave_secret.ErrReadChallengeNotFound) {
		t.Fatalf("expected missing challenge failure, got %v", err)
	}
}

func setupSecretTest(
	ctx context.Context,
	t *testing.T,
) (*testbed.Testbed, sobject.SharedObjectProvider, func()) {
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

	return tb, soProvider, func() {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
	}
}

func readParentSecret(
	ctx context.Context,
	t *testing.T,
	ws world.WorldState,
	objectKey string,
) *s4wave_secret.Secret {
	t.Helper()
	obj, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("parent Secret object not found")
	}
	var secret *s4wave_secret.Secret
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		secret, err = s4wave_secret.UnmarshalSecret(ctx, bcs)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

func makePeer(t *testing.T) (spacewave_crypto.PrivKey, spacewave_crypto.PubKey, peer.ID) {
	t.Helper()
	priv, pub, err := spacewave_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub, peerID
}
