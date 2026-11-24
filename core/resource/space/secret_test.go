package resource_space

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
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
	// Mount a provider-backed Space for Secret creation.
	ctx := t.Context()
	tb, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	// Generate the reader identity and its public key grant.
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

	// Create a Secret with the generated reader grant.
	resp, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{
		ObjectKey:          "secrets/provider/access-token",
		DisplayName:        "Provider credential",
		Kind:               s4wave_secret.SecretKindProviderCredential,
		ContentType:        s4wave_secret.ProviderCredentialContentType,
		Value:              []byte(`{"api_key":"opencode-go"}`),
		ReaderPublicKeyPem: readerPubPEM,
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if resp.GetSecret().GetRef() == nil {
		t.Fatal("expected nested SharedObjectRef")
	}

	// Read the Secret through its signed payload challenge.
	secretResource := s4wave_secret.NewSecretResource(tb.Logger, tb.Bus, tb.WorldState, "secrets/provider/access-token")
	begin, err := secretResource.BeginReadPayload(ctx, &s4wave_secret.BeginReadPayloadRequest{
		ReaderPeerId: readerPeerID.String(),
		ExpectedKind: s4wave_secret.SecretKindProviderCredential,
	})
	if err != nil {
		t.Fatalf("BeginReadPayload: %v", err)
	}

	// Sign the Secret payload challenge with the granted reader key.
	signature, err := peer.NewSignature(
		s4wave_secret.ReadPayloadChallengeSignatureContext,
		readerPriv,
		hash.HashType_HashType_SHA256,
		begin.GetChallenge(),
		true,
	)
	if err != nil {
		t.Fatalf("NewSignature: %v", err)
	}

	// Read the payload with the granted reader proof.
	read, err := secretResource.ReadPayload(ctx, &s4wave_secret.ReadPayloadRequest{
		ChallengeId: begin.GetChallengeId(),
		Signature:   signature,
	})
	if err != nil {
		t.Fatalf("ReadPayload: %v", err)
	}

	// Verify the signed read returns the saved Secret payload.
	if got := string(read.GetPayload().GetValue()); got != `{"api_key":"opencode-go"}` {
		t.Fatalf("payload mismatch: %q", got)
	}
}

