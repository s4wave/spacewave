package cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	peer_api "github.com/s4wave/spacewave/net/peer/api"
)

// RunIdentifyController runs an identify controller.
func (a *ClientArgs) RunIdentifyController(_ *cli.Context) error {
	// Connect the identify command to the Bifrost daemon.
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Load the identification key and validate the controller configuration.
	dat, _, err := a.LoadOrGenerateIdentifyKey()
	if err != nil {
		return err
	}
	a.IdentifyConf.PrivKey = string(dat)
	if err := a.IdentifyConf.Validate(); err != nil {
		return err
	}

	// Start the identify controller through the daemon API.
	req, err := c.Identify(a.GetContext(), &peer_api.IdentifyRequest{
		Config: &a.IdentifyConf,
	})
	if err != nil {
		return err
	}

	// Print identify-controller status updates as they arrive.
	for {
		resp, err := req.Recv()
		if err != nil {
			return err
		}

		os.Stdout.WriteString(resp.GetControllerStatus().String())
		os.Stdout.WriteString("\n")
	}
}
