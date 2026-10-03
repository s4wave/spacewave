package graph

import (
	"context"
	"testing"
)

// TestSimpleGraph tests constructing a simple network graph
func TestSimpleGraph(t *testing.T) {
	// Create two disconnected LANs in the simulated network graph.
	g := NewGraph()
	lan1 := AddLAN(g)
	lan2 := AddLAN(g)

	// Prepare a helper that generates peers on the test graph.
	ctx := context.Background()
	addPeer := func() *Peer {
		p, err := GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		return p
	}

	// assertLinkedPeerCount asserts the number of linked peers
	assertLinkedPeerCount := func(p1 *Peer, count int) {
		lp := p1.GetLinkedPeers(g)
		if len(lp) != count {
			t.Fatalf(
				"expected %d but got %d peers linked to %s",
				count,
				len(lp),
				p1.GetPeerID().String(),
			)
		}
	}

	// Verify newly created peers start with no network links.
	p0 := addPeer()
	p1 := addPeer()
	assertLinkedPeerCount(p0, 0)
	assertLinkedPeerCount(p1, 0)

	// Join both initial peers to the first LAN.
	lan1.AddPeer(g, p0)
	assertLinkedPeerCount(p0, 0)
	lan1.AddPeer(g, p1)

	// Verify peers on the second LAN become linked within that LAN.
	p2 := addPeer()
	p3 := addPeer()
	lan2.AddPeer(g, p2)
	assertLinkedPeerCount(p2, 0)
	lan2.AddPeer(g, p3)
	assertLinkedPeerCount(p2, 1)

	// Assert association counts.
	l1ap := lan1.GetAssociatedPeers(g)
	if len(l1ap) != 2 {
		t.Fatalf("expected 2 associated peers got %v", len(l1ap))
	}
	l2ap := lan2.GetAssociatedPeers(g)
	if len(l2ap) != 2 {
		t.Fatalf("expected 2 associated peers got %v", len(l2ap))
	}

	// Assert reachability
	assertReachability := func(p1 *Peer, p2 *Peer, reachable bool) {
		path := g.ShortestPath(p1.Node, p2.Node)
		ir := len(path) != 0
		if reachable != ir {
			if ir {
				t.Fatalf(
					"did not expect reachability between %s and %s",
					p1.GetPeerID().String(),
					p2.GetPeerID().String(),
				)
			} else {
				t.Fatalf(
					"expected reachability between %s and %s",
					p1.GetPeerID().String(),
					p2.GetPeerID().String(),
				)
			}
		}
	}

	// Verify reachability within each LAN and prepare checks across the LANs.
	assertReachability(p0, p1, true)
	assertReachability(p2, p3, true)
	assertNetworkCrossCommunication := func(expected bool) {
		assertReachability(p0, p3, expected)
		assertReachability(p0, p2, expected)
		assertReachability(p2, p1, expected)
	}

	// Connect the LANs together (we expect reachability now).
	assertNetworkCrossCommunication(false)
	lan1.AddConnectionToLAN(g, lan2)
	assertNetworkCrossCommunication(true)
	assertLinkedPeerCount(p2, 3)
}
