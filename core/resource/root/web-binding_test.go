//go:build !js

package resource_root

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/sobject"
	space "github.com/s4wave/spacewave/core/space"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// testRootHandler answers MountSessionByIdx and AccessWebListener on the Root
// service and counts the calls that reach it.
type testRootHandler struct {
	calls atomic.Int32
}

func (h *testRootHandler) GetServiceID() string {
	return s4wave_root.SRPCRootResourceServiceServiceID
}

func (h *testRootHandler) GetMethodIDs() []string {
	return []string{"MountSessionByIdx", "AccessWebListener"}
}

func (h *testRootHandler) InvokeMethod(_, methodID string, strm srpc.Stream) (bool, error) {
	switch methodID {
	case "MountSessionByIdx":
		req := &s4wave_root.MountSessionByIdxRequest{}
		if err := strm.MsgRecv(req); err != nil {
			return true, err
		}
		h.calls.Add(1)
		return true, strm.MsgSend(&s4wave_root.MountSessionByIdxResponse{NotFound: true})
	case "AccessWebListener":
		h.calls.Add(1)
		return true, strm.MsgSend(&s4wave_root.AccessWebListenerResponse{})
	}
	return false, nil
}

func TestParseWebListenRequestBinding(t *testing.T) {
	// Reject a binding that names only a Space or only a session.
	for _, req := range []*s4wave_root.AccessWebListenerRequest{
		{SpaceId: "space"},
		{SessionIdx: 1},
	} {
		if _, err := parseWebListenRequest(req); err == nil {
			t.Fatalf("binding %v should be rejected", req)
		}
	}

	// Keep bound and unbound listeners on separate reuse keys.
	bound, err := parseWebListenRequest(&s4wave_root.AccessWebListenerRequest{SpaceId: "space", SessionIdx: 1})
	if err != nil {
		t.Fatal(err)
	}
	unbound, err := parseWebListenRequest(&s4wave_root.AccessWebListenerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if bound.reuseKey() == unbound.reuseKey() {
		t.Fatalf("bound and unbound listeners share reuse key %q", bound.reuseKey())
	}
}

func TestWebListenerBoundResourceService(t *testing.T) {
	// Serve a Root with a test handler.
	rootHandler := &testRootHandler{}
	rootMux := srpc.NewMux()
	if err := rootMux.Register(rootHandler); err != nil {
		t.Fatal(err)
	}

	// Start a listener bound to session 1 over that Root.
	spec, err := parseWebListenRequest(&s4wave_root.AccessWebListenerRequest{SpaceId: "space", SessionIdx: 1})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := newWebListener(t.Context(), logrus.NewEntry(logrus.New()), nil, rootMux, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	wsURL := "ws" + strings.TrimPrefix(listener.url, "http") + webResourcePath

	// Refuse the websocket without the capability cookie.
	ctx := t.Context()
	if _, resp, err := websocket.Dial(ctx, wsURL, nil); err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("websocket without capability = %v %v, want 401", resp, err)
	}

	// Exchange a bootstrap secret for the capability cookie.
	resp, err := exchangeWebBootstrap(listener)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cookie := findWebCapabilityCookie(resp.Cookies())
	if cookie == nil {
		t.Fatal("missing capability cookie")
	}

	// Open the Resource service over the websocket with the cookie.
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": {cookie.String()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	muxed, err := srpc.NewWebSocketConn(ctx, conn, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	resClient, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(srpc.NewClientWithMuxedConn(muxed)))
	if err != nil {
		t.Fatal(err)
	}
	defer resClient.Release()

	// Reach the Root through the filtered Resource service.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	root := s4wave_root.NewSRPCRootResourceServiceClient(rootClient)

	// Serve the bound session.
	if _, err := root.MountSessionByIdx(ctx, &s4wave_root.MountSessionByIdxRequest{SessionIdx: 1}); err != nil {
		t.Fatalf("bound session: %v", err)
	}

	// Deny another session and a method outside the binding.
	if _, err := root.MountSessionByIdx(ctx, &s4wave_root.MountSessionByIdxRequest{SessionIdx: 2}); err == nil {
		t.Fatal("other session should be denied")
	}
	if _, err := root.AccessWebListener(ctx, &s4wave_root.AccessWebListenerRequest{}); err == nil {
		t.Fatal("AccessWebListener should be denied")
	}
	if calls := rootHandler.calls.Load(); calls != 1 {
		t.Fatalf("root handler calls = %d, want 1", calls)
	}
}

// TestRenderBoundBootShell checks native app and package release bindings.
func TestRenderBoundBootShell(t *testing.T) {
	// Render the shell for a listener bound to session 2 and one Space.
	metadata := &webListenerReleaseBootMetadata{importMapScript: `<script type="importmap">{}</script>`}
	spec := &webListenSpec{spaceID: "space/1", sessionIdx: 2}
	shell, err := renderWebListenerBootShell(metadata, spec, "/b/pa/spacewave-app/manifest/root/v/b/fe/", "/b/pa/spacewave-web/manifest/web-root/pkgs/")
	if err != nil {
		t.Fatal(err)
	}

	// Load the native app over the Resource websocket, not the WASM runtime.
	text := string(shell)
	for _, want := range []string{
		`"/u/2/so/space%2F1"`,
		`"/b/pa/spacewave-app/manifest/root/v/b/fe/"`,
		`"bldr-web-pkg/":"/b/pa/spacewave-web/manifest/web-root/pkgs/"`,
		`renderBoundApp(`,
		`"/_spacewave/resource"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("bound boot shell missing %s: %s", want, text)
		}
	}
	if strings.Contains(text, "/boot.mjs") {
		t.Fatalf("bound boot shell should not start the WASM runtime: %s", text)
	}
}

// testSendStream records the messages a handler sends.
type testSendStream struct {
	srpc.Stream
	sent []srpc.Message
}

func (s *testSendStream) MsgSend(msg srpc.Message) error {
	s.sent = append(s.sent, msg)
	return nil
}

// testSpacesHandler answers WatchResourcesList with a fixed Space list.
type testSpacesHandler struct {
	spaces []*space.SpaceSoListEntry
}

func (h *testSpacesHandler) InvokeMethod(_, _ string, strm srpc.Stream) (bool, error) {
	return true, strm.MsgSend(&s4wave_session.WatchResourcesListResponse{SpacesList: h.spaces})
}

func TestWebBindingNarrowsSpacesList(t *testing.T) {
	// List two Spaces in the session.
	spaceEntry := func(id string) *space.SpaceSoListEntry {
		return &space.SpaceSoListEntry{Entry: &sobject.SharedObjectListEntry{
			Ref: &sobject.SharedObjectRef{ProviderResourceRef: &provider.ProviderResourceRef{Id: id}},
		}}
	}
	handler := &testSpacesHandler{spaces: []*space.SpaceSoListEntry{spaceEntry("bound"), spaceEntry("other")}}

	// Watch the list through a binding to one of them.
	binding := &webBinding{le: logrus.NewEntry(logrus.New()), sessionIdx: 1, spaceID: "bound"}
	strm := &testSendStream{}
	if _, err := binding.filter(handler).InvokeMethod(s4wave_session.SRPCSessionResourceServiceServiceID, "WatchResourcesList", strm); err != nil {
		t.Fatal(err)
	}

	// Send only the bound Space and leave the handler's list intact.
	if len(strm.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(strm.sent))
	}
	got := strm.sent[0].(*s4wave_session.WatchResourcesListResponse).GetSpacesList()
	if len(got) != 1 || got[0].GetEntry().GetRef().GetProviderResourceRef().GetId() != "bound" {
		t.Fatalf("spaces list = %v, want only the bound Space", got)
	}
	if len(handler.spaces) != 2 || handler.spaces[1].GetEntry().GetRef().GetProviderResourceRef().GetId() != "other" {
		t.Fatalf("handler list changed: %v", handler.spaces)
	}
}
