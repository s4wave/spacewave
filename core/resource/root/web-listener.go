//go:build !tinygo

package resource_root

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	ma "github.com/aperturerobotics/go-multiaddr"
	manet "github.com/aperturerobotics/go-multiaddr/net"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	web_runtime_http "github.com/s4wave/spacewave/bldr/web/runtime/http"
	bifrost_http "github.com/s4wave/spacewave/net/http"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"github.com/sirupsen/logrus"
)

const defaultWebListenMultiaddr = "/ip4/127.0.0.1/tcp/0"

const webCapabilityCookie = "spacewave_local_cap"

const webCapabilityTTL = 10 * time.Minute

const webListenerReadHeaderTimeout = 5 * time.Second

// webResourcePath serves a bound listener's Resource service over a websocket.
const webResourcePath = "/_spacewave/resource"

// WebAppPluginID is the plugin whose browser build a bound listener serves.
const WebAppPluginID = "spacewave-app"

// webAppFrontendPath is the root of the app plugin's frontend build. A bound
// listener's shell loads the app entry from its Vite manifest.
const webAppFrontendPath = "/b/pa/" + WebAppPluginID + "/v/b/fe/"

// AccessWebListener creates or reuses a localhost web listener.
func (s *CoreRootServer) AccessWebListener(
	ctx context.Context,
	req *s4wave_root.AccessWebListenerRequest,
) (*s4wave_root.AccessWebListenerResponse, error) {
	// Parse the listen address and the optional Space binding.
	spec, err := parseWebListenRequest(req)
	if err != nil {
		return nil, err
	}

	// Resolve daemon-owned listeners through the shared registry.
	if req.GetBackground() {
		listener, reused, err := s.webListeners.access(ctx, s.b, s.rootMux, spec)
		if err != nil {
			return nil, err
		}
		return listener.response(0, reused)
	}

	// Create a web listener whose lifetime follows the client resource.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	listener, err := newWebListener(ctx, s.le, s.b, s.rootMux, spec)
	if err != nil {
		return nil, err
	}
	id, err := resourceCtx.AddResource(srpc.NewMux(), listener.Close)
	if err != nil {
		listener.Close()
		return nil, err
	}
	return listener.response(id, false)
}

// WatchWebListeners streams daemon-owned localhost web listeners.
func (s *CoreRootServer) WatchWebListeners(
	_ *s4wave_root.WatchWebListenersRequest,
	strm s4wave_root.SRPCRootResourceService_WatchWebListenersStream,
) error {
	ctx := strm.Context()
	var prev *s4wave_root.WatchWebListenersResponse
	for {
		var waitCh <-chan struct{}
		var listeners []*s4wave_root.WebListenerInfo
		s.webListeners.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			listeners = s.webListeners.listLocked()
		})

		resp := &s4wave_root.WatchWebListenersResponse{Listeners: listeners}
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// StopWebListener stops a daemon-owned localhost web listener.
func (s *CoreRootServer) StopWebListener(
	ctx context.Context,
	req *s4wave_root.StopWebListenerRequest,
) (*s4wave_root.StopWebListenerResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stopped := s.webListeners.stop(req.GetListenerId())
	return &s4wave_root.StopWebListenerResponse{NotFound: !stopped}, nil
}

type webListenerRegistry struct {
	le *logrus.Entry

	bcast     broadcast.Broadcast
	listeners map[string]*webListener
}

func newWebListenerRegistry(le *logrus.Entry) *webListenerRegistry {
	return &webListenerRegistry{
		le:        le,
		listeners: make(map[string]*webListener),
	}
}

func (r *webListenerRegistry) close() {
	var listeners []*webListener
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for _, listener := range r.listeners {
			listeners = append(listeners, listener)
		}
		r.listeners = make(map[string]*webListener)
		broadcast()
	})
	for _, listener := range listeners {
		listener.Close()
	}
}

func (r *webListenerRegistry) access(
	ctx context.Context,
	b bus.Bus,
	rootMux srpc.Invoker,
	spec *webListenSpec,
) (*webListener, bool, error) {
	// Register explicit-port listeners without reuse.
	if spec.port != 0 {
		listener, err := newWebListener(ctx, r.le, b, rootMux, spec)
		if err != nil {
			return nil, false, err
		}
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.listeners["explicit:"+listener.id] = listener
			broadcast()
		})
		return listener, false, nil
	}

	// Reuse the existing listener for this ephemeral-port address and binding.
	key := spec.reuseKey()
	var existing *webListener
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		existing = r.listeners[key]
	})
	if existing != nil {
		return existing, true, nil
	}

	// Register a new listener unless another caller already supplied one.
	listener, err := newWebListener(ctx, r.le, b, rootMux, spec)
	if err != nil {
		return nil, false, err
	}
	var reused bool
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		existing = r.listeners[key]
		if existing != nil {
			reused = true
			return
		}
		r.listeners[key] = listener
		broadcast()
	})
	if reused {
		listener.Close()
		return existing, true, nil
	}
	return listener, false, nil
}

