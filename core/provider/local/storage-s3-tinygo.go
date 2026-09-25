//go:build tinygo

package provider_local

import (
	"context"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// errNoS3Client reports that TinyGo builds carry no S3 client.
var errNoS3Client = errors.New("this build has no S3 client")

// CheckS3Location reports that TinyGo builds carry no S3 client.
func CheckS3Location(
	context.Context,
	*account_settings.S3Location,
	*block_store_s3.Credentials,
) *block_store_s3.CheckResult {
	return &block_store_s3.CheckResult{
		Outcome: block_store_s3.CheckOutcome_CHECK_OUTCOME_FAILED,
		Detail:  errNoS3Client.Error(),
	}
}

// buildS3BlockStore reports that TinyGo builds carry no S3 client.
func buildS3BlockStore(
	*account_settings.S3Location,
	*block_store_s3.Credentials,
) (backendStore, error) {
	return nil, errNoS3Client
}
