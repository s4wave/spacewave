//go:build !js && !wasip1

package cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	api "github.com/aperturerobotics/hydra/daemon/api"
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

	dat, err := ni.MarshalJSON()
	if err != nil {
		return err
	}

	return writeIndentedJSON(os.Stdout, dat)
}
