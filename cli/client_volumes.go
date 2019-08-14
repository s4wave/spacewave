package cli

import (
	"encoding/json"
	"os"

	api "github.com/aperturerobotics/hydra/daemon/api"
	"github.com/urfave/cli"
)

// RunListVolumes runs listing volumes.
func (a *ClientArgs) RunListVolumes(_ *cli.Context) error {
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	ni, err := c.ListVolumes(ctx, &api.ListVolumesRequest{})
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