func (r *webListenerRegistry) list() []*s4wave_root.WebListenerInfo {
	var infos []*s4wave_root.WebListenerInfo
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		infos = r.listLocked()
	})
	return infos
}

func (r *webListenerRegistry) listLocked() []*s4wave_root.WebListenerInfo {
	// Build a sorted listener list for the registry snapshot.
	listeners := make([]*webListener, 0, len(r.listeners))
	for _, listener := range r.listeners {
		listeners = append(listeners, listener)
	}
	slices.SortFunc(listeners, func(a *webListener, b *webListener) int {
		return strings.Compare(a.id, b.id)
	})
	infos := make([]*s4wave_root.WebListenerInfo, 0, len(listeners))
	for _, listener := range listeners {
		infos = append(infos, listener.info(true))
	}
	return infos
}

func (r *webListenerRegistry) stop(listenerID string) bool {
	// Remove the requested listener from the registry before closing it.
	var listener *webListener
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		for key, existing := range r.listeners {
			if existing.id != listenerID {
				continue
			}
			listener = existing
			delete(r.listeners, key)
			broadcast()
			return
		}
	})
	if listener == nil {
		return false
	}
	listener.Close()
	return true
}

type webListener struct {
	id              string
	listenMultiaddr string
	url             string
	spec            *webListenSpec
	le              *logrus.Entry
	b               bus.Bus
	bldrHTTP        *web_runtime_http.Handler
	// resources serves the bound Resource service, or is nil when unbound.
	resources *srpc.HTTPServer
	// appAssets holds the app plugin's browser files mounted while bound.
	appAssets directive.Reference
	server    *http.Server
	listener  net.Listener
	// cancel ends the requests of the listener, including hijacked websockets
	// that http.Server.Close leaves open.
	cancel context.CancelFunc
	closed atomic.Bool

	bcast         broadcast.Broadcast
	bootstrapKeys map[string]time.Time
	capabilities  map[string]time.Time
}

// newWebListener starts a web listener for spec. rootMux is the Root resource
// mux a bound listener serves through its webBinding; an unbound listener and
// a listener without a bus ignore it.
func newWebListener(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	rootMux srpc.Invoker,
	spec *webListenSpec,
) (*webListener, error) {
	// Stop before binding when the request is already canceled.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Build the bound Resource service before taking the port.
	var resources *srpc.HTTPServer
	if spec.spaceID != "" {
		if rootMux == nil {
			return nil, errors.New("bound web listener requires the Root resource")
		}
		binding := &webBinding{
			le:         le.WithField("space-id", spec.spaceID),
			sessionIdx: spec.sessionIdx,
			spaceID:    spec.spaceID,
		}
		mux := srpc.NewMux()
		server := resource_server.NewFilteredResourceServer(rootMux, binding.filter)
		if err := server.Register(mux); err != nil {
			return nil, err
		}
		var err error
		resources, err = srpc.NewHTTPServer(mux, webResourcePath, nil)
		if err != nil {
			return nil, err
		}
	}

	// Bind the web listener to the requested TCP address.
	lis, err := net.Listen("tcp", net.JoinHostPort(spec.host, strconv.Itoa(int(spec.port))))
	if err != nil {
		return nil, errors.Wrap(err, "listen web")
	}

	// Resolve the bound listener address and generate its public identifier.
	resolved, err := manet.FromNetAddr(lis.Addr())
	if err != nil {
		_ = lis.Close()
		return nil, errors.Wrap(err, "resolve listener multiaddr")
	}
	idSecret, err := newWebSecret()
	if err != nil {
		_ = lis.Close()
		return nil, err
	}

	// Create the native runtime handler and HTTP server for the listener.
	var bldrHTTP *web_runtime_http.Handler
	if b != nil {
		bldrHTTP = web_runtime_http.NewHandler(le, b)
	}
	host, port, err := tcpListenHostPort(lis.Addr())
	if err != nil {
		_ = lis.Close()
		return nil, err
	}
	serveCtx, cancel := context.WithCancel(context.Background())
	listener := &webListener{
		id:              "web-" + strconv.FormatUint(uint64(port), 10) + "-" + idSecret[:8],
		listenMultiaddr: resolved.String(),
		url:             "http://" + net.JoinHostPort(host, strconv.Itoa(int(port))),
		spec:            spec,
		le:              le,
		b:               b,
		bldrHTTP:        bldrHTTP,
		resources:       resources,
		listener:        lis,
		cancel:          cancel,
		bootstrapKeys:   make(map[string]time.Time),
		capabilities:    make(map[string]time.Time),
	}
	if spec.spaceID != "" && b != nil {
		_, listener.appAssets, err = b.AddDirective(bldr_plugin.NewLoadPluginAssets(WebAppPluginID), nil)
		if err != nil {
			cancel()
			_ = lis.Close()
			return nil, err
		}
	}
	listener.server = &http.Server{
		Handler:           listener,
		ReadHeaderTimeout: webListenerReadHeaderTimeout,
		BaseContext:       func(net.Listener) context.Context { return serveCtx },
	}

	// Serve web requests until the listener closes.
	go func() {
		err := listener.server.Serve(lis)
		if err != nil && err != http.ErrServerClosed {
			le.WithError(err).Warn("web listener stopped")
		}
	}()
	return listener, nil
}

