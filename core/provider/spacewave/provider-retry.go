package provider_spacewave

import (
	"time"

	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
)

// providerRetryDelay prefers the server-provided retry delay when it is longer
// than the local fallback backoff.
func providerRetryDelay(err error, fallback time.Duration) time.Duration {
	return clouderror.RetryDelay(err, fallback)
}

// nextProviderRetryDelay advances the backoff and applies any server-provided
// retry-after hint.
func nextProviderRetryDelay(bo cbackoff.BackOff, err error) time.Duration {
	return providerRetryDelay(err, bo.NextBackOff())
}
