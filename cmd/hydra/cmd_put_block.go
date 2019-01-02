package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var blockDataFile string

// runPutBlock runs the put block command.
func runPutBlock(*cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	c, err := GetClient()
	if err != nil {
		return err
	}

	var dat []byte
	if blockDataFile == "" || blockDataFile == "-" {
		le.Debug("reading from stdin")
		dat, err = ioutil.ReadAll(os.Stdin)
	} else {
		le.Debugf("reading from file %s", blockDataFile)
		dat, err = ioutil.ReadFile(blockDataFile)
	}

	le.Debug("putting block")
	resp, err := c.PutBlock(ctx, &api.PutBlockRequest{
		BucketOpArgs: bucketOpArgs,
		Data:         dat,
	})
	if err != nil {
		return err
	}

	d, err := json.Marshal(resp)
	if err != nil {
		le.WithError(err).Warn("unable to marshal put block result")
		return err
	}
	le.Debug(string(d))
	os.Stdout.WriteString(resp.GetEvent().GetBlockRef().MarshalString())
	os.Stdout.WriteString("\n")
	return nil
}
