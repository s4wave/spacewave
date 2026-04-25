package provider_spacewave

import (
	"time"

	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/pkg/errors"
)

// cloudRetryAfter returns the server-provided retry delay, if any.
func cloudRetryAfter(err error) time.Duration {
	var ce *cloudError
	if !errors.As(err, &ce) || ce.RetryAfterSeconds == 0 {
		return 0
	}
	return time.Duration(ce.RetryAfterSeconds) * time.Second
}

// providerRetryDelay prefers the server-provided retry delay when it is longer
// than the local fallback backoff.
func providerRetryDelay(err error, fallback time.Duration) time.Duration {
	retryAfter := cloudRetryAfter(err)
	if retryAfter > 0 && (fallback == cbackoff.Stop || retryAfter > fallback) {
		return retryAfter
	}
	return fallback
}

// nextProviderRetryDelay advances the backoff and applies any server-provided
// retry-after hint.
func nextProviderRetryDelay(bo cbackoff.BackOff, err error) time.Duration {
	return providerRetryDelay(err, bo.NextBackOff())
}
