package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/aperturerobotics/hydra/volume"
	"github.com/urfave/cli"
)

var listBucketRequest volume.ListBucketsRequest

// runListBuckets runs the list buckets command.
func runListBuckets(*cli.Context) error {
	ctx := context.Background()
	c, err := GetClient()
	if err != nil {
		return err
	}

	ni, err := c.ListBuckets(ctx, &listBucketRequest)
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
