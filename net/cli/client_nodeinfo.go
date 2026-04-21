package cli

import (
	"encoding/json"
	"os"

	"github.com/aperturerobotics/cli"
	peer_api "github.com/s4wave/spacewave/net/peer/api"
)

// RunPeerInfo runs the peer information command.
func (a *ClientArgs) RunPeerInfo(_ *cli.Context) error {
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	ni, err := c.GetPeerInfo(ctx, &peer_api.GetPeerInfoRequest{})
	if err != nil {
		return err
	}

	dat, err := json.MarshalIndent(ni, "", "\t")
	if err != nil {
		return err
	}
	os.Stdout.WriteString(string(dat))
	os.Stdout.WriteString("\n")
	return nil
}
