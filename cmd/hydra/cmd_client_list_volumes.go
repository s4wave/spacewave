package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/urfave/cli"
)

// runListVolumes runs the list volumes command.
func runListVolumes(*cli.Context) error {
	ctx := context.Background()
	c, err := GetClient()
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
