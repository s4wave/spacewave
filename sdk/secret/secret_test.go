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

// TestCreateSecretStoresPayloadOnlyInNestedSharedObject checks that a payload
// lives in its nested shared object, never in the parent World.
func TestCreateSecretStoresPayloadOnlyInNestedSharedObject(t *testing.T) {
	// Start a local testbed for nested Secret storage.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()

	// Store a provider credential and verify its parent metadata.
	token := "provider-credential-secret-value"
	secret, err := s4wave_secret.CreateSecret(ctx, tb.Bus, soProvider, tb.BusEngine, s4wave_secret.CreateSecretOptions{
		ObjectKey:   "secrets/provider/opencode-go",
		DisplayName: "OpenCode Go credential",
		Kind:        s4wave_secret.SecretKindProviderCredential,
		ContentType: s4wave_secret.ProviderCredentialContentType,
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

	// Confirm the parent record uses the Secret object type.
	if err := world_types.CheckObjectType(ctx, tb.WorldState, "secrets/provider/opencode-go", s4wave_secret.SecretTypeID); err != nil {
		t.Fatalf("CheckObjectType: %v", err)
	}

	// Ensure the parent block does not expose the credential bytes.
	parent := readParentSecret(ctx, t, tb.WorldState, "secrets/provider/opencode-go")
	parentData, err := parent.MarshalVT()
	if err != nil {
		t.Fatalf("marshal parent: %v", err)
	}
	if bytes.Contains(parentData, []byte(token)) {
		t.Fatal("parent Secret block contains raw token bytes")
	}

	// Read the nested payload through both Secret read APIs.
	payload, err := s4wave_secret.ReadSecretPayload(ctx, tb.Bus, secret)
	if err != nil {
		t.Fatalf("ReadSecretPayload: %v", err)
	}
	if got := string(payload.GetValue()); got != token {
		t.Fatalf("payload value mismatch: %q", got)
	}
	readCredential, err := s4wave_secret.ReadProviderCredentialPayload(ctx, tb.Bus, secret)
	if err != nil {
		t.Fatalf("ReadProviderCredentialPayload: %v", err)
	}
	if string(readCredential) != token {
		t.Fatalf("provider credential mismatch: %q", readCredential)
	}
}

// TestSSHSecretContractStoresCredentialPayloadOnlyInNestedSharedObject checks
// the same for an SSH credential.
func TestSSHSecretContractStoresCredentialPayloadOnlyInNestedSharedObject(t *testing.T) {
	// Start a local testbed for SSH Secret storage.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()

	// Confirm the SSH credential constructor preserves key bytes and kind.
	privateKey := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nspacewave-secret\n-----END OPENSSH PRIVATE KEY-----")
	payload := s4wave_secret.NewSSHPrivateKeyPayload(privateKey, time.Unix(150, 0))
	if payload.GetContentType() != s4wave_secret.SSHPrivateKeyContentType {
		t.Fatalf("content type = %q, want %q", payload.GetContentType(), s4wave_secret.SSHPrivateKeyContentType)
	}
	if !bytes.Equal(payload.GetValue(), privateKey) {
		t.Fatal("private key payload value mismatch")
	}

	// Store the SSH private key in its nested SharedObject.
	secret, err := s4wave_secret.CreateSecret(ctx, tb.Bus, soProvider, tb.BusEngine, s4wave_secret.CreateSecretOptions{
		ObjectKey:   "secrets/ssh/private-key",
		DisplayName: "SSH private key",
		Kind:        s4wave_secret.SecretKindSSHPrivateKey,
		ContentType: s4wave_secret.SSHPrivateKeyContentType,
		Value:       privateKey,
		Timestamp:   time.Unix(150, 0),
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	// Ensure the parent Secret block omits the private key bytes.
	parent := readParentSecret(ctx, t, tb.WorldState, "secrets/ssh/private-key")
	parentData, err := parent.MarshalVT()
	if err != nil {
		t.Fatalf("marshal parent: %v", err)
	}
	if bytes.Contains(parentData, privateKey) {
		t.Fatal("parent Secret block contains raw SSH private key bytes")
	}

	// Read the private key and reject a request for another credential kind.
	readPrivateKey, err := s4wave_secret.ReadSSHCredentialPayload(ctx, tb.Bus, secret, s4wave_secret.SecretKindSSHPrivateKey)
	if err != nil {
		t.Fatalf("ReadSSHCredentialPayload: %v", err)
	}
	if !bytes.Equal(readPrivateKey, privateKey) {
		t.Fatal("read private key payload mismatch")
	}
	if _, err := s4wave_secret.ReadSSHCredentialPayload(ctx, tb.Bus, secret, s4wave_secret.SecretKindSSHPassword); !errors.Is(err, s4wave_secret.ErrSecretKindMismatch) {
		t.Fatalf("expected SSH kind mismatch, got %v", err)
	}

	// Confirm text-based SSH credentials share the text content type.
	passwordPayload := s4wave_secret.NewSSHPasswordPayload("hunter2", time.Unix(151, 0))
	if passwordPayload.GetContentType() != s4wave_secret.SSHTextCredentialContentType {
		t.Fatalf("password content type = %q", passwordPayload.GetContentType())
	}
	passphrasePayload := s4wave_secret.NewSSHPassphrasePayload("key-passphrase", time.Unix(152, 0))
	if passphrasePayload.GetContentType() != s4wave_secret.SSHTextCredentialContentType {
		t.Fatalf("passphrase content type = %q", passphrasePayload.GetContentType())
	}
}

// TestSecretPayloadAccessUsesSharedObjectGrants checks that only a peer granted
// on the nested object reads the payload, and only until it is removed.
func TestSecretPayloadAccessUsesSharedObjectGrants(t *testing.T) {
	// Create a secret in a fresh provider.
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

	// The nested object accepts participant changes.
	so, soRef, err := sobject.ExMountSharedObject(ctx, tb.Bus, secret.GetRef(), false, nil)
	if err != nil {
		t.Fatalf("mount nested SO: %v", err)
	}
	if _, ok := so.(sobject.InviteHost); !ok {
		t.Fatal("nested SO does not support participant mutation")
	}

	// Grant one peer read access and leave another ungranted.
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

	// Remount the nested object to observe the grant.
	so, soRef, err = sobject.ExMountSharedObject(ctx, tb.Bus, secret.GetRef(), false, nil)
	if err != nil {
		t.Fatalf("remount nested SO after grant: %v", err)
	}
	ih, ok := so.(sobject.InviteHost)
	if !ok {
		t.Fatal("remounted nested SO does not support participant mutation")
	}

	// The granted peer reads the payload from the host state.
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
	).WithConfigHistory(ih.GetSOHost().ReadConfigEntry)
	grantedPayload, err := s4wave_secret.ReadSecretPayloadFromSnapshot(ctx, grantedSnap)
	if err != nil {
		t.Fatalf("granted ReadSecretPayloadFromSnapshot: %v", err)
	}
	if !bytes.Equal(grantedPayload.GetValue(), value) {
		t.Fatalf("granted payload mismatch: %q", grantedPayload.GetValue())
	}

	// The ungranted peer is denied.
	ungrantedSnap := sobject.NewSOStateParticipantHandle(
		tb.Logger,
		tb.StepFactorySet,
		so.GetSharedObjectID(),
		state,
		ungrantedPriv,
		ungrantedPeerID,
	).WithConfigHistory(ih.GetSOHost().ReadConfigEntry)
	if _, err := s4wave_secret.ReadSecretPayloadFromSnapshot(ctx, ungrantedSnap); !errors.Is(err, s4wave_secret.ErrPayloadAccessDenied) {
		t.Fatalf("expected ungranted access denied, got %v", err)
	}

	// Remove the granted peer.
	removed, err := s4wave_secret.RemoveSecretParticipant(ctx, tb.Bus, secret, grantedPeerID.String(), nil)
	if err != nil {
		t.Fatalf("RemoveSecretParticipant: %v", err)
	}
	if !removed {
		t.Fatal("expected participant removal")
	}
	soRef.Release()

	// Remount the nested object to observe the removal.
	so, soRef, err = sobject.ExMountSharedObject(ctx, tb.Bus, secret.GetRef(), false, nil)
	if err != nil {
		t.Fatalf("remount nested SO after revocation: %v", err)
	}
	defer soRef.Release()
	ih, ok = so.(sobject.InviteHost)
	if !ok {
		t.Fatal("revoked nested SO does not support participant mutation")
	}

	// The removed peer is denied under the new host state.
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
	).WithConfigHistory(ih.GetSOHost().ReadConfigEntry)
	if _, err := s4wave_secret.ReadSecretPayloadFromSnapshot(ctx, revokedSnap); !errors.Is(err, s4wave_secret.ErrPayloadAccessDenied) {
		t.Fatalf("expected revoked access denied, got %v", err)
	}
}

// TestSecretResourceReadPayloadRequiresSignedGrantedPeer checks that the Secret
// resource reads a payload only for a granted peer that signs its challenge.
func TestSecretResourceReadPayloadRequiresSignedGrantedPeer(t *testing.T) {
	// Start the testbed for the Secret resource read path.
	ctx := t.Context()
	tb, soProvider, release := setupSecretTest(ctx, t)
	defer release()

	// Create the payload whose bytes the resource will protect.
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

	// Grant one peer read access and retain another as an unauthorized caller.
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

	// Reject unauthorized and mismatched-kind challenge requests.
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

	// Sign a valid challenge and read the granted payload once.
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

	// Issue another challenge, then revoke its reader before payload access.
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

	// Reject new challenges and signatures from ungranted peers.
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

// setupSecretTest starts a testbed with a local shared object provider.
func setupSecretTest(
	ctx context.Context,
	t *testing.T,
) (*testbed.Testbed, sobject.SharedObjectProvider, func()) {
	// Start the real in-memory testbed and release it on failure.
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Register a local provider for the test peer.
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

	// Open the provider account and obtain its SharedObject feature.
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

// readParentSecret reads the Secret at objectKey in ws.
func readParentSecret(
	ctx context.Context,
	t *testing.T,
	ws world.WorldState,
	objectKey string,
) *s4wave_secret.Secret {
	// Load the parent World object while retaining its state for decoding.
	t.Helper()
	obj, found, err := ws.GetObject(ctx, objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("parent Secret object not found")
	}

	// Decode the Secret block from the parent object root.
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

// makePeer generates an Ed25519 peer.
func makePeer(t *testing.T) (spacewave_crypto.PrivKey, spacewave_crypto.PubKey, peer.ID) {
	// Generate the peer's key pair and derive its network identity.
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
