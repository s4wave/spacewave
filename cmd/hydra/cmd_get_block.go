package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var getBlockRef string

// runGetBlock runs the get block command.
func runGetBlock(*cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	br, err := cid.UnmarshalString(getBlockRef)
	if err != nil {
		return err
	}

	c, err := GetClient()
	if err != nil {
		return err
	}

	resp, err := c.GetBlock(ctx, &api.GetBlockRequest{
		BucketOpArgs: bucketOpArgs,
		BlockRef:     br,
	})
	if err != nil {
		return err
	}

	data := resp.Data
	resp.Data = nil
	d, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	le.Debug(string(d))
	os.Stdout.Write(data)
	return nil
}
