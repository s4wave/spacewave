package peer

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// TestSignedMsg tests signing a message.
func TestSignedMsg(t *testing.T) {
	// Create a peer and obtain its signing key.
	p, err := NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Record the expected identity and private key.
	ctx := context.Background()
	exPeerID := p.GetPeerID()
	privKey, err := p.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Sign and immediately verify the test message.
	encContext := "bifrost/peer TestSignedMsg"
	msg := "hello world from signed message test"
	smsg, err := NewSignedMsg(encContext, privKey, hash.RecommendedHashType, []byte(msg))
	if err == nil {
		_, _, err = smsg.ExtractAndVerify(encContext)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the sender identity returned by the signed message.
	_, peerID, err := smsg.ExtractAndVerify(encContext)
	if err != nil {
		t.Fatal(err.Error())
	}
	if peerID != exPeerID {
		t.Fatalf("peer id mismatch: %s != %s", exPeerID.String(), peerID.String())
	}

	// Confirm that the original body remains intact.
	if !bytes.Equal(smsg.Data, []byte(msg)) {
		t.FailNow()
	}
}

func TestSignedMsgExtractAndVerifyRejectsTamperedData(t *testing.T) {
	// Create a peer and obtain its signing key.
	p, err := NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	privKey, err := p.GetPrivKey(context.Background())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Sign a body that will be tampered with after signing.
	encContext := "bifrost/peer TestSignedMsg tamper"
	smsg, err := NewSignedMsg(encContext, privKey, hash.RecommendedHashType, []byte("signed body"))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Replace the body and require signature verification to reject it.
	smsg.Data = []byte("tampered body")
	_, _, err = smsg.ExtractAndVerify(encContext)
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("expected ErrSignatureInvalid, got %v", err)
	}
}
