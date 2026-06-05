//go:build goscript

package s4wave_layout

import (
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
)

type layoutHostRegistrationTestServer struct {
	watched       bool
	navigateTabID string
	replaceTabID  string
	addTabID      string
}

func (s *layoutHostRegistrationTestServer) WatchLayoutModel(SRPCLayoutHost_WatchLayoutModelStream) error {
	s.watched = true
	return nil
}

func (s *layoutHostRegistrationTestServer) NavigateTab(_ context.Context, req *NavigateTabRequest) (*NavigateTabResponse, error) {
	s.navigateTabID = req.GetTabId()
	return &NavigateTabResponse{}, nil
}

func (s *layoutHostRegistrationTestServer) ReplaceTab(_ context.Context, req *ReplaceTabRequest) (*ReplaceTabResponse, error) {
	s.replaceTabID = req.GetTabId()
	return &ReplaceTabResponse{}, nil
}

func (s *layoutHostRegistrationTestServer) AddTab(_ context.Context, req *AddTabRequest) (*AddTabResponse, error) {
	s.addTabID = req.GetTab().GetId()
	return &AddTabResponse{TabId: s.addTabID}, nil
}

type layoutHostTestStream struct {
	ctx      context.Context
	recvData []byte
	sentData []byte
}

func newLayoutHostTestStream(msg srpc.Message) *layoutHostTestStream {
	data, err := msg.MarshalVT()
	if err != nil {
		panic(err)
	}
	return &layoutHostTestStream{
		ctx:      context.Background(),
		recvData: data,
	}
}

func (s *layoutHostTestStream) Context() context.Context {
	return s.ctx
}

func (s *layoutHostTestStream) MsgSend(msg srpc.Message) error {
	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}
	s.sentData = data
	return nil
}

func (s *layoutHostTestStream) MsgRecv(msg srpc.Message) error {
	return msg.UnmarshalVT(s.recvData)
}

func (s *layoutHostTestStream) CloseSend() error {
	return nil
}

func (s *layoutHostTestStream) Close() error {
	return nil
}

func TestGoScriptLayoutHostRegistration(t *testing.T) {
	mux := srpc.NewMux()
	server := &layoutHostRegistrationTestServer{}
	if err := SRPCRegisterLayoutHost(mux, server); err != nil {
		t.Fatalf("register LayoutHost: %v", err)
	}

	if !mux.HasService(SRPCLayoutHostServiceID) {
		t.Fatalf("expected mux to register %s", SRPCLayoutHostServiceID)
	}

	handler := NewSRPCLayoutHostHandler(server, "")
	methods := handler.GetMethodIDs()
	for _, method := range []string{"WatchLayoutModel", "NavigateTab", "ReplaceTab", "AddTab"} {
		if !slices.Contains(methods, method) {
			t.Fatalf("expected LayoutHost handler methods to include %s: %v", method, methods)
		}
	}

	if ok, err := handler.InvokeMethod(SRPCLayoutHostServiceID, "WatchLayoutModel", newLayoutHostTestStream(&WatchLayoutModelRequest{})); err != nil || !ok {
		t.Fatalf("invoke WatchLayoutModel ok=%v err=%v", ok, err)
	}
	if !server.watched {
		t.Fatal("expected WatchLayoutModel to dispatch to server")
	}

	if ok, err := handler.InvokeMethod(SRPCLayoutHostServiceID, "NavigateTab", newLayoutHostTestStream(&NavigateTabRequest{TabId: "nav-tab"})); err != nil || !ok {
		t.Fatalf("invoke NavigateTab ok=%v err=%v", ok, err)
	}
	if server.navigateTabID != "nav-tab" {
		t.Fatalf("expected NavigateTab request tab id, got %q", server.navigateTabID)
	}

	if ok, err := handler.InvokeMethod(SRPCLayoutHostServiceID, "ReplaceTab", newLayoutHostTestStream(&ReplaceTabRequest{TabId: "replace-tab"})); err != nil || !ok {
		t.Fatalf("invoke ReplaceTab ok=%v err=%v", ok, err)
	}
	if server.replaceTabID != "replace-tab" {
		t.Fatalf("expected ReplaceTab request tab id, got %q", server.replaceTabID)
	}

	addStream := newLayoutHostTestStream(&AddTabRequest{Tab: &TabDef{Id: "add-tab"}})
	if ok, err := handler.InvokeMethod(SRPCLayoutHostServiceID, "AddTab", addStream); err != nil || !ok {
		t.Fatalf("invoke AddTab ok=%v err=%v", ok, err)
	}
	var addResp AddTabResponse
	if err := addResp.UnmarshalVT(addStream.sentData); err != nil {
		t.Fatalf("unmarshal AddTab response: %v", err)
	}
	if addResp.GetTabId() != "add-tab" {
		t.Fatalf("expected AddTab response tab id, got %q", addResp.GetTabId())
	}
}
