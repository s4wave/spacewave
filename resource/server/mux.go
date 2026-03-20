package resource_server

import "github.com/aperturerobotics/starpc/srpc"

// NewResourceMux creates a new srpc.Mux and registers services with it.
// Panics if any registration function returns an error.
func NewResourceMux(register ...func(srpc.Mux) error) srpc.Mux {
	mux := srpc.NewMux()
	for _, r := range register {
		if err := r(mux); err != nil {
			panic(err)
		}
	}
	return mux
}
