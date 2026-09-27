//go:build !js

package spacewave_cli

import (
	"io"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
)

// desktopQuitAckHandler closes admission before sending a desktop Quit reply.
type desktopQuitAckHandler struct {
	// requestShutdown claims listener shutdown for the requesting connection.
	requestShutdown func(*trackedConn)
}

// GetServiceID identifies the focused desktop Quit acknowledgement fixture.
func (*desktopQuitAckHandler) GetServiceID() string { return "test.DesktopQuitAck" }

// GetMethodIDs exposes the focused desktop Quit acknowledgement fixture.
func (*desktopQuitAckHandler) GetMethodIDs() []string { return []string{"Quit"} }

// InvokeMethod claims listener shutdown and then acknowledges the requester.
func (h *desktopQuitAckHandler) InvokeMethod(serviceID, methodID string, stream srpc.Stream) (bool, error) {
	if serviceID != h.GetServiceID() || methodID != "Quit" {
		return false, nil
	}
	if err := stream.MsgRecv(&emptypb.Empty{}); err != nil && err != io.EOF {
		return true, err
	}
	requester := stream.Context().Value(daemonConnCtxKey{}).(*trackedConn)
	h.requestShutdown(requester)
	if err := stream.MsgSend(&emptypb.Empty{}); err != nil {
		return true, err
	}
	return true, stream.CloseSend()
}

// _ is a type assertion.
var _ srpc.Handler = (*desktopQuitAckHandler)(nil)
