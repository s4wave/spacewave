package peer

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// TestSignedMsg tests signing a message.
func TestSignedMsg(t *testing.T) {
	p, err := NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	ctx := context.Background()
	exPeerID := p.GetPeerID()
	privKey, err := p.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	encContext := "bifrost/peer TestSignedMsg"
	msg := "hello world from signed message test"
	smsg, err := NewSignedMsg(encContext, privKey, hash.RecommendedHashType, []byte(msg))
	if err == nil {
		_, _, err = smsg.ExtractAndVerify(encContext)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	_, peerID, err := smsg.ExtractAndVerify(encContext)
	if err != nil {
		t.Fatal(err.Error())
	}
	if peerID != exPeerID {
		t.Fatalf("peer id mismatch: %s != %s", exPeerID.String(), peerID.String())
	}

	if !bytes.Equal(smsg.Data, []byte(msg)) {
		t.FailNow()
	}
}
