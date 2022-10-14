package cli

import (
	"encoding/json"
	"io"
	"os"

	bucket_json "github.com/aperturerobotics/hydra/bucket/json"
	"github.com/aperturerobotics/hydra/core"
	reconciler_example "github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/urfave/cli/v2"
)

// RunApplyBucketConf runs applying a bucket configuration.
func (a *ClientArgs) RunApplyBucketConf(_ *cli.Context) error {
	le := a.GetLogger()
	ctx := a.GetContext()

	// parse json to bucket configuration.
	dat, err := os.ReadFile(a.ApplyBucketConfigFile)
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

	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	req := &a.ApplyBucketConfigReq
	req.Config = bconf
	req.VolumeIdList = a.ApplyBucketConfigReqVolumeIDs.Value()
	resp, err := c.ApplyBucketConfig(ctx, req)
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

// RunListBuckets runs listing buckets.
func (a *ClientArgs) RunListBuckets(_ *cli.Context) error {
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	req := a.ListBucketsReq.CloneVT()
	req.VolumeIdList = a.ListBucketsReqVolumeIDs.Value()
	ni, err := c.ListBuckets(ctx, req)
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
