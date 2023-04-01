package plugin_entrypoint_controller

import cbackoff "github.com/cenkalti/backoff"
import "github.com/aperturerobotics/util/backoff"

func buildBackoff() cbackoff.BackOff {
	return (&backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		Exponential: &backoff.Exponential{
			InitialInterval: 100,
			MaxInterval:     1200,
		},
	}).Construct()
}
