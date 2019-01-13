package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// runGetObject runs the get object command.
func runGetObject(*cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	c, err := GetClient()
	if err != nil {
		return err
	}

	objectStoreOpArgs.Op = api.ObjectStoreOp_ObjectStoreOp_GET_KEY
	if err := objectStoreOpArgs.Validate(); err != nil {
		return err
	}

	resp, err := c.ObjectStoreOp(ctx, objectStoreOpArgs)
	if err != nil {
		return err
	}
	if !resp.GetFound() {
		return errors.New("object not found")
	}

	data := resp.GetData()
	resp.Data = nil
	d, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	le.Debug(string(d))
	os.Stdout.Write(data)
	return nil
}
