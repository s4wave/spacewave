package object

import (
	"github.com/aperturerobotics/hydra/kvtx/prefixer"
)

// Prefixer implements an object store prefixer.
type Prefixer = kvtx_prefixer.Prefixer

// NewPrefixer constructs a new object store prefixer.
func NewPrefixer(base ObjectStore, prefix []byte) ObjectStore {
	return kvtx_prefixer.NewPrefixer(base, prefix)
}
