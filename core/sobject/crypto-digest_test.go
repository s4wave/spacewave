package sobject_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

func TestDigestSOOperationEnvelope(t *testing.T) {
	exactEnvelope := []byte{0x00, 0x01, 0x02, 0xff}
	want := sha256.Sum256(exactEnvelope)

	got := sobject.DigestSOOperationEnvelope(exactEnvelope)
	if !bytes.Equal(got, want[:]) {
		t.Fatalf("digest = %x, want %x", got, want)
	}

	if bytes.Equal(got, sobject.DigestSOOperationEnvelope(append(exactEnvelope, 0x00))) {
		t.Fatal("digest must bind exact envelope bytes")
	}
}

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

func TestDigestSOTerminalReceipt(t *testing.T) {
	receipt := &sobject.SOTerminalReceipt{
		Inner:               []byte("receipt-inner"),
		ValidatorSignatures: []*peer.Signature{{SigData: []byte("sig-1")}},
	}
	serialized, err := receipt.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(serialized)

	got, err := sobject.DigestSOTerminalReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want[:]) {
		t.Fatalf("digest = %x, want %x", got, want)
	}

	receipt.ValidatorSignatures[0].SigData = []byte("sig-2")
	changed, err := sobject.DigestSOTerminalReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(changed, got) {
		t.Fatal("terminal receipt digest must include validator signatures")
	}

	if _, err := sobject.DigestSOTerminalReceipt(nil); err == nil {
		t.Fatal("nil terminal receipt should fail")
	}
}

func TestDigestSOValidatorSet(t *testing.T) {
	config := &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
		{PeerId: "ignored-reader", Role: sobject.SOParticipantRole_SOParticipantRole_READER},
		{PeerId: "validator-z", Role: sobject.SOParticipantRole_SOParticipantRole_VALIDATOR},
		{PeerId: "owner-a", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: "ignored-writer", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
	}}
	preimage := append([]byte("spacewave/sharedobject/validator-set/v1"), make([]byte, 8)...)
	binary.BigEndian.PutUint64(preimage[len(preimage)-8:], 2)
	for _, peerID := range []string{"owner-a", "validator-z"} {
		preimage = binary.BigEndian.AppendUint64(preimage, uint64(len(peerID)))
		preimage = append(preimage, peerID...)
	}
	want := sha256.Sum256(preimage)

	got, err := sobject.DigestSOValidatorSet(config)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want[:]) {
		t.Fatalf("digest = %x, want %x", got, want)
	}

	config.Participants[1], config.Participants[2] = config.Participants[2], config.Participants[1]
	if reordered, err := sobject.DigestSOValidatorSet(config); err != nil || !bytes.Equal(reordered, got) {
		t.Fatalf("reordered validator set digest = %x, err %v; want %x", reordered, err, got)
	}
	config.Participants[0].PeerId = "different-reader"
	if filtered, err := sobject.DigestSOValidatorSet(config); err != nil || !bytes.Equal(filtered, got) {
		t.Fatalf("filtered validator set digest = %x, err %v; want %x", filtered, err, got)
	}

	duplicate := &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
		{PeerId: "same", Role: sobject.SOParticipantRole_SOParticipantRole_VALIDATOR},
		{PeerId: "same", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
	}}
	if _, err := sobject.DigestSOValidatorSet(duplicate); err == nil {
		t.Fatal("duplicate eligible peer IDs should fail")
	}
	missingPeerID := &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
		{Role: sobject.SOParticipantRole_SOParticipantRole_VALIDATOR},
	}}
	if _, err := sobject.DigestSOValidatorSet(missingPeerID); err == nil {
		t.Fatal("empty eligible peer ID should fail")
	}
	if _, err := sobject.DigestSOValidatorSet(nil); err == nil {
		t.Fatal("nil config should fail")
	}
}
