//go:build tinygo

package transport

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/sirupsen/logrus"
)

func sessionTransportFactories(b bus.Bus) []controller.Factory {
	return nil
}

func (t *SessionTransport) startWebRTCControllers(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
) (*transport_controller.Controller, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return nil, nil, nil
}
