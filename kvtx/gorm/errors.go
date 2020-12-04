package kvtx_gorm

import "errors"

var (
	// ErrConstraintsNotImplemented is returned for the constraints calls.
	ErrConstraintsNotImplemented = errors.New("constraints not implemented")
)
