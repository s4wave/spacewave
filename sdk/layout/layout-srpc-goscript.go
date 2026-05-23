//go:build goscript

package s4wave_layout

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
)

// SRPCLayoutHostServer is the GoScript-visible layout host server interface.
type SRPCLayoutHostServer interface {
	WatchLayoutModel(SRPCLayoutHost_WatchLayoutModelStream) error

	NavigateTab(context.Context, *NavigateTabRequest) (*NavigateTabResponse, error)

	AddTab(context.Context, *AddTabRequest) (*AddTabResponse, error)
}

// SRPCLayoutHost_WatchLayoutModelStream is the GoScript-visible layout model stream.
type SRPCLayoutHost_WatchLayoutModelStream interface {
	srpc.Stream
	Send(*LayoutModel) error
	SendAndClose(*LayoutModel) error
	Recv() (*WatchLayoutModelRequest, error)
	RecvTo(*WatchLayoutModelRequest) error
}

// SRPCRegisterLayoutHost registers a layout host with an SRPC mux.
func SRPCRegisterLayoutHost(mux srpc.Mux, impl SRPCLayoutHostServer) error {
	return nil
}
