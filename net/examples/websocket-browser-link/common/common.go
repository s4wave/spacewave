package common

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	link_holdopen_controller "github.com/s4wave/spacewave/net/link/hold-open"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	"github.com/s4wave/spacewave/net/pubsub/floodsub"
	floodsub_controller "github.com/s4wave/spacewave/net/pubsub/floodsub/controller"
	"github.com/sirupsen/logrus"
)

var (
	log = logrus.New()
	le  = logrus.NewEntry(log)
)

func init() {
	log.SetLevel(logrus.DebugLevel)
}

// BuildCommonBus builds a common bus.
// Also returns a cancel function.
func BuildCommonBus(ctx context.Context) (bus.Bus, *static.Resolver, crypto.PrivKey, error) {
	p, err := peer.NewPeer(nil)
	if err != nil {
		return nil, nil, nil, err
	}

	peerPrivKey, err := p.GetPrivKey(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	peerID := p.GetPeerID()
	peerPrivKeyPem, err := keypem.MarshalPrivKeyPem(peerPrivKey)
	if err != nil {
		return nil, nil, nil, err
	}

	// Construct the bus
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return nil, nil, nil, err
	}
	sr.AddFactory(peer_controller.NewFactory(b))
	sr.AddFactory(link_holdopen_controller.NewFactory(b))
	sr.AddFactory(floodsub_controller.NewFactory(b))

	le = le.WithField("peer-id", peerID.String())
	le.Debug("constructing peer controller")
	_, _, err = b.AddDirective(
		resolver.NewLoadControllerWithConfig(&peer_controller.Config{
			PrivKey: string(peerPrivKeyPem),
		}),
		bus.NewCallbackHandler(func(val directive.AttachedValue) {
			le.Debug("node controller resolved")
		}, nil, nil),
	)
	if err != nil {
		return nil, nil, nil, err
	}

	// keep links open
	holdOpen, err := link_holdopen_controller.NewController(b, le)
	if err != nil {
		return nil, nil, nil, err
	}
	_, err = b.AddController(ctx, holdOpen, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	// use pubsub: floodsub
	_, _, err = b.AddDirective(resolver.NewLoadControllerWithConfig(&floodsub_controller.Config{
		FloodsubConfig: &floodsub.Config{},
	}), nil)
	if err != nil {
		return nil, nil, nil, err
	}

	return b, sr, peerPrivKey, nil
}

// GetLogEntry returns the root log entry.
func GetLogEntry() *logrus.Entry {
	return le
}
