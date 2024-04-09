package volume_rpc

import (
	"context"
	"testing"

	"github.com/aperturerobotics/bifrost/peer"
)

// TestGetPeerPrivResponse tests the GetPeerPrivResponse object.
func TestGetPeerPrivResponse(t *testing.T) {
	testPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	ctx := context.Background()
	privKey, err := testPeer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	resp, err := NewGetPeerPrivResponse(privKey)
	if err == nil {
		err = resp.Validate()
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	parsedPriv, err := resp.ParsePrivKey()
	if err != nil {
		t.Fatal(err.Error())
	}
	if !parsedPriv.Equals(privKey) {
		t.Fail()
	}
}
