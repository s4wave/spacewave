package tptaddr

import "errors"

// ErrInvalidTptAddr is returned if the tptaddr was invalid.
var ErrInvalidTptAddr = errors.New("invalid transport address: expected transport-type-id|addr")
