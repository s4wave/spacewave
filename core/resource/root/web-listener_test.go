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
	spec, err := parseWebListenSpec("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatal(err)
	}
	if spec.host != "127.0.0.1" || spec.port != 0 {
		t.Fatalf("spec = %#v, want 127.0.0.1:0", spec)
	}

	if _, err := parseWebListenSpec("/ip4/0.0.0.0/tcp/0"); err == nil {
		t.Fatal("expected non-loopback host error")
	}
	if _, err := parseWebListenSpec("/udp/0"); err == nil {
		t.Fatal("expected missing host/tcp error")
	}
}

func TestWebListenerServesHealth(t *testing.T) {
	listener, err := newWebListener(t.Context(), logrus.NewEntry(logrus.New()), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

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
	listener, err := newWebListener(t.Context(), logrus.NewEntry(logrus.New()), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	resp, err := http.Get(listener.url + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("boot shell status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(text, "/_spacewave/bootstrap") || !strings.Contains(text, "/boot.mjs") {
		t.Fatalf("boot shell missing bootstrap/release wiring: %s", text)
	}
	if !strings.Contains(text, "if (otp)") {
		t.Fatalf("boot shell should allow reloads with an existing capability: %s", text)
	}
}

func TestWebListenerBootstrapSetsSingleUseCapability(t *testing.T) {
	listener, err := newWebListener(t.Context(), logrus.NewEntry(logrus.New()), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

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
	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/browser-release.json" {
			t.Fatalf("upstream path = %s, want /browser-release.json", req.URL.Path)
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"shellAssets":{"entrypoint":"app.js"}}`))
	}))
	defer upstream.Close()
	t.Setenv("SPACEWAVE_WEB_ENDPOINT", upstream.URL)

	listener, err := newWebListener(t.Context(), logrus.NewEntry(logrus.New()), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	resp, err := http.Get(listener.url + "/browser-release.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ungated release response = %d, want 401", resp.StatusCode)
	}

	resp, err = exchangeWebBootstrap(listener)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	cookie := findWebCapabilityCookie(resp.Cookies())
	if cookie == nil {
		t.Fatal("missing capability cookie")
	}

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
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "app.js") {
		t.Fatalf("release response = %d %q, want proxied descriptor", resp.StatusCode, string(body))
	}
}

func TestWebListenerRegistryReusesPortZeroHostname(t *testing.T) {
	reg := newWebListenerRegistry(logrus.NewEntry(logrus.New()))

	a, reused, err := reg.access(t.Context(), nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if reused {
		t.Fatal("first listener should not be reused")
	}

	b, reused, err := reg.access(t.Context(), nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatal(err)
	}
	if !reused {
		t.Fatal("second port-0 listener should be reused")
	}
	if a != b {
		t.Fatal("expected registry to return the existing listener")
	}
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
	reg := newWebListenerRegistry(logrus.NewEntry(logrus.New()))

	a, reused, err := reg.access(t.Context(), nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if reused {
		t.Fatal("first listener should not be reused")
	}

	parts := strings.Split(a.listenMultiaddr, "/tcp/")
	if len(parts) != 2 {
		t.Fatalf("unexpected listener multiaddr: %s", a.listenMultiaddr)
	}
	if _, reused, err := reg.access(t.Context(), nil, "/ip4/127.0.0.1/tcp/"+parts[1]); err == nil || reused {
		t.Fatalf("explicit port should allocate distinctly and fail while in use, reused=%v err=%v", reused, err)
	}
}

func TestWebListenerRegistryRetainsExplicitPort(t *testing.T) {
	reg := newWebListenerRegistry(logrus.NewEntry(logrus.New()))
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}

	listener, reused, err := reg.access(t.Context(), nil, "/ip4/127.0.0.1/tcp/"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if reused {
		t.Fatal("explicit listener should not be reused")
	}

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

func TestWebListenerRegistryAcquiresKeepalive(t *testing.T) {
	var held int
	resetKeepalive := SetWebListenerKeepaliveFunc(func(listenerID string) func() {
		if listenerID == "" {
			t.Fatal("listener id should be set before keepalive")
		}
		held++
		return func() {
			held--
		}
	})
	defer resetKeepalive()

	reg := newWebListenerRegistry(logrus.NewEntry(logrus.New()))
	listener, reused, err := reg.access(t.Context(), nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatal(err)
	}
	if reused {
		t.Fatal("first listener should not be reused")
	}
	if held != 1 {
		t.Fatalf("held keepalives = %d, want 1", held)
	}

	reusedListener, reused, err := reg.access(t.Context(), nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatal(err)
	}
	if !reused || reusedListener != listener {
		t.Fatal("second port-0 listener should reuse existing listener")
	}
	if held != 1 {
		t.Fatalf("reused listener keepalives = %d, want 1", held)
	}

	listener.Close()
	if held != 0 {
		t.Fatalf("held keepalives after close = %d, want 0", held)
	}
}

func TestAccessWebListenerBackgroundResponseIncludesLifecycleData(t *testing.T) {
	server := NewCoreRootServer(logrus.NewEntry(logrus.New()), nil)
	defer server.Close()

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
	if !strings.HasPrefix(resp.GetUrl(), "http://") {
		t.Fatalf("url = %q, want http:// URL", resp.GetUrl())
	}
	if resp.GetBootstrapSecret() == "" {
		t.Fatal("missing bootstrap secret")
	}
	if resp.GetReused() {
		t.Fatal("first listener should not be reused")
	}

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
	server := NewCoreRootServer(logrus.NewEntry(logrus.New()), nil)
	defer server.Close()

	resp, err := server.AccessWebListener(t.Context(), &s4wave_root.AccessWebListenerRequest{
		ListenMultiaddr: "/ip4/127.0.0.1/tcp/0",
		Background:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

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

	missing, err := server.StopWebListener(t.Context(), &s4wave_root.StopWebListenerRequest{
		ListenerId: "missing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !missing.GetNotFound() {
		t.Fatal("missing listener should report not found")
	}

	stopped, err := server.StopWebListener(t.Context(), &s4wave_root.StopWebListenerRequest{
		ListenerId: resp.GetListenerId(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if stopped.GetNotFound() {
		t.Fatal("existing listener should stop")
	}

	listeners = server.webListeners.list()
	if len(listeners) != 0 {
		t.Fatalf("listeners after stop = %d, want 0", len(listeners))
	}
}
