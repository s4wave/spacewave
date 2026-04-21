package keypem

import "errors"

// ErrUnexpectedPemType is returned for an unexpected pem type.
var ErrUnexpectedPemType = errors.New("keypem: unexpected pem type")
