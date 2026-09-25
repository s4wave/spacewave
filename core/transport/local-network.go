//go:build !tinygo

package transport

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

// WithInprocNetwork enables authenticated links to other sessions on this native network.
// Network membership follows Execute; separate networks never share packet routes.
func WithInprocNetwork(network *inproc.Network) SessionTransportOption {
	return WithLocalTransport(func(ctx context.Context, le *logrus.Entry, b bus.Bus, peerID peer.ID) (*transport_controller.Controller, func(), error) {
		// Construct the transport under the session's existing peer controller.
		controller := inproc.BuildInprocController(le, b, peerID, &inproc.Config{})
		release, err := b.AddController(ctx, controller, nil)
		if err != nil {
			return nil, nil, err
		}
		raw, err := controller.GetTransport(ctx)
		if err != nil {
			release()
			return nil, nil, err
		}
		transport, ok := raw.(*inproc.Inproc)
		if !ok {
			release()
			return nil, nil, errors.New("in-process controller returned an unexpected transport")
		}

		// Remove packet reachability before releasing the controller.
		detach, err := network.Attach(ctx, transport)
		if err != nil {
			release()
			return nil, nil, err
		}
		return controller, func() { detach(); release() }, nil
	})
}
