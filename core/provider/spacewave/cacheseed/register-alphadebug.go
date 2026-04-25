//go:build alphadebug

package provider_spacewave_cacheseed

import (
	"github.com/aperturerobotics/starpc/srpc"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
)

// Register installs the CacheSeedInspector service on mux, streaming from
// buf. Only compiled when the =alphadebug= build tag is set.
func Register(mux srpc.Mux, buf *provider_spacewave.CacheSeedBuffer) error {
	return SRPCRegisterCacheSeedInspector(mux, NewService(buf))
}
