package cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	peer_api "github.com/s4wave/spacewave/net/peer/api"
)

// RunPeerInfo runs the peer information command.
func (a *ClientArgs) RunPeerInfo(_ *cli.Context) error {
	// Connect the peer-info command to the Bifrost daemon.
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Fetch the local peer information from the daemon.
	ni, err := c.GetPeerInfo(ctx, &peer_api.GetPeerInfoRequest{})
	if err != nil {
		return err
	}

	// Encode the local peer information as JSON and print it.
	dat, err := ni.MarshalJSON()
	if err != nil {
		return err
	}
	os.Stdout.WriteString(string(dat))
	os.Stdout.WriteString("\n")
	return nil
}
