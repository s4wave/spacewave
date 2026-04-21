package cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	peer_api "github.com/s4wave/spacewave/net/peer/api"
)

// RunIdentifyController runs an identify controller.
func (a *ClientArgs) RunIdentifyController(_ *cli.Context) error {
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	dat, _, err := a.LoadOrGenerateIdentifyKey()
	if err != nil {
		return err
	}
	a.IdentifyConf.PrivKey = string(dat)
	if err := a.IdentifyConf.Validate(); err != nil {
		return err
	}

	req, err := c.Identify(a.GetContext(), &peer_api.IdentifyRequest{
		Config: &a.IdentifyConf,
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
