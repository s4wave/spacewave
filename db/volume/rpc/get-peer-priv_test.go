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

	// Build and validate the private-key response.
	resp, err := NewGetPeerPrivResponse(privKey)
	if err == nil {
		err = resp.Validate()
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify parsing the response recovers the original private key.
	parsedPriv, err := resp.ParsePrivKey()
	if err != nil {
		t.Fatal(err.Error())
	}
	if !parsedPriv.Equals(privKey) {
		t.Fail()
	}
}
