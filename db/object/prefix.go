package object

import (
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
)

// Prefixer implements an object store prefixer.
type Prefixer = kvtx_prefixer.Prefixer

// NewPrefixer constructs a new object store prefixer.
func NewPrefixer(base ObjectStore, prefix []byte) ObjectStore {
	return kvtx_prefixer.NewPrefixer(base, prefix)
}
