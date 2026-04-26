package resource_root

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	ma "github.com/aperturerobotics/go-multiaddr"
	manet "github.com/aperturerobotics/go-multiaddr/net"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	web_pkg_http "github.com/s4wave/spacewave/bldr/web/pkg/http"
	bifrost_http "github.com/s4wave/spacewave/net/http"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	"github.com/sirupsen/logrus"
)

const defaultWebListenMultiaddr = "/ip4/127.0.0.1/tcp/0"

const webCapabilityCookie = "spacewave_local_cap"

const webCapabilityTTL = 10 * time.Minute

// AccessWebListener creates or reuses a localhost web listener.
func (s *CoreRootServer) AccessWebListener(
	ctx context.Context,
	req *s4wave_root.AccessWebListenerRequest,
) (*s4wave_root.AccessWebListenerResponse, error) {
	if req.GetBackground() {
		listener, reused, err := s.webListeners.access(ctx, s.b, req.GetListenMultiaddr())
		if err != nil {
			return nil, err
		}
		return listener.response(0, reused)
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	listener, err := newWebListener(ctx, s.le, s.b, req.GetListenMultiaddr())
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
	listenMultiaddr string,
) (*webListener, bool, error) {
	spec, err := parseWebListenSpec(listenMultiaddr)
	if err != nil {
		return nil, false, err
	}
	if spec.port != 0 {
		listener, err := newWebListenerWithSpec(ctx, r.le, b, spec)
		if err != nil {
			return nil, false, err
		}
		listener.holdDaemonKeepalive()
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.listeners["explicit:"+listener.id] = listener
			broadcast()
		})
		return listener, false, nil
	}

	key := spec.reuseKey()
	var existing *webListener
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		existing = r.listeners[key]
	})
	if existing != nil {
		return existing, true, nil
	}

	listener, err := newWebListenerWithSpec(ctx, r.le, b, spec)
	if err != nil {
		return nil, false, err
	}
	listener.holdDaemonKeepalive()
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
	id               string
	listenMultiaddr  string
	url              string
	le               *logrus.Entry
	b                bus.Bus
	pkgServer        *web_pkg_http.Server
	server           *http.Server
	listener         net.Listener
	closeOnce        sync.Once
	releaseKeepalive func()

	bcast         broadcast.Broadcast
	bootstrapKeys map[string]time.Time
	capabilities  map[string]time.Time
}

func newWebListener(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	listenMultiaddr string,
) (*webListener, error) {
	spec, err := parseWebListenSpec(listenMultiaddr)
	if err != nil {
		return nil, err
	}
	return newWebListenerWithSpec(ctx, le, b, spec)
}

func newWebListenerWithSpec(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	spec *webListenSpec,
) (*webListener, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lis, err := net.Listen("tcp", net.JoinHostPort(spec.host, strconv.Itoa(int(spec.port))))
	if err != nil {
		return nil, errors.Wrap(err, "listen web")
	}
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
	var pkgServer *web_pkg_http.Server
	if b != nil {
		pkgServer = web_pkg_http.NewServer(le, b, false)
	}
	host, port, err := tcpListenHostPort(lis.Addr())
	if err != nil {
		_ = lis.Close()
		return nil, err
	}
	listener := &webListener{
		id:              "web-" + strconv.FormatUint(uint64(port), 10) + "-" + idSecret[:8],
		listenMultiaddr: resolved.String(),
		url:             "http://" + net.JoinHostPort(host, strconv.Itoa(int(port))),
		le:              le,
		b:               b,
		pkgServer:       pkgServer,
		listener:        lis,
		bootstrapKeys:   make(map[string]time.Time),
		capabilities:    make(map[string]time.Time),
	}
	listener.server = &http.Server{Handler: listener}
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
	}
}

// Close closes the listener.
func (l *webListener) Close() {
	l.closeOnce.Do(func() {
		_ = l.server.Close()
		_ = l.listener.Close()
		if l.releaseKeepalive != nil {
			l.releaseKeepalive()
		}
	})
}

