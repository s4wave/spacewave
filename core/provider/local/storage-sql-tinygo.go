//go:build tinygo

package provider_local

import (
	"context"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/sirupsen/logrus"
)

// errNoSQLClient reports that TinyGo builds carry no SQL client.
var errNoSQLClient = errors.New("this build has no SQL client")

// CheckSQLLocation reports that TinyGo builds carry no SQL client.
func CheckSQLLocation(
	context.Context,
	*account_settings.SqlLocation,
	*block_store_s3.Credentials,
) *block_store_s3.CheckResult {
	return &block_store_s3.CheckResult{
		Outcome: block_store_s3.CheckOutcome_CHECK_OUTCOME_FAILED,
		Detail:  errNoSQLClient.Error(),
	}
}

// buildSQLBlockStore reports that TinyGo builds carry no SQL client.
func buildSQLBlockStore(
	context.Context,
	*logrus.Entry,
	*account_settings.SqlLocation,
	string,
	*block_store_s3.Credentials,
) (backendStore, error) {
	return nil, errNoSQLClient
}
