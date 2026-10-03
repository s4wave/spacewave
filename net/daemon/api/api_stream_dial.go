package bifrost_api

import (
	stream_api "github.com/s4wave/spacewave/net/stream/api"
	stream_api_dial "github.com/s4wave/spacewave/net/stream/api/dial"
)

// DialStream dials a outgoing stream.
// Stream data is sent over the request / response streams.
func (a *API) DialStream(serv stream_api.SRPCStreamService_DialStreamStream) error {
	// Receive the initial stream dial request.
	ctx := serv.Context()
	msg, err := serv.Recv()
	if err != nil {
		return err
	}

	// Validate the requested stream dial configuration.
	conf := msg.GetConfig()
	if err := conf.Validate(); err != nil {
		return err
	}

	return stream_api_dial.ProcessRPC(
		ctx,
		a.bus,
		conf,
		stream_api.NewDialServerRPC(serv),
	)
}
