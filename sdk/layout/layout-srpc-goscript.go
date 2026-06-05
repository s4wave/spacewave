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

	ReplaceTab(context.Context, *ReplaceTabRequest) (*ReplaceTabResponse, error)

	AddTab(context.Context, *AddTabRequest) (*AddTabResponse, error)
}

const SRPCLayoutHostServiceID = "s4wave.layout.LayoutHost"

type SRPCLayoutHostHandler struct {
	serviceID string
	impl      SRPCLayoutHostServer
}

// NewSRPCLayoutHostHandler constructs a new RPC handler.
func NewSRPCLayoutHostHandler(impl SRPCLayoutHostServer, serviceID string) srpc.Handler {
	if serviceID == "" {
		serviceID = SRPCLayoutHostServiceID
	}
	return &SRPCLayoutHostHandler{impl: impl, serviceID: serviceID}
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
	return mux.Register(NewSRPCLayoutHostHandler(impl, ""))
}

func (d *SRPCLayoutHostHandler) GetServiceID() string { return d.serviceID }

func (SRPCLayoutHostHandler) GetMethodIDs() []string {
	return []string{
		"WatchLayoutModel",
		"NavigateTab",
		"ReplaceTab",
		"AddTab",
	}
}

func (d *SRPCLayoutHostHandler) InvokeMethod(
	serviceID, methodID string,
	strm srpc.Stream,
) (bool, error) {
	if serviceID != "" && serviceID != d.GetServiceID() {
		return false, nil
	}

	switch methodID {
	case "WatchLayoutModel":
		return true, d.InvokeMethod_WatchLayoutModel(d.impl, strm)
	case "NavigateTab":
		return true, d.InvokeMethod_NavigateTab(d.impl, strm)
	case "ReplaceTab":
		return true, d.InvokeMethod_ReplaceTab(d.impl, strm)
	case "AddTab":
		return true, d.InvokeMethod_AddTab(d.impl, strm)
	default:
		return false, nil
	}
}

func (SRPCLayoutHostHandler) InvokeMethod_WatchLayoutModel(impl SRPCLayoutHostServer, strm srpc.Stream) error {
	clientStrm := &srpcLayoutHost_WatchLayoutModelStream{strm}
	return impl.WatchLayoutModel(clientStrm)
}

func (SRPCLayoutHostHandler) InvokeMethod_NavigateTab(impl SRPCLayoutHostServer, strm srpc.Stream) error {
	req := new(NavigateTabRequest)
	if err := strm.MsgRecv(req); err != nil {
		return err
	}
	out, err := impl.NavigateTab(strm.Context(), req)
	if err != nil {
		return err
	}
	return strm.MsgSend(out)
}

func (SRPCLayoutHostHandler) InvokeMethod_ReplaceTab(impl SRPCLayoutHostServer, strm srpc.Stream) error {
	req := new(ReplaceTabRequest)
	if err := strm.MsgRecv(req); err != nil {
		return err
	}
	out, err := impl.ReplaceTab(strm.Context(), req)
	if err != nil {
		return err
	}
	return strm.MsgSend(out)
}

func (SRPCLayoutHostHandler) InvokeMethod_AddTab(impl SRPCLayoutHostServer, strm srpc.Stream) error {
	req := new(AddTabRequest)
	if err := strm.MsgRecv(req); err != nil {
		return err
	}
	out, err := impl.AddTab(strm.Context(), req)
	if err != nil {
		return err
	}
	return strm.MsgSend(out)
}

type srpcLayoutHost_WatchLayoutModelStream struct {
	srpc.Stream
}

func (x *srpcLayoutHost_WatchLayoutModelStream) Send(m *LayoutModel) error {
	return x.MsgSend(m)
}

func (x *srpcLayoutHost_WatchLayoutModelStream) SendAndClose(m *LayoutModel) error {
	if m != nil {
		if err := x.MsgSend(m); err != nil {
			return err
		}
	}
	return x.CloseSend()
}

func (x *srpcLayoutHost_WatchLayoutModelStream) Recv() (*WatchLayoutModelRequest, error) {
	m := new(WatchLayoutModelRequest)
	if err := x.MsgRecv(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (x *srpcLayoutHost_WatchLayoutModelStream) RecvTo(m *WatchLayoutModelRequest) error {
	return x.MsgRecv(m)
}
