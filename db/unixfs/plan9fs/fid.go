package plan9fs

import (
	"errors"

	"github.com/s4wave/spacewave/db/unixfs"
)

// errBadFid is returned when a fid is not found.
var errBadFid = errors.New("fid not found")

// errFidInUse is returned when a fid is already in use.
var errFidInUse = errors.New("fid already in use")

// Fid is an active 9p file identifier bound to an FSHandle.
type Fid struct {
	id     uint32
	handle *unixfs.FSHandle
	uid    uint32
	opened bool
}
