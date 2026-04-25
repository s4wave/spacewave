package provider_spacewave

import "github.com/aperturerobotics/util/refcount"

var snapshotRefCountOptions = &refcount.Options{
	RetryBackoff: providerBackoff,
	ShouldRetry: func(err error) bool {
		return !isNonRetryableCloudError(err)
	},
	RetryDelay: providerRetryDelay,
}

var writeTicketBundleRefCountOptions = &refcount.Options{
	RetryBackoff: providerBackoff,
	ShouldRetry: func(err error) bool {
		return !isNonRetryableCloudError(err)
	},
	RetryDelay: providerRetryDelay,
}
