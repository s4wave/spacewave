package dex_solicit

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

// TestSessionReopensEachTimeOnRetainedLink proves that two linked nodes open
// a new DEX session each time their session ends while the link stays up.
func TestSessionReopensEachTimeOnRetainedLink(t *testing.T) {
	// Bound the test and share one protocol context between the nodes.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	le := logrus.NewEntry(logrus.New())
	protocolContext := []byte("shared-object")
	a := newTestRelayNode(t, ctx, le, "a", protocolContext, 0)
	b := newTestRelayNode(t, ctx, le, "b", protocolContext, 0)

	// Hold the link from a to b for the whole test.
	bTransport, err := b.transport.GetTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bPeerID := bTransport.(*inproc.Inproc).GetPeerID()
	_, linkRef, err := a.tb.Bus.AddDirective(
		ctx,
		link.NewEstablishLinkWithPeer("", bPeerID),
		directive.NewTypedCallbackHandler[link.MountedLink](nil, nil, nil, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(linkRef.Release)
	connectTestRelayNodes(t, ctx, a, b)

	// End the session several times and expect a new one on both nodes.
	for round := range 4 {
		sess := waitTestDexSession(t, a.dex, bPeerID)
		aStarts, bStarts := a.dex.GetSessionStarts(), b.dex.GetSessionStarts()
		sess.close()

		waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
		if err := a.dex.WaitSessionStart(waitCtx, aStarts); err != nil {
			waitCancel()
			t.Fatalf("round %d: node a opened no new session: %v", round, err)
		}
		if err := b.dex.WaitSessionStart(waitCtx, bStarts); err != nil {
			waitCancel()
			t.Fatalf("round %d: node b opened no new session: %v", round, err)
		}
		waitCancel()
	}
}
