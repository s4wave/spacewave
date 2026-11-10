package packahead

import "github.com/pkg/errors"

// ErrClosed is returned by calls on a closed Log.
var ErrClosed = errors.New("packahead log is closed")

// ErrKeyTooShort is returned for a key shorter than keyTailLen bytes.
var ErrKeyTooShort = errors.New("packahead key is shorter than 16 bytes")

// ErrValueTooLarge is returned for a block that does not fit in an empty pack.
var ErrValueTooLarge = errors.New("packahead block does not fit in a pack")

// ErrReserve is returned by an Append that would leave less free disk than
// Options.ReserveBytes. It writes nothing; the same Append succeeds once the
// disk has room again.
var ErrReserve = errors.New("packahead log is below its disk reserve")
