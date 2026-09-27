//go:build !js

package spacewave_cli

import (
	emptypb "github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
)

// socketResourceWatch streams test events through the retained Resource client.
type socketResourceWatch struct {
	// events supplies one response per event.
	events <-chan struct{}
}

// GetServiceID identifies the Resource child service.
func (w *socketResourceWatch) GetServiceID() string { return "test.DesktopResourceWatch" }

// GetMethodIDs lists the Resource child stream.
func (w *socketResourceWatch) GetMethodIDs() []string { return []string{"Watch"} }

// InvokeMethod keeps one Resource stream live while desktop generations change.
func (w *socketResourceWatch) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Route only this Resource child method.
	if serviceID != w.GetServiceID() || methodID != "Watch" {
		return false, nil
	}
	if err := strm.MsgRecv(&emptypb.Empty{}); err != nil {
		return true, err
	}

	// Emit each event on the same stream until its client leaves.
	for {
		select {
		case _, ok := <-w.events:
			if !ok {
				return true, nil
			}
			if err := strm.MsgSend(&emptypb.Empty{}); err != nil {
				return true, err
			}
		case <-strm.Context().Done():
			return true, strm.Context().Err()
		}
	}
}

// _ is a type assertion.
var _ srpc.Invoker = (*socketResourceWatch)(nil)
