//go:build !js && !wasip1

package cli

import (
	"io"
	"os"

	"github.com/aperturerobotics/cli"
	bucket_json "github.com/s4wave/spacewave/db/bucket/json"
	"github.com/s4wave/spacewave/db/core"
)

// RunApplyBucketConf runs applying a bucket configuration.
func (a *ClientArgs) RunApplyBucketConf(_ *cli.Context) error {
	// Resolve the request context and read the bucket configuration file.
	le := a.GetLogger()
	ctx := a.GetContext()
	dat, err := os.ReadFile(a.ApplyBucketConfigFile)
	if err != nil {
		return err
	}

	// Build the resolver bus and parse the JSON configuration.
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	jconf, err := bucket_json.ParseConfig(dat)
	if err != nil {
		return err
	}
	bconf, err := jconf.ResolveToProto(ctx, b)
	if err != nil {
		return err
	}

	// Submit the resolved bucket configuration to the daemon.
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

	// Stream each applied configuration result to standard output.
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
		d, err := acr.MarshalJSON()
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
	// Resolve the client and request context.
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Configure and execute the bucket listing.
	req := a.ListBucketsReq.CloneVT()
	req.VolumeIdList = a.ListBucketsReqVolumeIDs.Value()
	ni, err := c.ListBuckets(ctx, req)
	if err != nil {
		return err
	}

	// Marshal the response and write formatted JSON.
	dat, err := ni.MarshalJSON()
	if err != nil {
		return err
	}
	return writeIndentedJSON(os.Stdout, dat)
}
