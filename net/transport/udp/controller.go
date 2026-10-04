package udp

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/transport"
	tc "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/sirupsen/logrus"
)

// NewController builds the transport controller for a UDP configuration.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config, opts ...tc.Option) (*tc.Controller, error) {
	// Resolve the peer identity constraint for the UDP transport.
	peerIDConstraint, err := conf.ParseTransportPeerID()
	if err != nil {
		return nil, err
	}

	// Construct the transport controller around the UDP listener.
	return tc.NewController(
		le,
		b,
		controller.NewInfo(ControllerID, Version, "udp transport"),
		peerIDConstraint,
		conf.GetVerbose(),
		func(
			ctx context.Context,
			le *logrus.Entry,
			pkey crypto.PrivKey,
			handler transport.TransportHandler,
		) (transport.Transport, error) {
			return NewUDP(
				ctx,
				le,
				pkey,
				handler,
				conf.GetPacketOpts(),
				0,
				conf.GetListenAddr(),
				conf.GetDialers(),
			)
		},
		opts...,
	), nil
}
