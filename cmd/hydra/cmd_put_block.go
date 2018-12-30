package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"

	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var blockDataFile string
var putBlockVolumeRegex string
var putBlockBucketID string

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

	resp, err := c.PutBlock(ctx, &api.PutBlockRequest{
		// BucketId is the bucket ID to put the block into.
		BucketId: putBlockBucketID,
		// VolumeIdRegex is the regex of volume IDs to put the block into.
		// If empty, will only apply to volumes that have the bucket.
		VolumeIdRegex: putBlockVolumeRegex,
		// Data is the data to put in the block.
		// May be constrained by the bucket block size limit.
		Data: dat,
	})
	if err != nil {
		return err
	}

	for {
		msg, err := resp.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		d, err := json.Marshal(msg)
		if err != nil {
			le.WithError(err).Warn("unable to marshal put block result")
			continue
		}
		os.Stdout.WriteString(string(d))
		os.Stdout.WriteString("\n")
	}
	return nil
}
