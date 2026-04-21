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
	ctx := serv.Context()
	msg, err := serv.Recv()
	if err != nil {
		return err
	}

	conf := msg.GetConfig()
	if err := conf.Validate(); err != nil {
		return err
	}

	dir := resolver.NewLoadControllerWithConfig(conf)

	// wait until it's ready
	ctrl, _, ctrlRef, err := loader.WaitExecControllerRunningTyped[*stream_api_accept.Controller](ctx, a.bus, dir, nil)
	if err != nil {
		return err
	}
	defer ctrlRef.Release()

	return ctrl.AttachRPC(stream_api.NewAcceptServerRPC(serv))
}
