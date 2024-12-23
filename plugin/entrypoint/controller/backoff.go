package plugin_entrypoint_controller

import (
	"github.com/aperturerobotics/util/backoff"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
)

func buildBackoff() cbackoff.BackOff {
	return (&backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		Exponential: &backoff.Exponential{
			InitialInterval: 100,
			MaxInterval:     1200,
		},
	}).Construct()
}