func (l *webListener) response(resourceID uint32, reused bool) (*s4wave_root.AccessWebListenerResponse, error) {
	secret, err := l.issueBootstrapSecret()
	if err != nil {
		return nil, err
	}
	return &s4wave_root.AccessWebListenerResponse{
		ResourceId:      resourceID,
		ListenerId:      l.id,
		ListenMultiaddr: l.listenMultiaddr,
		Url:             l.url,
		BootstrapSecret: secret,
		Reused:          reused,
	}, nil
}

func (l *webListener) info(background bool) *s4wave_root.WebListenerInfo {
	return &s4wave_root.WebListenerInfo{
		ListenerId:      l.id,
		ListenMultiaddr: l.listenMultiaddr,
		Url:             l.url,
		Background:      background,
		SpaceId:         l.spec.spaceID,
		SessionIdx:      l.spec.sessionIdx,
	}
}

// Close closes the listener.
func (l *webListener) Close() {
	// Stop serving once, then release the bound app files.
	if !l.closed.CompareAndSwap(false, true) {
		return
	}
	l.cancel()
	_ = l.server.Close()
	_ = l.listener.Close()
	if l.appAssets != nil {
		l.appAssets.Release()
	}
}

// ServeHTTP serves the boot shell and bootstrap exchange to anyone, and every
// other route only to a request carrying a live capability cookie.
func (l *webListener) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Route web requests through bootstrap, authorization, and runtime handlers.
	if req.URL.Path == "/_spacewave/health" {
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = rw.Write([]byte("ok\n"))
		return
	}
	if req.URL.Path == "/_spacewave/bootstrap" {
		l.exchangeBootstrap(rw, req)
		return
	}
	if isWebListenerBootShellPath(req.URL.Path) {
		l.serveBootShell(rw, req)
		return
	}
	if !l.isAuthorized(req) {
		http.Error(rw, "spacewave: missing localhost capability", http.StatusUnauthorized)
		return
	}
	if req.URL.Path == webResourcePath {
		if l.resources == nil {
			http.NotFound(rw, req)
			return
		}
		l.resources.ServeHTTP(rw, req)
		return
	}
	if strings.HasPrefix(req.URL.Path, "/b/") || strings.HasPrefix(req.URL.Path, "/p/") {
		l.serveNativeRuntimeHTTP(rw, req)
		return
	}
	l.serveReleaseWebHTTP(rw, req)
}

