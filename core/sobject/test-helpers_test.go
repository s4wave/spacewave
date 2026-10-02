package sobject

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

const mockSharedObjectID = "test_object"

// mockConfigHash stands in for a config chain head in configs built by hand.
var mockConfigHash = bytes.Repeat([]byte{0xc0}, 32)

// mockPrevOpHash stands in for a previous operation the state does not hold.
var mockPrevOpHash = bytes.Repeat([]byte{0xa0}, 32)

// linkAt returns state's next link for the author of priv, moved to nonce.
// A nil state yields a link with no other heads.
func linkAt(state *SOState, priv crypto.PrivKey, nonce uint64) *SOOperationLink {
	// Start from the author's next link.
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		panic(err)
	}
	link := &SOOperationLink{}
	if state != nil {
		link = state.NextOperationLink(peerID.String())
	}
	if len(link.ConfigHash) == 0 {
		link.ConfigHash = mockConfigHash
	}

	// Move the link to nonce, naming a predecessor after the first.
	if nonce == link.Nonce {
		return link
	}
	link.Nonce = nonce
	switch {
	case nonce <= 1:
		link.PrevOpHash = nil
	case len(link.PrevOpHash) == 0:
		link.PrevOpHash = mockPrevOpHash
	}
	return link
}

func createMockPeers(t *testing.T, count uint64) []peer.Peer {
	t.Helper()

	peers := make([]peer.Peer, count)
	for i := range count {
		p, err := peer.NewPeer(nil)
		if err != nil {
			t.Fatalf("create peer %d: %v", i+1, err)
		}
		peers[i] = p
	}
	return peers
}

func mustMarshalVT[T interface{ MarshalVT() ([]byte, error) }](
	t *testing.T,
	msg T,
) []byte {
	t.Helper()
	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
