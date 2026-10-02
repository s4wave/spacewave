package sobject_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

func TestDigestSOAuthoritativeRoot(t *testing.T) {
	root := &sobject.SORoot{
		Inner: []byte{0x01, 0x02},
		AccountNonces: []*sobject.SOAccountNonce{
			{PeerId: "peer-a", Nonce: 7},
		},
		ValidatorSignatures: []*peer.Signature{{SigData: []byte("old")}},
	}
	nonceData, err := root.AccountNonces[0].MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	signatureData := append(append([]byte{}, root.Inner...), nonceData...)
	preimage := append([]byte("spacewave/sharedobject/authoritative-root/v1"), make([]byte, 8)...)
	binary.BigEndian.PutUint64(preimage[len(preimage)-8:], uint64(len(signatureData)))
	preimage = append(preimage, signatureData...)
	want := sha256.Sum256(preimage)

	got, err := sobject.DigestSOAuthoritativeRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want[:]) {
		t.Fatalf("digest = %x, want %x", got, want)
	}

	root.ValidatorSignatures[0].SigData = []byte("new")
	gotWithoutSignatureChange, err := sobject.DigestSOAuthoritativeRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotWithoutSignatureChange, got) {
		t.Fatal("root digest must exclude validator signatures")
	}

	root.AccountNonces[0].Nonce++
	gotWithNonceChange, err := sobject.DigestSOAuthoritativeRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(gotWithNonceChange, got) {
		t.Fatal("root digest must bind account nonces")
	}

	if _, err := sobject.DigestSOAuthoritativeRoot(nil); err == nil {
		t.Fatal("nil root should fail")
	}
}
