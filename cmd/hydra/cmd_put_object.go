package main

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// runPutObject runs the put object command.
func runPutObject(*cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	c, err := GetClient()
	if err != nil {
		return err
	}

	var dat []byte
	if objectStoreFile == "" || objectStoreFile == "-" {
		le.Debug("reading from stdin")
		dat, err = ioutil.ReadAll(os.Stdin)
	} else {
		le.Debugf("reading from file %s", objectStoreFile)
		dat, err = ioutil.ReadFile(objectStoreFile)
	}
	if err != nil {
		return err
	}

	objectStoreOpArgs.Data = dat
	objectStoreOpArgs.Op = api.ObjectStoreOp_ObjectStoreOp_PUT_KEY
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
