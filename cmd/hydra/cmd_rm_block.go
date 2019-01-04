package main

import (
	"context"
	"os"

	"github.com/aperturerobotics/hydra/cid"
	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// runRmBlock runs the rm block command.
func runRmBlock(*cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	br, err := cid.UnmarshalString(getBlockRef)
	if err != nil {
		return err
	}

	c, err := GetClient()
	if err != nil {
		return err
	}

	_, err = c.BucketOp(ctx, &api.BucketOpRequest{
		Op:           api.BucketOp_BucketOp_BLOCK_RM,
		BucketOpArgs: bucketOpArgs,
		BlockRef:     br,
	})
	if err != nil {
		return err
	}
	os.Stdout.WriteString(br.MarshalString())
	os.Stdout.WriteString("\n")
	return nil
}
