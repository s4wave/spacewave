package assembly

import "errors"

var (
	// ErrEmptyAssembly is returned if a non-empty Assembly was required.
	ErrEmptyAssembly = errors.New("assembly cannot be empty")
)
