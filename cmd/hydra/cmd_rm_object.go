package main

import (
	"context"

	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// runRmObject runs the rm object command.
func runRmObject(*cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	c, err := GetClient()
	if err != nil {
		return err
	}

	objectStoreOpArgs.Op = api.ObjectStoreOp_ObjectStoreOp_DELETE_KEY
	if err := objectStoreOpArgs.Validate(); err != nil {
		return err
	}

	resp, err := c.ObjectStoreOp(ctx, objectStoreOpArgs)
	if err != nil {
		return err
	}
	_ = resp
	return nil
}
