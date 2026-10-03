package bifrost_api

import (
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	stream_api "github.com/s4wave/spacewave/net/stream/api"
	stream_api_accept "github.com/s4wave/spacewave/net/stream/api/accept"
)

// AcceptStream accepts an incoming stream.
// Stream data is sent over the request / response streams.
func (a *API) AcceptStream(serv stream_api.SRPCStreamService_AcceptStreamStream) error {
	// Receive the initial stream acceptance request.
	ctx := serv.Context()
	msg, err := serv.Recv()
	if err != nil {
		return err
	}

	// Validate the requested stream acceptance configuration.
	conf := msg.GetConfig()
	if err := conf.Validate(); err != nil {
		return err
	}

	// Wait for the stream acceptance controller and retain it until attachment ends.
	dir := resolver.NewLoadControllerWithConfig(conf)
	ctrl, _, ctrlRef, err := loader.WaitExecControllerRunningTyped[*stream_api_accept.Controller](ctx, a.bus, dir, nil)
	if err != nil {
		return err
	}
	defer ctrlRef.Release()

	// Serve the accept stream until it ends.
	return ctrl.AttachRPC(stream_api.NewAcceptServerRPC(serv))
}
