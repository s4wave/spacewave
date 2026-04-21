package stream_api_dial

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	stream_api "github.com/s4wave/spacewave/net/stream/api/rpc"
)

// ProcessRPC processes an RPC by dialing the desired target.
func ProcessRPC(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
	rpc stream_api.RPC,
) error {
	if err := conf.Validate(); err != nil {
		return err
	}

	localPeerID, err := conf.ParseLocalPeerID()
	if err != nil {
		return err
	}

	remotePeerID, err := conf.ParsePeerID()
	if err != nil {
		return err
	}

	// Dial the target.
	if err := rpc.Send(&stream_api.Data{
		State: stream_api.StreamState_StreamState_ESTABLISHING,
	}); err != nil {
		return err
	}
	strm, rel, err := link.OpenStreamWithPeerEx(
		ctx,
		b,
		protocol.ID(conf.GetProtocolId()),
		localPeerID, remotePeerID,
		conf.GetTransportId(),
		stream.OpenOpts{},
	)
	if err != nil {
		return err
	}

	defer rel()
	defer strm.GetStream().Close()

	if err := rpc.Send(&stream_api.Data{
		State: stream_api.StreamState_StreamState_ESTABLISHED,
	}); err != nil {
		return err
	}
	return stream_api.AttachRPCToStream(rpc, strm.GetStream(), nil)
}
