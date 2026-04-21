package peer_ssh

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/peer"
)

// TestSshKeys tests generating a key and converting to a ssh signer and pub key.
func TestSshKeys(t *testing.T) {
	// generate peer
	p, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// create signer
	ctx := context.Background()
	privKey, err := p.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, err = NewSigner(privKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	// create pub key
	_, err = NewPublicKey(p.GetPubKey())
	if err != nil {
		t.Fatal(err.Error())
	}
}
