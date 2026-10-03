//go:build !js && !wasip1

package cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	api "github.com/s4wave/spacewave/db/daemon/api"
)

// RunListVolumes runs listing volumes.
func (a *ClientArgs) RunListVolumes(_ *cli.Context) error {
	// Connect to the daemon that supplies attached volume information.
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Request the daemon's attached volume records.
	ni, err := c.ListVolumes(ctx, &api.ListVolumesRequest{})
	if err != nil {
		return err
	}

	// Encode the volume records for the command output.
	dat, err := ni.MarshalJSON()
	if err != nil {
		return err
	}

	return writeIndentedJSON(os.Stdout, dat)
}
