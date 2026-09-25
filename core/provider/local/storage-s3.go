//go:build !tinygo

package provider_local

import (
	"context"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// CheckS3Location writes, reads back, and deletes a probe object in the
// bucket, and classifies the first failure.
func CheckS3Location(
	ctx context.Context,
	location *account_settings.S3Location,
	creds *block_store_s3.Credentials,
) *block_store_s3.CheckResult {
	client, err := buildS3Client(location, creds)
	if err != nil {
		return &block_store_s3.CheckResult{
			Outcome: block_store_s3.CheckOutcome_CHECK_OUTCOME_FAILED,
			Detail:  err.Error(),
		}
	}
	return block_store_s3.CheckBucket(ctx, client, location.GetBucket(), location.GetObjectPrefix())
}

// buildS3BlockStore opens a writable block store on the location's bucket.
func buildS3BlockStore(
	location *account_settings.S3Location,
	creds *block_store_s3.Credentials,
) (block.StoreOps, error) {
	client, err := buildS3Client(location, creds)
	if err != nil {
		return nil, err
	}
	return block_store_s3.NewS3Block(true, client, location.GetBucket(), location.GetObjectPrefix(), 0), nil
}

// buildS3Client builds a signing client for the location's endpoint.
func buildS3Client(
	location *account_settings.S3Location,
	creds *block_store_s3.Credentials,
) (*block_store_s3.Client, error) {
	return block_store_s3.BuildClient(&block_store_s3.ClientConfig{
		Endpoint:    location.GetEndpoint(),
		Credentials: creds,
		DisableSsl:  location.GetDisableSsl(),
		Region:      location.GetRegion(),
	})
}