func (l *webListener) exchangeBootstrap(rw http.ResponseWriter, req *http.Request) {
	// Require a POST request for the bootstrap exchange.
	if req.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate the capability token for the supplied bootstrap secret.
	secret := req.Header.Get("X-Spacewave-Bootstrap")
	token, err := newWebSecret()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Exchange the unexpired bootstrap secret for a retained capability.
	var ok bool
	now := time.Now()
	expires := now.Add(webCapabilityTTL)
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Consume the bootstrap secret and register its capability if valid.
		l.pruneExpiredKeys(now)
		bootstrapExpires, found := l.bootstrapKeys[secret]
		ok = found && now.Before(bootstrapExpires)
		if found {
			delete(l.bootstrapKeys, secret)
		}
		if !ok {
			return
		}
		l.capabilities[token] = expires
		broadcast()
	})
	if !ok {
		http.Error(rw, "invalid bootstrap secret", http.StatusUnauthorized)
		return
	}

	// Return the capability cookie and token to the bootstrap client. The
	// listener serves plain HTTP on a local host only, so the cookie is not
	// Secure: WebKit drops Secure cookies on HTTP loopback even though it
	// treats loopback as a secure context.
	http.SetCookie(rw, &http.Cookie{ //nolint:gosec // plain HTTP on a local host; see above.
		Name:     webCapabilityCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(webCapabilityTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = rw.Write([]byte(token + "\n"))
}

func (l *webListener) issueBootstrapSecret() (string, error) {
	// Generate and retain an expiring bootstrap secret for this listener.
	secret, err := newWebSecret()
	if err != nil {
		return "", err
	}
	now := time.Now()
	expires := now.Add(webCapabilityTTL)
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.pruneExpiredKeys(now)
		l.bootstrapKeys[secret] = expires
		broadcast()
	})
	return secret, nil
}

func (l *webListener) pruneExpiredKeys(now time.Time) {
	for key, expires := range l.bootstrapKeys {
		if !now.Before(expires) {
			delete(l.bootstrapKeys, key)
		}
	}
	for key, expires := range l.capabilities {
		if !now.Before(expires) {
			delete(l.capabilities, key)
		}
	}
}

func (l *webListener) isAuthorized(req *http.Request) bool {
	// Validate the request capability against the listener expiration record.
	cookie, err := req.Cookie(webCapabilityCookie)
	if err != nil || cookie.Value == "" {
		return false
	}
	var ok bool
	now := time.Now()
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		expires, found := l.capabilities[cookie.Value]
		ok = found && now.Before(expires)
		if found && !ok {
			delete(l.capabilities, cookie.Value)
		}
	})
	return ok
}

func isWebListenerBootShellPath(path string) bool {
	return path == "/" || path == "/index.html" || path == "/display" || strings.HasPrefix(path, "/display/")
}

func (l *webListener) serveBootShell(rw http.ResponseWriter, req *http.Request) {
	// Fetch the released web app HTML for its boot metadata.
	html, err := l.fetchReleaseRootHTML(req.Context())
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}

	// Extract the release import map and stylesheet links.
	metadata, err := webListenerReleaseBootMetadataFromHTML(html)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}

	// Render and serve the local bootstrap shell.
	shell, err := renderWebListenerBootShell(metadata, l.spec)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Cache-Control", "no-store")
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = rw.Write(shell)
}

