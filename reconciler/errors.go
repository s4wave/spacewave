package reconciler

import "errors"

// ErrReconcilerIDEmpty is returned if the reconciler id was empty.
var ErrReconcilerIDEmpty = errors.New("reconciler id cannot be empty")
