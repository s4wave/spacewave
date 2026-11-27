package volume_rpc

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/peer"
)

// TestGetPeerPrivResponse tests the GetPeerPrivResponse object.
func TestGetPeerPrivResponse(t *testing.T) {
	// Create a peer and obtain its generated private key.
	testPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	ctx := context.Background()
	privKey, err := testPeer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build the private-key response.
	resp, err := NewGetPeerPrivResponse(privKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the response reconstructs the original peer and private key.
	parsedPeer, err := resp.ParsePeer()
	if err != nil {
		t.Fatal(err.Error())
	}
	parsedPriv, err := parsedPeer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !parsedPriv.Equals(privKey) || parsedPeer.GetPeerID() != testPeer.GetPeerID() {
		t.Fail()
	}

	// Preserve the required-key error for an empty response.
	if _, err := (&GetPeerPrivResponse{}).ParsePeer(); err != peer.ErrNoPrivKey {
		t.Fatalf("empty response: got %v, want %v", err, peer.ErrNoPrivKey)
	}
}
