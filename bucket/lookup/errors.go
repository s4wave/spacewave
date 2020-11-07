package bucket_lookup

import "errors"

var (
	// ErrEmptyBlockRef is returned if Lookup is called with an empty block ref.
	ErrEmptyBlockRef = errors.New("empty block reference")
)