func (l *webListener) fetchReleaseRootHTML(ctx context.Context) (string, error) {
	// Build an authorized request for the released web app root.
	remoteURL, err := url.JoinPath(webAppEndpoint(), "/")
	if err != nil {
		return "", err
	}
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return "", err
	}
	if auth := webAppAuthorization(); auth != "" {
		upstreamReq.Header.Set("Authorization", auth)
	}

	// Fetch the release root and read its successful response body.
	resp, err := http.DefaultClient.Do(upstreamReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.Errorf("spacewave: release root returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type webListenerReleaseBootMetadata struct {
	importMapScript string
	stylesheetLinks string
}

var (
	releaseImportMapScriptRE = regexp.MustCompile(`(?is)<script\s+type=["']importmap["'][^>]*>.*?</script>`)
	releaseStylesheetLinkRE  = regexp.MustCompile(`(?is)<link\s+[^>]*rel=["']stylesheet["'][^>]*>`)
)

func webListenerReleaseBootMetadataFromHTML(html string) (*webListenerReleaseBootMetadata, error) {
	importMapScript := releaseImportMapScriptRE.FindString(html)
	if importMapScript == "" {
		return nil, errors.New("spacewave: release root missing import map")
	}
	return &webListenerReleaseBootMetadata{
		importMapScript: importMapScript,
		stylesheetLinks: strings.Join(releaseStylesheetLinkRE.FindAllString(html, -1), "\n"),
	}, nil
}

// renderWebListenerBootShell renders the shell that exchanges the bootstrap
// secret for the capability cookie and then starts the app. An unbound
// listener starts the released WASM runtime. A bound listener skips it: the
// shell loads the native app entry and renders it over the listener's
// Resource websocket, opening the bound Space when no route is given.
func renderWebListenerBootShell(metadata *webListenerReleaseBootMetadata, spec *webListenSpec) ([]byte, error) {
	start := "await import('/boot.mjs');"
	if spec.spaceID != "" {
		route := "/u/" + strconv.FormatUint(uint64(spec.sessionIdx), 10) + "/so/" + url.PathEscape(spec.spaceID)
		start = `if (!location.hash) history.replaceState(null, '', '#' + ` + quoteWebListenerScriptString(route) + `);
const appBase = ` + quoteWebListenerScriptString(webAppFrontendPath) + `;
const manifestResp = await fetch(appBase + '.vite/manifest.json');
if (!manifestResp.ok) {
  setBootstrapFailure('Spacewave app manifest failed: ' + manifestResp.status);
  throw new Error('Spacewave app manifest failed');
}
const manifest = await manifestResp.json();
const seen = new Set();
const addStyles = (key) => {
  const entry = manifest[key];
  if (!entry || seen.has(key)) return;
  seen.add(key);
  for (const href of entry.css ?? []) {
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.href = appBase + href;
    document.head.append(link);
  }
  for (const dep of entry.imports ?? []) addStyles(dep);
};
addStyles('app/App.tsx');
const app = await import(appBase + manifest['app/App.tsx'].file);
app.renderBoundApp(document.getElementById('bldr-root'), ` + quoteWebListenerScriptString(webResourcePath) + `);`
	}
	stylesheetLinks := quoteWebListenerScriptString(metadata.stylesheetLinks)
	return []byte(`<!doctype html>
<meta charset="utf-8">
<title>Spacewave</title>
` + metadata.importMapScript + `
<div id="bldr-root" role="main"></div>
<script type="module">
function setBootstrapFailure(message) {
  const status = document.querySelector('[data-sw-boot-status]');
  if (status) status.textContent = message;
  const root = document.getElementById('bldr-root') || document.getElementById('root') || document.body;
  if (root) root.textContent = message;
}
const params = new URLSearchParams(location.hash.slice(1));
const otp = params.get('otp') || params.get('spacewave_bootstrap') || '';
if (otp) {
  const boot = await fetch('/_spacewave/bootstrap', {
    method: 'POST',
    headers: { 'X-Spacewave-Bootstrap': otp },
  });
  if (!boot.ok) {
    setBootstrapFailure('Spacewave bootstrap failed: ' + await boot.text());
    throw new Error('Spacewave bootstrap failed');
  }
  try { localStorage.setItem('spacewave-has-session', '1'); } catch (_) {}
  history.replaceState(null, '', location.pathname + location.search);
}
const stylesheetLinks = ` + stylesheetLinks + `;
if (stylesheetLinks) document.head.insertAdjacentHTML('beforeend', stylesheetLinks);
` + start + `
</script>`), nil
}

func quoteWebListenerScriptString(value string) string {
	// Escape HTML delimiters in the JavaScript string literal.
	quoted := strconv.Quote(value)
	quoted = strings.ReplaceAll(quoted, "<", `\u003c`)
	quoted = strings.ReplaceAll(quoted, ">", `\u003e`)
	quoted = strings.ReplaceAll(quoted, "&", `\u0026`)
	return quoted
}

func (l *webListener) serveNativeRuntimeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Serve the Bldr frontend, package, and plugin file routes from the bus.
	if l.b == nil {
		http.Error(rw, "spacewave: native runtime unavailable", http.StatusNotFound)
		return
	}
	if l.bldrHTTP.ServeBldrHTTP(rw, req) {
		return
	}

	// Resolve the native HTTP handler and retain it through the response.
	handler, _, handlerRef, err := bifrost_http.ExLookupFirstHTTPHandler(
		req.Context(),
		l.b,
		req.Method,
		req.URL,
		"",
		true,
		nil,
	)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	if handlerRef == nil {
		http.Error(rw, "spacewave: native handler not found", http.StatusNotFound)
		return
	}
	defer handlerRef.Release()
	handler.ServeHTTP(rw, req)
}

