//go:build tinygo

package provider_local

import (
	"context"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// CheckS3Location reports that TinyGo builds carry no S3 client.
func CheckS3Location(
	context.Context,
	*account_settings.S3Location,
	*block_store_s3.Credentials,
) *block_store_s3.CheckResult {
	return &block_store_s3.CheckResult{
		Outcome: block_store_s3.CheckOutcome_CHECK_OUTCOME_FAILED,
		Detail:  "this build has no S3 client",
	}
}