func (l *webListener) holdDaemonKeepalive() {
	if l.releaseKeepalive != nil {
		return
	}
	l.releaseKeepalive = acquireWebListenerKeepalive(l.id)
}

func (l *webListener) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/_spacewave/health" {
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = rw.Write([]byte("ok\n"))
		return
	}
	if req.URL.Path == "/_spacewave/bootstrap" {
		l.exchangeBootstrap(rw, req)
		return
	}
	if req.URL.Path == "/" || req.URL.Path == "/index.html" {
		l.serveBootShell(rw)
		return
	}
	if !l.isAuthorized(req) {
		http.Error(rw, "spacewave: missing localhost capability", http.StatusUnauthorized)
		return
	}
	if strings.HasPrefix(req.URL.Path, "/b/") || strings.HasPrefix(req.URL.Path, "/p/") {
		l.serveNativeRuntimeHTTP(rw, req)
		return
	}
	l.serveReleaseWebHTTP(rw, req)
}

func (l *webListener) exchangeBootstrap(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := req.Header.Get("X-Spacewave-Bootstrap")
	token, err := newWebSecret()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	var ok bool
	now := time.Now()
	expires := now.Add(webCapabilityTTL)
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
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
	http.SetCookie(rw, &http.Cookie{
		Name:     webCapabilityCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(webCapabilityTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = rw.Write([]byte(token + "\n"))
}

func (l *webListener) issueBootstrapSecret() (string, error) {
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

func (l *webListener) serveBootShell(rw http.ResponseWriter) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = rw.Write([]byte(`<!doctype html>
<meta charset="utf-8">
<title>Spacewave</title>
<div id="root">Starting Spacewave...</div>
<script type="module">
const params = new URLSearchParams(location.hash.slice(1));
const otp = params.get('otp') || params.get('spacewave_bootstrap') || '';
if (otp) {
  const boot = await fetch('/_spacewave/bootstrap', {
    method: 'POST',
    headers: { 'X-Spacewave-Bootstrap': otp },
  });
  if (!boot.ok) {
    document.getElementById('root').textContent = 'Spacewave bootstrap failed: ' + await boot.text();
    throw new Error('Spacewave bootstrap failed');
  }
  history.replaceState(null, '', location.pathname + location.search);
}
await import('/boot.mjs');
</script>`))
}

func (l *webListener) serveNativeRuntimeHTTP(rw http.ResponseWriter, req *http.Request) {
	if l.b == nil {
		http.Error(rw, "spacewave: native runtime unavailable", http.StatusNotFound)
		return
	}
	pkgPrefix := bldr_plugin.PluginWebPkgHttpPrefix
	if strings.HasPrefix(req.URL.Path, pkgPrefix) && len(req.URL.Path) > len(pkgPrefix) {
		if l.pkgServer == nil {
			http.Error(rw, "spacewave: native package runtime unavailable", http.StatusNotFound)
			return
		}
		l.pkgServer.ServeWebModuleHTTP(req.URL.Path[len(pkgPrefix):], rw, req)
		return
	}
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
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	remoteURL, err := url.JoinPath(webAppEndpoint(), releaseWebRemotePath(req.URL.Path))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
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
	resp, err := http.DefaultClient.Do(upstreamReq)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
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

type webListenSpec struct {
	host string
	port uint32
}

func (s *webListenSpec) reuseKey() string {
	return strings.ToLower(strings.Trim(s.host, "[]"))
}

func parseWebListenSpec(listenMultiaddr string) (*webListenSpec, error) {
	raw := listenMultiaddr
	if raw == "" {
		raw = defaultWebListenMultiaddr
	}
	maddr, err := ma.NewMultiaddr(raw)
	if err != nil {
		return nil, errors.Wrap(err, "parse listen multiaddr")
	}
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
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return "", 0, errors.New("web listener is not tcp")
	}
	host := tcpAddr.IP.String()
	if host == "<nil>" || host == "" {
		host = "127.0.0.1"
	}
	return host, uint32(tcpAddr.Port), nil
}

func newWebSecret() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", errors.Wrap(err, "generate web secret")
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

// _ is a type assertion
var _ http.Handler = ((*webListener)(nil))
