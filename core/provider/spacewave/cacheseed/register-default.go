//go:build !alphadebug

package provider_spacewave_cacheseed

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/provider/spacewave/cacheseedbuffer"
)

// Register is a noop in production builds. The CacheSeedInspector service is
// only registered when the =alphadebug= build tag is set.
func Register(mux srpc.Mux, buf *cacheseedbuffer.Buffer) error {
	return nil
}
