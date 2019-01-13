package main

import (
	"context"
	"os"

	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// runListObjectKeys runs the list object keys command.
func runListObjectKeys(*cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	c, err := GetClient()
	if err != nil {
		return err
	}

	objectStoreOpArgs.Op = api.ObjectStoreOp_ObjectStoreOp_LIST_KEYS
	if err := objectStoreOpArgs.Validate(); err != nil {
		return err
	}

	resp, err := c.ObjectStoreOp(ctx, objectStoreOpArgs)
	if err != nil {
		return err
	}
	le.WithField("key-count", len(resp.GetKeys())).Debug("returned keys")
	for _, key := range resp.GetKeys() {
		os.Stdout.WriteString(key)
		os.Stdout.WriteString("\n")
	}
	return nil
}
