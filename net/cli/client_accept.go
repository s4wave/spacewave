package cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	stream_api "github.com/s4wave/spacewave/net/stream/api"
	stream_api_rpc "github.com/s4wave/spacewave/net/stream/api/rpc"
	"github.com/s4wave/spacewave/net/util/rwc"
)

// RunAccept runs the accept command.
func (a *ClientArgs) RunAccept(*cli.Context) error {
	// Connect the accept command to the Bifrost daemon.
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Open the incoming-stream RPC with the command context.
	client, err := c.AcceptStream(ctx)
	if err != nil {
		return err
	}

	// Apply the requested peer filter and send the accept configuration.
	if len(a.RemotePeerIdsCsv) != 0 {
		a.AcceptConf.RemotePeerIds = a.ParseRemotePeerIdsCsv()
	}
	err = client.Send(&stream_api.AcceptStreamRequest{
		Config: &a.AcceptConf,
	})
	if err != nil {
		return err
	}

	// Attach the accepted stream to stdin and stdout.
	rpcClient := stream_api.NewAcceptStreamClientRPC(client)
	return stream_api_rpc.AttachRPCToStream(
		rpcClient,
		rwc.NewReadWriteCloser(os.Stdin, os.Stdout),
		nil,
	)
}