func TestSpaceResourceReadSecretPayloadUsesMountedSessionGrant(t *testing.T) {
	// Mount a provider-backed Space under its Session signer.
	ctx := t.Context()
	tb, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	// Grant the mounted Session reader access to a Secret.
	readerPub, err := tb.Volume.GetPeerID().ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	readerPubPEM, err := keypem.MarshalPubKeyPem(readerPub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{
		ObjectKey:          "secrets/provider/session-token",
		DisplayName:        "Provider credential",
		Kind:               s4wave_secret.SecretKindProviderCredential,
		ContentType:        s4wave_secret.ProviderCredentialContentType,
		Value:              []byte(`{"api_key":"session-token"}`),
		ReaderPublicKeyPem: readerPubPEM,
	}); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	// Read the Secret payload under the mounted Session grant.
	read, err := resource.ReadSecretPayload(ctx, &s4wave_space.ReadSecretPayloadRequest{
		ObjectKey:    "secrets/provider/session-token",
		ExpectedKind: s4wave_secret.SecretKindProviderCredential,
	})
	if err != nil {
		t.Fatalf("ReadSecretPayload: %v", err)
	}
	if read.GetSecret().GetKind() != s4wave_secret.SecretKindProviderCredential {
		t.Fatalf("secret kind: got %q", read.GetSecret().GetKind())
	}
	if got := string(read.GetPayload().GetValue()); got != `{"api_key":"session-token"}` {
		t.Fatalf("payload mismatch: %q", got)
	}
	if _, err := resource.ReadSecretPayload(ctx, &s4wave_space.ReadSecretPayloadRequest{
		ObjectKey:    "secrets/provider/session-token",
		ExpectedKind: "other",
	}); !errors.Is(err, s4wave_secret.ErrSecretKindMismatch) {
		t.Fatalf("expected kind mismatch, got %v", err)
	}

	// A plugin this Space loaded reads; one bound to another Space is refused.
	ref := resource.space.GetSharedObjectRef()
	own := bldr_plugin.WithPluginCaller(ctx, &bldr_plugin.PluginCaller{PluginId: "glados-core", BindingKey: space.SpaceEngineId(ref)})
	if _, err := resource.ReadSecretPayload(own, &s4wave_space.ReadSecretPayloadRequest{
		ObjectKey:    "secrets/provider/session-token",
		ExpectedKind: s4wave_secret.SecretKindProviderCredential,
	}); err != nil {
		t.Fatalf("ReadSecretPayload as this Space's plugin: %v", err)
	}
	otherRef := ref.CloneVT()
	otherRef.ProviderResourceRef.Id = "other-space"
	other := bldr_plugin.WithPluginCaller(ctx, &bldr_plugin.PluginCaller{PluginId: "glados-core", BindingKey: space.SpaceEngineId(otherRef)})
	if _, err := resource.ReadSecretPayload(other, &s4wave_space.ReadSecretPayloadRequest{
		ObjectKey:    "secrets/provider/session-token",
		ExpectedKind: s4wave_secret.SecretKindProviderCredential,
	}); err == nil || !strings.Contains(err.Error(), "not bound to this Space") {
		t.Fatalf("expected another Space's plugin refused, got %v", err)
	}

	// Verify another Session identity cannot use the saved reader grant.
	otherPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherPeerID, err := peer.IDFromPrivateKey(otherPriv)
	if err != nil {
		t.Fatal(err)
	}
	resource.sessionPeerID = otherPeerID.String()
	if _, err := resource.ReadSecretPayload(ctx, &s4wave_space.ReadSecretPayloadRequest{
		ObjectKey:    "secrets/provider/session-token",
		ExpectedKind: s4wave_secret.SecretKindProviderCredential,
	}); !errors.Is(err, s4wave_secret.ErrPayloadAccessDenied) {
		t.Fatalf("expected payload access denied, got %v", err)
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
	// Start the World testbed for the provider-backed Secret Space.
	t.Helper()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Register and start the local SharedObject provider.
	providerID := "local"
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	_, provCtrlRef, err := tb.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     tb.Volume.GetPeerID().String(),
	}), nil)
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}

	// Open the provider account and create the parent Space SharedObject.
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

	// Create the parent Space SharedObject under the provider account.
	spaceRef, err := soProvider.CreateSharedObject(ctx, "space-"+sobject.NewSOOperationLocalID(), spaceMeta, "", "")
	if err != nil {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
		t.Fatal(err)
	}

	// Expose the provider-backed Space with its mounted Session signer.
	resource := &SpaceResource{
		le:            tb.Logger,
		b:             tb.Bus,
		space:         &secretSpaceBody{ref: spaceRef, engine: tb.Engine},
		sessionPeerID: tb.Volume.GetPeerID().String(),
	}
	return tb, resource, func() {
		provAccRef.Release()
		provCtrlRef.Release()
		tb.Release()
	}
}

