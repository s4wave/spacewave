package cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	stream_api "github.com/s4wave/spacewave/net/stream/api"
)

// RunForwarding runs the forwarding command.
func (a *ClientArgs) RunForwarding(_ *cli.Context) error {
	// Connect the forwarding command to the Bifrost daemon.
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Start stream forwarding with the configured target.
	req, err := c.ForwardStreams(ctx, &stream_api.ForwardStreamsRequest{
		ForwardingConfig: &a.ForwardingConf,
	})
	if err != nil {
		return err
	}

	// Print forwarding-controller status updates as they arrive.
	for {
		resp, err := req.Recv()
		if err != nil {
			return err
		}

		os.Stdout.WriteString(resp.GetControllerStatus().String())
		os.Stdout.WriteString("\n")
	}
}
