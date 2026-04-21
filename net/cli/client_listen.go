package cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	stream_api "github.com/s4wave/spacewave/net/stream/api"
)

// RunListen runs the listen command.
func (a *ClientArgs) RunListen(*cli.Context) error {
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}
	req, err := c.ListenStreams(ctx, &stream_api.ListenStreamsRequest{
		ListeningConfig: &a.ListeningConf,
	})
	if err != nil {
		return err
	}

	for {
		resp, err := req.Recv()
		if err != nil {
			return err
		}

		os.Stdout.WriteString(resp.GetControllerStatus().String())
		os.Stdout.WriteString("\n")
	}
}
