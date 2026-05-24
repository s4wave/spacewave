package sobject

import (
	"testing"

	"github.com/s4wave/spacewave/net/peer"
)

const mockSharedObjectID = "test_object"

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
