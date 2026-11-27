package sobject_invite

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
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
