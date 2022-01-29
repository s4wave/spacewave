package reconciler

import "errors"

var (
	// ErrReconcilerIDEmpty is returned if the reconciler id was empty.
	ErrReconcilerIDEmpty = errors.New("reconciler id cannot be empty")
)
