package peer_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestPrivKeyIntegrity(t *testing.T) {
	// Configure a logger for the peer controller identity check.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Generate a peer whose identity the controller must preserve.
	npeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the generated peer private key for controller configuration.
	ctx := context.Background()
	privKey, err := npeer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Encode the peer private key into the controller configuration.
	privKeyID := npeer.GetPeerID()
	peerControllerConf, err := NewConfigWithPrivKey(privKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Construct the peer controller from the encoded private key.
	f := NewFactory(nil)
	ctrl, err := f.Construct(ctx, peerControllerConf, controller.ConstructOpts{Logger: le})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check that the controller preserves the generated peer identity.
	cctrl := ctrl.(*Controller)
	if privKeyID.String() != cctrl.GetPeerID().String() {
		t.Fatalf("priv key id mismatch: %s != %s", privKeyID.String(), cctrl.GetPeerID().String())
	}
}