func (l *webListener) serveReleaseWebHTTP(rw http.ResponseWriter, req *http.Request) {
	// Require a read request before proxying release web assets.
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// The upstream host is operator-configured (webAppEndpoint); JoinPath cleans dot
	// segments, so the forwarded request path cannot escape that host.
	// #nosec G704 -- intentional reverse proxy to the configured web app endpoint.
	remoteURL, err := url.JoinPath(webAppEndpoint(), releaseWebRemotePath(req.URL.Path))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build the upstream request with range and authorization headers.
	// #nosec G704 -- intentional reverse proxy to the configured web app endpoint.
	upstreamReq, err := http.NewRequestWithContext(req.Context(), req.Method, remoteURL, nil)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	if rng := req.Header.Get("Range"); rng != "" {
		upstreamReq.Header.Set("Range", rng)
	}
	if auth := webAppAuthorization(); auth != "" {
		upstreamReq.Header.Set("Authorization", auth)
	}

	// Fetch the upstream response and retain its body through forwarding.
	// #nosec G704 -- intentional reverse proxy to the configured web app endpoint.
	resp, err := http.DefaultClient.Do(upstreamReq)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward the upstream headers and body to the web client.
	copyHTTPHeaders(rw.Header(), resp.Header)
	rw.WriteHeader(resp.StatusCode)
	if req.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(rw, resp.Body); err != nil {
		l.le.WithError(err).Debug("copy release web response")
	}
}

func releaseWebRemotePath(localPath string) string {
	return "/" + strings.TrimLeft(localPath, "/")
}

func copyHTTPHeaders(dst, src http.Header) {
	for k, vals := range src {
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

// webListenSpec is the address and binding of a web listener.
type webListenSpec struct {
	host string
	port uint32
	// spaceID and sessionIdx bind the listener to one Space, or are empty.
	spaceID    string
	sessionIdx uint32
}

// reuseKey identifies the ephemeral-port listeners a request may reuse: the
// same host and the same binding.
func (s *webListenSpec) reuseKey() string {
	key := strings.ToLower(strings.Trim(s.host, "[]"))
	if s.spaceID != "" {
		key += "/" + strconv.FormatUint(uint64(s.sessionIdx), 10) + "/" + s.spaceID
	}
	return key
}

// parseWebListenRequest parses the address and binding of a listener request.
func parseWebListenRequest(req *s4wave_root.AccessWebListenerRequest) (*webListenSpec, error) {
	// Parse the address, then require a complete binding or none.
	spec, err := parseWebListenSpec(req.GetListenMultiaddr())
	if err != nil {
		return nil, err
	}
	spec.spaceID, spec.sessionIdx = req.GetSpaceId(), req.GetSessionIdx()
	if (spec.spaceID == "") != (spec.sessionIdx == 0) {
		return nil, errors.New("web listener binding needs both a space id and a session index")
	}
	return spec, nil
}

func parseWebListenSpec(listenMultiaddr string) (*webListenSpec, error) {
	// Parse the requested web listen address or its default.
	raw := listenMultiaddr
	if raw == "" {
		raw = defaultWebListenMultiaddr
	}
	maddr, err := ma.NewMultiaddr(raw)
	if err != nil {
		return nil, errors.Wrap(err, "parse listen multiaddr")
	}

	// Extract and validate the localhost TCP host and port.
	var host string
	var port string
	for _, comp := range maddr {
		switch comp.Protocol().Code {
		case ma.P_IP4, ma.P_IP6, ma.P_DNS, ma.P_DNS4, ma.P_DNS6:
			if host != "" {
				return nil, errors.New("listen multiaddr has multiple host components")
			}
			host = comp.Value()
		case ma.P_TCP:
			if port != "" {
				return nil, errors.New("listen multiaddr has multiple tcp components")
			}
			port = comp.Value()
		}
	}
	if host == "" || port == "" {
		return nil, errors.New("listen multiaddr must contain host and tcp port")
	}
	if !isLocalWebHost(host) {
		return nil, errors.New("web listener host must be localhost or loopback")
	}

	// Decode the TCP port for the web listener specification.
	portU64, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, errors.Wrap(err, "parse tcp port")
	}
	return &webListenSpec{
		host: host,
		port: uint32(portU64),
	}, nil
}

func isLocalWebHost(host string) bool {
	normalized := strings.ToLower(strings.Trim(host, "[]"))
	if normalized == "localhost" {
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && ip.IsLoopback()
}

func tcpListenHostPort(addr net.Addr) (string, uint32, error) {
	// Resolve the bound TCP listener host with a loopback fallback.
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return "", 0, errors.New("web listener is not tcp")
	}
	host := tcpAddr.IP.String()
	if host == "<nil>" || host == "" {
		host = "127.0.0.1"
	}
	return host, uint32(tcpAddr.Port), nil //nolint:gosec // net.TCPAddr.Port is validated as a TCP port by the listener.
}

func newWebSecret() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", errors.Wrap(err, "generate web secret")
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

// _ is a type assertion
var _ http.Handler = (*webListener)(nil)
