package link_test

import (
	"testing"

	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/sirupsen/logrus"
)

func TestEstablishLinkWithPeerExIdleHasNoReference(t *testing.T) {
	// Start a network testbed without a peer to leave the directive idle.
	tb, err := testbed.NewTestbed(t.Context(), logrus.NewEntry(logrus.New()), testbed.TestbedOpts{NoPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Execute the idle peer link directive.
	value, release, err := link.EstablishLinkWithPeerEx(t.Context(), tb.Bus, "local", "remote", true)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the idle directive retains neither a link nor a reference.
	if value != nil || release != nil {
		t.Fatal("idle directive returned a value or release function")
	}
}
