//go:build alphadebug

package provider_spacewave_cacheseed

import (
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/core/provider/spacewave/cacheseedbuffer"
)

// Register installs the CacheSeedInspector service on mux, streaming from
// buf. Only compiled when the =alphadebug= build tag is set.
func Register(mux srpc.Mux, buf *cacheseedbuffer.Buffer) error {
	return SRPCRegisterCacheSeedInspector(mux, NewService(buf))
}
