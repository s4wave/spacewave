package provider_spacewave

import (
	"github.com/aperturerobotics/util/refcount"

	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
)

var snapshotRefCountOptions = &refcount.Options{
	RetryBackoff: providerBackoff,
	ShouldRetry: func(err error) bool {
		return !clouderror.IsNonRetryable(err)
	},
	RetryDelay: providerRetryDelay,
}