// TestSpaceResourceWritesSecretWithoutChangingGrants covers writer-only replacement.
func TestSpaceResourceWritesSecretWithoutChangingGrants(t *testing.T) {
	// Mount a provider-backed Space for writer grant replacement.
	ctx := t.Context()
	tb, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	// Encode the mounted Session public key for the writer grant.
	pub, err := tb.Volume.GetPeerID().ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	pem, err := keypem.MarshalPubKeyPem(pub)
	if err != nil {
		t.Fatal(err)
	}

	// Create a Secret whose mounted Session holds a writer grant.
	created, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{ObjectKey: "secrets/rotation", Kind: s4wave_secret.SecretKindProviderCredential, Value: []byte("old"), ReaderPublicKeyPem: pem, ParticipantRole: sobject.SOParticipantRole_SOParticipantRole_WRITER})
	if err != nil {
		t.Fatal(err)
	}

	// Replace the Secret payload under the mounted writer grant.
	_, err = resource.WriteSecretPayload(ctx, &s4wave_space.WriteSecretPayloadRequest{ObjectKey: "secrets/rotation", ExpectedKind: s4wave_secret.SecretKindProviderCredential, ContentType: "application/json", Value: []byte("new")})
	if err != nil {
		t.Fatal(err)
	}

	// Read the replacement and verify the Secret identity is unchanged.
	read, err := resource.ReadSecretPayload(ctx, &s4wave_space.ReadSecretPayloadRequest{ObjectKey: "secrets/rotation", ExpectedKind: s4wave_secret.SecretKindProviderCredential})
	if err != nil {
		t.Fatal(err)
	}
	if string(read.GetPayload().GetValue()) != "new" || !created.GetSecret().GetRef().EqualVT(read.GetSecret().GetRef()) {
		t.Fatal("replacement changed identity or failed to persist")
	}

	// Read access never authorizes credential replacement.
	_, pub, err = crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s4wave_secret.AddSecretParticipant(ctx, tb.Bus, created.GetSecret(), reader.String(), pub, sobject.SOParticipantRole_SOParticipantRole_READER, ""); err != nil {
		t.Fatal(err)
	}
	resource.sessionPeerID = reader.String()
	_, err = resource.WriteSecretPayload(ctx, &s4wave_space.WriteSecretPayloadRequest{ObjectKey: "secrets/rotation", Value: []byte("forbidden")})
	if !errors.Is(err, s4wave_secret.ErrPayloadAccessDenied) || !strings.Contains(err.Error(), "is not a writer") {
		t.Fatalf("reader write: %v", err)
	}
}

// TestSpaceResourceAddSecretParticipantGrantsPeer covers granting and raising
// a second peer, such as a Device, on an existing Secret.
func TestSpaceResourceAddSecretParticipantGrantsPeer(t *testing.T) {
	// Mount a provider-backed Space and create a Secret for the Session.
	ctx := t.Context()
	_, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()
	if _, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{ObjectKey: "secrets/device", Kind: s4wave_secret.SecretKindProviderCredential, Value: []byte("old")}); err != nil {
		t.Fatal(err)
	}

	// Make a new peer to grant.
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	device, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	// Grant it read access, then raise it to writer.
	reader := sobject.SOParticipantRole_SOParticipantRole_READER
	writer := sobject.SOParticipantRole_SOParticipantRole_WRITER
	added, err := resource.AddSecretParticipant(ctx, &s4wave_space.AddSecretParticipantRequest{ObjectKey: "secrets/device", PeerId: device.String(), ParticipantRole: reader})
	if err != nil {
		t.Fatalf("AddSecretParticipant: %v", err)
	}
	if added.GetOutcome() != sobject.SOParticipantAddOutcome_SOParticipantAddOutcome_ADDED || added.GetParticipantRole() != reader {
		t.Fatalf("grant: outcome %v role %v", added.GetOutcome(), added.GetParticipantRole())
	}
	raised, err := resource.AddSecretParticipant(ctx, &s4wave_space.AddSecretParticipantRequest{ObjectKey: "secrets/device", PeerId: device.String(), ParticipantRole: writer})
	if err != nil {
		t.Fatalf("raise AddSecretParticipant: %v", err)
	}
	if raised.GetOutcome() != sobject.SOParticipantAddOutcome_SOParticipantAddOutcome_RAISED || raised.GetParticipantRole() != writer {
		t.Fatalf("raise: outcome %v role %v", raised.GetOutcome(), raised.GetParticipantRole())
	}

	// The granted peer reads and replaces the payload.
	resource.sessionPeerID = device.String()
	if _, err := resource.WriteSecretPayload(ctx, &s4wave_space.WriteSecretPayloadRequest{ObjectKey: "secrets/device", ExpectedKind: s4wave_secret.SecretKindProviderCredential, Value: []byte("new")}); err != nil {
		t.Fatalf("WriteSecretPayload: %v", err)
	}
	read, err := resource.ReadSecretPayload(ctx, &s4wave_space.ReadSecretPayloadRequest{ObjectKey: "secrets/device", ExpectedKind: s4wave_secret.SecretKindProviderCredential})
	if err != nil {
		t.Fatalf("ReadSecretPayload: %v", err)
	}
	if got := string(read.GetPayload().GetValue()); got != "new" {
		t.Fatalf("payload mismatch: %q", got)
	}

	// A lower grant to the same peer leaves the role it holds.
	again, err := resource.AddSecretParticipant(ctx, &s4wave_space.AddSecretParticipantRequest{ObjectKey: "secrets/device", PeerId: device.String(), ParticipantRole: reader})
	if err != nil {
		t.Fatalf("repeated AddSecretParticipant: %v", err)
	}
	if again.GetOutcome() != sobject.SOParticipantAddOutcome_SOParticipantAddOutcome_UNCHANGED || again.GetParticipantRole() != writer {
		t.Fatalf("repeated grant: outcome %v role %v", again.GetOutcome(), again.GetParticipantRole())
	}

	// An owner role is not grantable here.
	if _, err := resource.AddSecretParticipant(ctx, &s4wave_space.AddSecretParticipantRequest{ObjectKey: "secrets/device", PeerId: device.String(), ParticipantRole: sobject.SOParticipantRole_SOParticipantRole_OWNER}); err == nil {
		t.Fatal("expected owner grant to be rejected")
	}
}

