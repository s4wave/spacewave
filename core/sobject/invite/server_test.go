package sobject_invite

import (
	"bytes"
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/zeebo/blake3"
)

func TestBuildJoinResponse(t *testing.T) {
	ctx := context.Background()
	p, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := p.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := BuildJoinResponse("inv-123", priv)
	if err != nil {
		t.Fatalf("BuildJoinResponse: %v", err)
	}

	if resp.GetInviteId() != "inv-123" {
		t.Fatal("invite_id mismatch")
	}
	if resp.GetResponderPeerId() != p.GetPeerID().String() {
		t.Fatal("responder_peer_id mismatch")
	}
	if len(resp.GetResponderPubkey()) == 0 {
		t.Fatal("responder_pubkey should not be empty")
	}
	if resp.GetSignature() == nil {
		t.Fatal("signature should not be nil")
	}

	// Verify the signature is valid.
	pubKey := priv.GetPublic()
	signData, err := (&sobject.SOJoinResponse{
		InviteId:        resp.GetInviteId(),
		ResponderPeerId: resp.GetResponderPeerId(),
		ResponderPubkey: resp.GetResponderPubkey(),
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	valid, err := resp.GetSignature().VerifyWithPublic("sobject join response", pubKey, signData)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("signature should be valid")
	}
}

func TestBuildJoinResponseSignatureContext(t *testing.T) {
	ctx := context.Background()
	p, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := p.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := BuildJoinResponse("inv-456", priv)
	if err != nil {
		t.Fatal(err)
	}

	// Wrong signing context should fail verification.
	signData, err := (&sobject.SOJoinResponse{
		InviteId:        resp.GetInviteId(),
		ResponderPeerId: resp.GetResponderPeerId(),
		ResponderPubkey: resp.GetResponderPubkey(),
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	valid, err := resp.GetSignature().VerifyWithPublic("wrong context", priv.GetPublic(), signData)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("signature should not validate with wrong context")
	}
}

func TestHashInviteToken(t *testing.T) {
	token := []byte("test-token")
	h := HashInviteToken(token)
	if len(h) != 32 {
		t.Fatalf("expected 32-byte hash, got %d", len(h))
	}

	// Same token produces same hash.
	h2 := HashInviteToken(token)
	if !bytes.Equal(h, h2) {
		t.Fatal("deterministic hash expected")
	}

	// Different token produces different hash.
	h3 := HashInviteToken([]byte("other-token"))
	if bytes.Equal(h, h3) {
		t.Fatal("different tokens should produce different hashes")
	}
}

func TestHashInviteTokenMatchesCreateSOInviteOp(t *testing.T) {
	// Verify that HashInviteToken produces the same result as the
	// BLAKE3 hashing used in CreateSOInviteOp.
	token := []byte("a]b^c_d`e{f|g}h~i")
	hashArr := blake3.Sum256(token)
	expected := hashArr[:]
	got := HashInviteToken(token)
	if !bytes.Equal(got, expected) {
		t.Fatal("HashInviteToken should match blake3.Sum256")
	}
}

func TestLookupFnNoMatch(t *testing.T) {
	lookupFn := func(_ context.Context, _ []byte) (*InviteLookupResult, error) {
		return nil, nil
	}
	result, err := lookupFn(context.Background(), []byte("unknown"))
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatal("expected nil result for unknown token hash")
	}
}

func TestLookupFnError(t *testing.T) {
	lookupFn := func(_ context.Context, _ []byte) (*InviteLookupResult, error) {
		return nil, errors.New("storage error")
	}
	_, err := lookupFn(context.Background(), []byte("any"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSerializeDeserializeInviteLink(t *testing.T) {
	msg := &sobject.SOInviteMessage{
		InviteId:       "inv-link-test",
		SharedObjectId: "so-test-123",
		OwnerPeerId:    "QmTest123",
		ProviderId:     "local",
		Token:          []byte("token-32-bytes-for-testing-here!"),
		Role:           sobject.SOParticipantRole_SOParticipantRole_WRITER,
		MaxUses:        10,
	}

	encoded, err := SerializeInviteLink(msg)
	if err != nil {
		t.Fatalf("SerializeInviteLink: %v", err)
	}
	if encoded == "" {
		t.Fatal("encoded should not be empty")
	}

	decoded, err := DeserializeInviteLink(encoded)
	if err != nil {
		t.Fatalf("DeserializeInviteLink: %v", err)
	}

	if decoded.GetInviteId() != msg.GetInviteId() {
		t.Fatal("invite_id mismatch")
	}
	if decoded.GetSharedObjectId() != msg.GetSharedObjectId() {
		t.Fatal("shared_object_id mismatch")
	}
	if decoded.GetOwnerPeerId() != msg.GetOwnerPeerId() {
		t.Fatal("owner_peer_id mismatch")
	}
	if decoded.GetRole() != msg.GetRole() {
		t.Fatal("role mismatch")
	}
	if decoded.GetMaxUses() != msg.GetMaxUses() {
		t.Fatal("max_uses mismatch")
	}
	if !bytes.Equal(decoded.GetToken(), msg.GetToken()) {
		t.Fatal("token mismatch")
	}
}
