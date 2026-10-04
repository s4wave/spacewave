package main

import (
	"context"
	"crypto/rand"
	"net"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/daemon"
	link_holdopen_controller "github.com/s4wave/spacewave/net/link/hold-open"
	"github.com/s4wave/spacewave/net/peer"
	tptc "github.com/s4wave/spacewave/net/transport/controller"
	udptpt "github.com/s4wave/spacewave/net/transport/udp"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

func init() {
	log.SetLevel(logrus.DebugLevel)
}

func genPeerIdentity() (peer.ID, crypto.PrivKey) {
	// Generate a fresh Ed25519 key and derive its peer identity.
	pk1, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	pid1, _ := peer.IDFromPrivateKey(pk1)
	log.Debugf("generated peer id: %s", pid1.String())

	return pid1, pk1
}

func execute() error {
	// Prepare context and logging for both local daemons.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create distinct identities for the UDP peers.
	_, pk1 := genPeerIdentity()
	p2, pk2 := genPeerIdentity()

	// Start the first daemon.
	d1, err := daemon.NewDaemon(ctx, pk1, daemon.ConstructOpts{
		LogEntry: le,
	})
	if err != nil {
		return errors.Wrap(err, "construct daemon 1")
	}

	// Start the second daemon.
	d2, err := daemon.NewDaemon(ctx, pk2, daemon.ConstructOpts{
		LogEntry: le,
	})
	if err != nil {
		return errors.Wrap(err, "construct daemon 2")
	}

	// Resolve each daemon's bus and factory registry.
	bus1 := d1.GetControllerBus()
	bus2 := d2.GetControllerBus()
	sr1 := d1.GetStaticResolver()
	sr2 := d2.GetStaticResolver()

	// Track the two hold-open controller routines.
	var wg sync.WaitGroup
	wg.Add(2)

	// Start a hold-open controller on each daemon.
	sr1.AddFactory(link_holdopen_controller.NewFactory(bus1))
	_, _, hr1, err := loader.WaitExecControllerRunning(
		ctx,
		bus1,
		resolver.NewLoadControllerWithConfig(&link_holdopen_controller.Config{}),
		nil,
	)
	if err != nil {
		return err
	}
	defer hr1.Release()

	// Start the hold-open controller on the second daemon.
	sr2.AddFactory(link_holdopen_controller.NewFactory(bus2))
	_, _, hr2, err := loader.WaitExecControllerRunning(
		ctx,
		bus2,
		resolver.NewLoadControllerWithConfig(&link_holdopen_controller.Config{}),
		nil,
	)
	if err != nil {
		return err
	}
	defer hr2.Release()

	// Start the first daemon's UDP transport.
	tc1, _, udpRef1, err := loader.WaitExecControllerRunningTyped[*tptc.Controller](
		ctx,
		bus1,
		resolver.NewLoadControllerWithConfig(&udptpt.Config{
			ListenAddr: ":5553",
		}),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "listen on udp 1")
	}
	defer udpRef1.Release()
	le.Info("UDP listening on: :5553")
	tpt1, _ := tc1.GetTransport(ctx)

	// Start the second daemon's UDP transport.
	tc2, _, udpRef2, err := loader.WaitExecControllerRunningTyped[*tptc.Controller](
		ctx,
		bus2,
		resolver.NewLoadControllerWithConfig(&udptpt.Config{
			ListenAddr: ":5554",
		}),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "listen on udp 2")
	}
	defer udpRef2.Release()
	le.Info("UDP listening on: :5554")
	_, _ = tc2.GetTransport(ctx)

	// Connect the first transport to the second peer over localhost.
	tpt1.(*udptpt.UDP).DialPeer(ctx, p2, (&net.UDPAddr{
		IP:   net.IP{127, 0, 0, 1},
		Port: 5554,
	}).String())
	<-ctx.Done()
	return nil
}

func main() {
	if err := execute(); err != nil {
		panic(err)
	}
}
