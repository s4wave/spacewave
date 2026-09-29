package inproc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
)

// TestNetworkIsolationAndAttachmentLifetime exercises real links across scoped packet networks.
func TestNetworkIsolationAndAttachmentLifetime(t *testing.T) {
	// Separate network values do not make their peers reachable to each other.
	// Boot two isolated testbeds and run one peer in each.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	firstBed, _ := buildTestbed(t, ctx)
	secondBed, _ := buildTestbed(t, ctx)
	_, first, firstRef := execPeer(ctx, t, firstBed, nil)
	t.Cleanup(firstRef.Release)
	_, second, secondRef := execPeer(ctx, t, secondBed, nil)
	t.Cleanup(secondRef.Release)

	// Attach each peer to its own isolated network.
	network := NewNetwork()
	detachFirst, err := network.Attach(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(detachFirst)
	other := NewNetwork()
	detachOther, err := other.Attach(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(detachOther)

	// Require a cross-network dial to time out.
	isolatedCtx, cancelIsolated := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancelIsolated()
	if _, err := second.GetPeerDialer(isolatedCtx, first.GetPeerID()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("separate networks resolved a peer: %v", err)
	}

	// Moving into the same network supplies automatic dialing without static peer maps.

	// Move the second peer into the first network and verify dialing.
	detachOther()
	detachSecond, err := network.Attach(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(detachSecond)

	// Require a duplicate attachment of the same peer to fail.
	if _, err := network.Attach(ctx, second); err == nil {
		t.Fatal("duplicate peer attachment succeeded")
	}
	detachSecond()

	// Reattach the peer and release its earlier detach handle.
	currentDetach, err := network.Attach(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(currentDetach)
	detachSecond()

	// Establish a link and require it to authenticate the requested peer.
	linked, release, err := link.EstablishLinkWithPeerEx(ctx, secondBed.Bus, second.GetPeerID(), first.GetPeerID(), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	if linked.GetRemotePeer() != first.GetPeerID() {
		t.Fatal("link did not authenticate the requested peer")
	}
}

// TestIncomingDialReturnsLocalLink preserves a dialer's result when the remote peer initiates QUIC.
func TestIncomingDialReturnsLocalLink(t *testing.T) {
	// Select the higher peer so its dial request produces an incoming connection.
	// Boot two testbeds and order the peers by identity.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	firstBed, _ := buildTestbed(t, ctx)
	secondBed, _ := buildTestbed(t, ctx)
	firstController, first, firstRef := execPeer(ctx, t, firstBed, nil)
	t.Cleanup(firstRef.Release)
	secondController, second, secondRef := execPeer(ctx, t, secondBed, nil)
	t.Cleanup(secondRef.Release)

	// Order the peers so the lower peer dials the higher one.
	if first.GetPeerID() < second.GetPeerID() {
		first, second = second, first
		firstController = secondController
	}

	// Attach both endpoints through production network ownership.
	network := NewNetwork()

	// Attach both endpoints through production network ownership.
	for _, endpoint := range []*Inproc{first, second} {
		detach, err := network.Attach(ctx, endpoint)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(detach)
	}

	// The caller must receive its local link even though the other endpoint dialed.
	linked, err := firstController.DialPeerAddr(ctx, second.GetPeerID(), &dialer.DialerOpts{Address: second.LocalAddr().String()})
	if err != nil {
		t.Fatal(err)
	}
	if linked.GetLocalPeer() != first.GetPeerID() || linked.GetRemotePeer() != second.GetPeerID() {
		t.Fatal("incoming dial returned the wrong local or remote peer")
	}
}
