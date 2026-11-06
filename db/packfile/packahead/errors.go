package packahead

import "github.com/pkg/errors"

// ErrClosed is returned by calls on a closed Log.
var ErrClosed = errors.New("packahead log is closed")

// ErrKeyTooShort is returned for a key shorter than keyTailLen bytes.
var ErrKeyTooShort = errors.New("packahead key is shorter than 16 bytes")

// ErrValueTooLarge is returned for a block that does not fit in an empty pack.
var ErrValueTooLarge = errors.New("packahead block does not fit in a pack")