// TestSpaceResourceCreateSecretSameKeyKeepsOne covers two callers creating a
// Secret under one key, as two wizard tabs do: one creates it, the other gets
// world.ErrObjectExists and leaves no nested SharedObject behind, and a later
// create under the key is refused before it makes one.
func TestSpaceResourceCreateSecretSameKeyKeepsOne(t *testing.T) {
	ctx := t.Context()
	_, resource, release := setupSecretSpaceResourceTest(ctx, t)
	defer release()

	// Two callers create under one key at once.
	const key = "secret/host-ssh-password"
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{ObjectKey: key, Kind: s4wave_secret.SecretKindSSHPassword, Value: []byte{byte(i)}})
		})
	}
	wg.Wait()
	created := 0
	for _, err := range errs {
		switch {
		case err == nil:
			created++
		case !errors.Is(err, world.ErrObjectExists):
			t.Fatalf("create: unexpected error class %T", err)
		}
	}
	if created != 1 {
		t.Fatalf("created %d Secrets under one key, want 1", created)
	}

	// A later create under the key is refused.
	_, err := resource.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{ObjectKey: key, Kind: s4wave_secret.SecretKindSSHPassword, Value: []byte{2}})
	if !errors.Is(err, world.ErrObjectExists) {
		t.Fatal("repeated create was not refused as existing")
	}
	if n := countSecretPayloads(ctx, t, resource); n != 1 {
		t.Fatalf("provider holds %d Secret payloads, want 1", n)
	}
}

// countSecretPayloads counts the Secret payload SharedObjects the resource's
// provider account lists.
func countSecretPayloads(ctx context.Context, t *testing.T, resource *SpaceResource) int {
	t.Helper()
	provRef := resource.space.GetSharedObjectRef().GetProviderResourceRef()
	provAcc, rel, err := provider.ExAccessProviderAccount(ctx, resource.b, provRef.GetProviderId(), provRef.GetProviderAccountId(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel.Release()
	soProvider, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err)
	}
	list, relList, err := soProvider.AccessSharedObjectList(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relList()
	n := 0
	for _, entry := range list.GetValue().GetSharedObjects() {
		if entry.GetMeta().GetBodyType() == s4wave_secret.SecretBodyType {
			n++
		}
	}
	return n
}
