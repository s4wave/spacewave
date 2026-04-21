package rwc

import (
	"context"
	"net"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// TestConn tests the conn.
func TestConn(t *testing.T) {
	ctx := context.Background()

	peer1, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	peer2, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	a1, a2 := peer.NewNetAddr(peer1.GetPeerID()), peer.NewNetAddr(peer2.GetPeerID())
	c1, c2 := net.Pipe()

	pc1 := NewConn(ctx, c1, a1, a2, 10)
	pc2 := NewConn(ctx, c2, a2, a1, 10)

	data := []byte("testing 1234")
	n, err := pc1.Write(data)
	if err == nil && n != len(data) {
		err = errors.Errorf("expected to write %d but wrote %d", len(data), n)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	outData := make([]byte, len(data)*2)
	on, err := pc2.Read(outData)
	if err != nil {
		t.Fatal(err.Error())
	}
	outData = outData[:on]
	if on != len(data) {
		t.Fatalf(
			"length incorrect received %v != %v data: %v",
			on,
			len(data),
			string(outData),
		)
	}
}
