package space_world_replica

import (
	"testing"

	"github.com/s4wave/spacewave/net/peer"
)

// TestReplicaProof checks that the owner accepts a proof only from the desired
// owner, over its stream, for the replicated object.
func TestReplicaProof(t *testing.T) {
	// The desired owner signs a proof of its copy.
	keyB, b := testPeer(t)
	keyC, c := testPeer(t)
	signed, err := SignReplicaProof(keyB, &SOReplicaProof{SharedObjectId: "so", PeerId: b, Seqno: 7})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := signed.Verify("so", b, b)
	if err != nil {
		t.Fatal(err)
	}
	if proof.GetSeqno() != 7 {
		t.Fatalf("seqno %d", proof.GetSeqno())
	}

	// A Device cannot sign a proof for another.
	if _, err := SignReplicaProof(keyC, &SOReplicaProof{SharedObjectId: "so", PeerId: b}); err == nil {
		t.Fatal("signed a proof naming another peer")
	}

	// The owner refuses a proof from another stream, another signer, another
	// object, or with changed bytes.
	if _, err := signed.Verify("so", b, c); err == nil {
		t.Fatal("accepted a proof from another stream peer")
	}
	other, err := SignReplicaProof(keyC, &SOReplicaProof{SharedObjectId: "so", PeerId: c})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Verify("so", b, b); err == nil {
		t.Fatal("accepted a proof signed by another peer")
	}
	if _, err := signed.Verify("other", b, b); err == nil {
		t.Fatal("accepted a proof for another object")
	}
	forged := signed.CloneVT()
	forged.Proof[len(forged.Proof)-1] ^= 1
	if _, err := forged.Verify("so", b, b); err == nil {
		t.Fatal("accepted a changed proof")
	}

	// A signature without its key is refused.
	bare := signed.CloneVT()
	bare.Signature.PubKey = nil
	if _, err := bare.Verify("so", b, b); err != peer.ErrEmptyPeerID {
		t.Fatalf("keyless proof: %v", err)
	}
}
