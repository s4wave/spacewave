//go:build !js

package resource_root

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"github.com/sirupsen/logrus"
)

func TestParseWebListenRequestFull(t *testing.T) {
	// Reject a full listener bound to a Space.
	if _, err := parseWebListenRequest(&s4wave_root.AccessWebListenerRequest{Full: true, SpaceId: "space", SessionIdx: 1}); err == nil {
		t.Fatal("full listener bound to a space should be rejected")
	}

	// Keep full and unbound listeners on separate reuse keys.
	full, err := parseWebListenRequest(&s4wave_root.AccessWebListenerRequest{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	unbound, err := parseWebListenRequest(&s4wave_root.AccessWebListenerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if full.reuseKey() == unbound.reuseKey() {
		t.Fatalf("full and unbound listeners share reuse key %q", full.reuseKey())
	}
}

func TestWebListenerFullResourceService(t *testing.T) {
	// Serve a Root with a test handler.
	rootHandler := &testRootHandler{}
	rootMux := srpc.NewMux()
	if err := rootMux.Register(rootHandler); err != nil {
		t.Fatal(err)
	}

	// Start a full listener over that Root.
	spec, err := parseWebListenRequest(&s4wave_root.AccessWebListenerRequest{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := newWebListener(t.Context(), logrus.NewEntry(logrus.New()), nil, rootMux, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	wsURL := "ws" + strings.TrimPrefix(listener.url, "http") + webResourcePath

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

	// Refuse a page on another local port even with the cookie.
	ctx := t.Context()
	if _, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": {cookie.String()}, "Origin": {"http://127.0.0.1:1"}},
	}); err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin websocket = %v %v, want 403", resp, err)
	}

	// Open the Resource service from the listener's own origin.
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": {cookie.String()}, "Origin": {listener.url}},
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

	// Reach the Root through the unfiltered Resource service.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	root := s4wave_root.NewSRPCRootResourceServiceClient(rootClient)

	// Serve any session and the methods a binding denies.
	if _, err := root.MountSessionByIdx(ctx, &s4wave_root.MountSessionByIdxRequest{SessionIdx: 2}); err != nil {
		t.Fatalf("session 2: %v", err)
	}
	if _, err := root.AccessWebListener(ctx, &s4wave_root.AccessWebListenerRequest{}); err != nil {
		t.Fatalf("AccessWebListener: %v", err)
	}
	if calls := rootHandler.calls.Load(); calls != 2 {
		t.Fatalf("root handler calls = %d, want 2", calls)
	}
}

// TestRenderFullBootShell checks a full listener loads the native app without
// a forced route.
func TestRenderFullBootShell(t *testing.T) {
	// Render the shell for a full listener.
	metadata := &webListenerReleaseBootMetadata{importMapScript: `<script type="importmap">{"imports":{}}</script>`}
	spec := &webListenSpec{full: true}
	shell, err := renderWebListenerBootShell(metadata, spec, "/b/pa/spacewave-app/manifest/root/v/b/fe/", "/b/pa/spacewave-web/manifest/web-root/pkgs/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Load the native app without the WASM runtime or a route.
	text := string(shell)
	if !strings.Contains(text, "renderBoundApp(") {
		t.Fatalf("full boot shell should render the native app: %s", text)
	}
	if strings.Contains(text, "/boot.mjs") || strings.Contains(text, "/u/") {
		t.Fatalf("full boot shell should neither start the WASM runtime nor force a route: %s", text)
	}
}
