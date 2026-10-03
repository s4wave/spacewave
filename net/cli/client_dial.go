package cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	stream_api "github.com/s4wave/spacewave/net/stream/api"
	stream_api_rpc "github.com/s4wave/spacewave/net/stream/api/rpc"
	"github.com/s4wave/spacewave/net/util/rwc"
)

// RunDial runs the dial command.
func (a *ClientArgs) RunDial(*cli.Context) error {
	// Connect the dial command to the Bifrost daemon.
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Open the outgoing-stream RPC with the command context.
	client, err := c.DialStream(ctx)
	if err != nil {
		return err
	}

	// Send the dial configuration.
	err = client.Send(&stream_api.DialStreamRequest{
		Config: &a.DialConf,
	})
	if err != nil {
		return err
	}

	// Attach the outgoing stream to stdin and stdout.
	rpcClient := stream_api.NewDialStreamClientRPC(client)
	return stream_api_rpc.AttachRPCToStream(
		rpcClient,
		rwc.NewReadWriteCloser(os.Stdin, os.Stdout),
		nil,
	)
}
