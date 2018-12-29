package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"

	"github.com/aperturerobotics/hydra/bucket/json"
	"github.com/aperturerobotics/hydra/core"
	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var applyBucketConfFile string
var applyBucketConfVolumeRegex string

// runApplyBucketConf runs the apply bucket config command.
func runApplyBucketConf(*cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// parse json to bucket configuration.
	dat, err := ioutil.ReadFile(applyBucketConfFile)
	if err != nil {
		return err
	}

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	sr.AddFactory(reconciler_example.NewFactory(b))

	var jconf bucket_json.Config
	if err := json.Unmarshal(dat, &jconf); err != nil {
		return err
	}

	bconf, err := jconf.ResolveToProto(ctx, b)
	if err != nil {
		return err
	}

	c, err := GetClient()
	if err != nil {
		return err
	}

	resp, err := c.PutBucketConfig(ctx, &api.PutBucketConfigRequest{
		VolumeIdRegex: applyBucketConfVolumeRegex,
		Config:        bconf,
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

		acr, err := bucket_json.NewApplyBucketConfigResult(
			ctx,
			b,
			msg.GetApplyConfResult(),
		)
		if err != nil {
			le.WithError(err).Warn("unable to print bucket config result")
			continue
		}
		d, err := json.Marshal(acr)
		if err != nil {
			le.WithError(err).Warn("unable to json marshal bucket config result")
			continue
		}
		os.Stdout.WriteString(string(d))
		os.Stdout.WriteString("\n")
	}
	return nil
}
