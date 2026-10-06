//go:build !js

package resource_root

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"github.com/sirupsen/logrus"
)

func TestParseWebListenSpec(t *testing.T) {
	// Parse a loopback listen address and verify its host and ephemeral port.
	spec, err := parseWebListenSpec("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatal(err)
	}
	if spec.host != "127.0.0.1" || spec.port != 0 {
		t.Fatalf("spec = %#v, want 127.0.0.1:0", spec)
	}

	// Verify web listen parsing rejects non-loopback and incomplete addresses.
	if _, err := parseWebListenSpec("/ip4/0.0.0.0/tcp/0"); err == nil {
		t.Fatal("expected non-loopback host error")
	}
	if _, err := parseWebListenSpec("/udp/0"); err == nil {
		t.Fatal("expected missing host/tcp error")
	}
}

func TestWebListenerServesHealth(t *testing.T) {
	// Start a localhost web listener for the health request.
	listener, err := newWebListener(t.Context(), logrus.NewEntry(logrus.New()), nil, nil, testWebListenSpec(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Fetch the listener health response and verify its status and body.
	resp, err := http.Get(listener.url + "/_spacewave/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("health response = %d %q", resp.StatusCode, string(body))
	}
}

func TestWebListenerServesBootShell(t *testing.T) {
	// Serve release boot metadata from a local upstream fixture.
	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			t.Fatalf("upstream path = %s, want /", req.URL.Path)
		}
		_, _ = rw.Write([]byte(`<!doctype html>
<script type="importmap">{"imports":{"react":"/entrypoint/release/pkgs/react/index.mjs"}}</script>
<link rel="stylesheet" href="/static/app.css"/>
<script type="module" src="/boot.mjs"></script>`))
	}))
	defer upstream.Close()
	t.Setenv("SPACEWAVE_WEB_ENDPOINT", upstream.URL)

	// Start the web listener against the release fixture.
	listener, err := newWebListener(t.Context(), logrus.NewEntry(logrus.New()), nil, nil, testWebListenSpec(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Fetch and read the listener boot shell.
	resp, err := http.Get(listener.url + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the shell includes release assets and capability bootstrap wiring.
	text := string(body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("boot shell status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(text, "/_spacewave/bootstrap") || !strings.Contains(text, "await import('/boot.mjs')") {
		t.Fatalf("boot shell missing bootstrap/release wiring: %s", text)
	}
	if !strings.Contains(text, `"react":"/entrypoint/release/pkgs/react/index.mjs"`) {
		t.Fatalf("boot shell missing release import map: %s", text)
	}
	if !strings.Contains(text, `/static/app.css`) {
		t.Fatalf("boot shell should defer release stylesheet until after bootstrap: %s", text)
	}
	if !strings.Contains(text, "if (otp)") {
		t.Fatalf("boot shell should allow reloads with an existing capability: %s", text)
	}
}

func TestWebListenerServesDisplayBootShellBeforeCapability(t *testing.T) {
	// Serve release boot metadata for the display route fixture.
	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			t.Fatalf("upstream path = %s, want /", req.URL.Path)
		}
		_, _ = rw.Write([]byte(`<!doctype html>
<script type="importmap">{"imports":{"react":"/entrypoint/release/pkgs/react/index.mjs"}}</script>
<script type="module" src="/boot.mjs"></script>`))
	}))
	defer upstream.Close()
	t.Setenv("SPACEWAVE_WEB_ENDPOINT", upstream.URL)

	// Start the web listener against the display fixture.
	listener, err := newWebListener(t.Context(), logrus.NewEntry(logrus.New()), nil, nil, testWebListenSpec(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Fetch and read the display boot shell before capability exchange.
	resp, err := http.Get(listener.url + "/display?path=docs%2Fhello")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the display shell includes the release bootstrap wiring.
	text := string(body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("display boot shell status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(text, "/_spacewave/bootstrap") || !strings.Contains(text, "await import('/boot.mjs')") {
		t.Fatalf("display boot shell missing bootstrap/release wiring: %s", text)
	}
}

func TestWebListenerBootstrapSetsSingleUseCapability(t *testing.T) {
	// Start a web listener for the single-use bootstrap exchange.
	listener, err := newWebListener(t.Context(), logrus.NewEntry(logrus.New()), nil, nil, testWebListenSpec(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Exchange the bootstrap secret and verify its bounded capability cookie.
	secret, err := listener.issueBootstrapSecret()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := exchangeWebBootstrapWithSecret(listener, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	cookie := findWebCapabilityCookie(resp.Cookies())
	if resp.StatusCode != http.StatusOK || cookie == nil {
		t.Fatalf("bootstrap response = %d cookie=%v, want 200 with capability cookie", resp.StatusCode, cookie)
	}
	if !cookie.HttpOnly || cookie.MaxAge <= 0 {
		t.Fatalf("capability cookie should be http-only and bounded: %#v", cookie)
	}

	// Verify a consumed bootstrap secret cannot be exchanged again.
	resp, err = exchangeWebBootstrapWithSecret(listener, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("second bootstrap status = %d, want 401", resp.StatusCode)
	}
}

func TestWebListenerGatesReleaseAssets(t *testing.T) {
	// Serve a release descriptor from a local upstream fixture.
	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/browser-release.json" {
			t.Fatalf("upstream path = %s, want /browser-release.json", req.URL.Path)
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"shellAssets":{"entrypoint":"app.js"}}`))
	}))
	defer upstream.Close()
	t.Setenv("SPACEWAVE_WEB_ENDPOINT", upstream.URL)

	// Start the web listener against the release descriptor fixture.
	listener, err := newWebListener(t.Context(), logrus.NewEntry(logrus.New()), nil, nil, testWebListenSpec(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Verify release assets reject a request without a capability.
	resp, err := http.Get(listener.url + "/browser-release.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ungated release response = %d, want 401", resp.StatusCode)
	}

	// Exchange a bootstrap secret for the release asset capability.
	resp, err = exchangeWebBootstrap(listener)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	cookie := findWebCapabilityCookie(resp.Cookies())
	if cookie == nil {
		t.Fatal("missing capability cookie")
	}

	// Request the release descriptor with the capability cookie.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, listener.url+"/browser-release.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the authorized request receives the upstream descriptor.
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "app.js") {
		t.Fatalf("release response = %d %q, want proxied descriptor", resp.StatusCode, string(body))
	}
}

func TestWebListenerRegistryReusesPortZeroHostname(t *testing.T) {
	// Create a listener registry for ephemeral-port reuse.
	reg := newWebListenerRegistry(logrus.NewEntry(logrus.New()))

	// Allocate the first listener and verify it was created.
	a, reused, err := reg.access(t.Context(), nil, nil, testWebListenSpec(t, "/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if reused {
		t.Fatal("first listener should not be reused")
	}

	// Access the same listen address and verify its listener is reused.
	b, reused, err := reg.access(t.Context(), nil, nil, testWebListenSpec(t, "/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	if !reused {
		t.Fatal("second port-0 listener should be reused")
	}
	if a != b {
		t.Fatal("expected registry to return the existing listener")
	}

	// Verify each listener response issues a fresh bootstrap secret.
	aResp, err := a.response(0, false)
	if err != nil {
		t.Fatal(err)
	}
	bResp, err := b.response(0, true)
	if err != nil {
		t.Fatal(err)
	}
	if aResp.GetBootstrapSecret() == bResp.GetBootstrapSecret() {
		t.Fatal("reused listener should issue a fresh bootstrap secret")
	}
}

// testWebListenSpec parses an unbound listen address.
func testWebListenSpec(t *testing.T, listenMultiaddr string) *webListenSpec {
	t.Helper()
	spec, err := parseWebListenSpec(listenMultiaddr)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func exchangeWebBootstrap(listener *webListener) (*http.Response, error) {
	secret, err := listener.issueBootstrapSecret()
	if err != nil {
		return nil, err
	}
	return exchangeWebBootstrapWithSecret(listener, secret)
}

func exchangeWebBootstrapWithSecret(listener *webListener, secret string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, listener.url+"/_spacewave/bootstrap", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Spacewave-Bootstrap", secret)
	return http.DefaultClient.Do(req)
}

func findWebCapabilityCookie(cookies []*http.Cookie) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == webCapabilityCookie {
			return cookie
		}
	}
	return nil
}

func TestWebListenerRegistryDoesNotReuseExplicitPort(t *testing.T) {
	// Create a listener registry for explicit-port allocation.
	reg := newWebListenerRegistry(logrus.NewEntry(logrus.New()))

	// Allocate a listener whose port remains in use.
	a, reused, err := reg.access(t.Context(), nil, nil, testWebListenSpec(t, "/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if reused {
		t.Fatal("first listener should not be reused")
	}

	// Verify an explicit request for the occupied port fails without reuse.
	parts := strings.Split(a.listenMultiaddr, "/tcp/")
	if len(parts) != 2 {
		t.Fatalf("unexpected listener multiaddr: %s", a.listenMultiaddr)
	}
	if _, reused, err := reg.access(t.Context(), nil, nil, testWebListenSpec(t, "/ip4/127.0.0.1/tcp/"+parts[1])); err == nil || reused {
		t.Fatalf("explicit port should allocate distinctly and fail while in use, reused=%v err=%v", reused, err)
	}
}

func TestWebListenerRegistryRetainsExplicitPort(t *testing.T) {
	// Find an available TCP port for the explicit listener fixture.
	reg := newWebListenerRegistry(logrus.NewEntry(logrus.New()))
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}

	// Allocate the explicit-port listener and verify it is new.
	listener, reused, err := reg.access(t.Context(), nil, nil, testWebListenSpec(t, "/ip4/127.0.0.1/tcp/"+strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if reused {
		t.Fatal("explicit listener should not be reused")
	}

	// Verify the registry retains the explicit background listener.
	var found bool
	reg.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, existing := range reg.listeners {
			if existing == listener {
				found = true
			}
		}
	})
	if !found {
		t.Fatal("explicit background listener should be retained by the registry")
	}
}

func TestAccessWebListenerBackgroundResponseIncludesLifecycleData(t *testing.T) {
	// Create a root server for background listener access.
	server := NewCoreRootServer(logrus.NewEntry(logrus.New()), nil)
	defer server.Close()

	// Access a background listener and verify its lifecycle response.
	resp, err := server.AccessWebListener(t.Context(), &s4wave_root.AccessWebListenerRequest{
		ListenMultiaddr: "/ip4/127.0.0.1/tcp/0",
		Background:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetResourceId() != 0 {
		t.Fatalf("background resource id = %d, want 0", resp.GetResourceId())
	}
	if resp.GetListenerId() == "" {
		t.Fatal("missing listener id")
	}
	if !strings.Contains(resp.GetListenMultiaddr(), "/tcp/") {
		t.Fatalf("listen multiaddr = %q, want resolved tcp multiaddr", resp.GetListenMultiaddr())
	}

	// Verify the response URL, bootstrap secret, and initial reuse status.
	if !strings.HasPrefix(resp.GetUrl(), "http://") {
		t.Fatalf("url = %q, want http:// URL", resp.GetUrl())
	}
	if resp.GetBootstrapSecret() == "" {
		t.Fatal("missing bootstrap secret")
	}
	if resp.GetReused() {
		t.Fatal("first listener should not be reused")
	}

	// Verify a second access reuses the listener and issues a fresh secret.
	reused, err := server.AccessWebListener(t.Context(), &s4wave_root.AccessWebListenerRequest{
		ListenMultiaddr: "/ip4/127.0.0.1/tcp/0",
		Background:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reused.GetReused() {
		t.Fatal("second listener should be reused")
	}
	if reused.GetListenerId() != resp.GetListenerId() {
		t.Fatalf("reused listener id = %q, want %q", reused.GetListenerId(), resp.GetListenerId())
	}
	if reused.GetBootstrapSecret() == resp.GetBootstrapSecret() {
		t.Fatal("reused listener should return a fresh bootstrap secret")
	}
}

func TestRootServerListsAndStopsBackgroundWebListeners(t *testing.T) {
	// Create a root server for background listener lifecycle checks.
	server := NewCoreRootServer(logrus.NewEntry(logrus.New()), nil)
	defer server.Close()

	// Access a background listener through the root service.
	resp, err := server.AccessWebListener(t.Context(), &s4wave_root.AccessWebListenerRequest{
		ListenMultiaddr: "/ip4/127.0.0.1/tcp/0",
		Background:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the listener appears in the background registry.
	listeners := server.webListeners.list()
	if len(listeners) != 1 {
		t.Fatalf("listeners = %d, want 1", len(listeners))
	}
	if listeners[0].GetListenerId() != resp.GetListenerId() {
		t.Fatalf("listener id = %q, want %q", listeners[0].GetListenerId(), resp.GetListenerId())
	}
	if !listeners[0].GetBackground() {
		t.Fatal("listed listener should be background-owned")
	}

	// Verify stopping an unknown listener reports it missing.
	missing, err := server.StopWebListener(t.Context(), &s4wave_root.StopWebListenerRequest{
		ListenerId: "missing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !missing.GetNotFound() {
		t.Fatal("missing listener should report not found")
	}

	// Stop the registered listener through the root service.
	stopped, err := server.StopWebListener(t.Context(), &s4wave_root.StopWebListenerRequest{
		ListenerId: resp.GetListenerId(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if stopped.GetNotFound() {
		t.Fatal("existing listener should stop")
	}

	// Verify the stopped listener is removed from the registry.
	listeners = server.webListeners.list()
	if len(listeners) != 0 {
		t.Fatalf("listeners after stop = %d, want 0", len(listeners))
	}
}
