package engine

import (
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
)

// metadataPrefix separates public metadata from block, graph, and journal keys.
const metadataPrefix = 0x01

// MetadataStore exposes the public metadata namespace. Transactions validate
// only the ranges they read, so block and GC writes never invalidate them.
func (e *Engine) MetadataStore() kvtx.Store {
	return kvtx_prefixer.NewPrefixer(e, []byte{metadataPrefix})
}
