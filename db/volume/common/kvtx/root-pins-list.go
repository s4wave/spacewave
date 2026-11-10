package kvtx

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// RootPin is a root this process holds outside every named root: the root a
// reader pinned, or a block an open stage owns.
type RootPin struct {
	// Root is the pinned root or the stage's block.
	Root *block.BlockRef
	// Stage is the owning stage's node, or empty for a reader pin.
	Stage string
	// Opened is when the pin took its first reader, or the stage opened.
	Opened time.Time
}

// ListRootPins returns the roots this process holds outside every named root.
// It does not list them yet.
func (v *Volume) ListRootPins(context.Context) ([]RootPin, error) {
	return nil, nil
}
