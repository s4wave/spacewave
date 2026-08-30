package transport

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	ws "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
	signaling_rpc "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	"github.com/sirupsen/logrus"
)

// signalingWebSocketPingInterval is the liveness interval for signaling WS.
const (
	signalingWebSocketPingInterval  = 15 * time.Second
	signalingWebSocketRetryDelay    = 250 * time.Millisecond
	signalingWebSocketMaxRetryDelay = 5 * time.Second
)

type signalingDialFunc func(
	context.Context,
	*logrus.Entry,
	string,
	bifrost_crypto.PrivKey,
) (*signaling_rpc_client.Client, *ws.Conn, func(), error)

// dialSignalingClient dials a SignalingDO via WebSocket and returns a
// signaling client using direct SRPC over yamux (no bifrost transport).
func dialSignalingClient(
	ctx context.Context,
	le *logrus.Entry,
	url string,
	priv bifrost_crypto.PrivKey,
) (*signaling_rpc_client.Client, *ws.Conn, func(), error) {
	conn, _, err := ws.Dial(ctx, url, nil)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "dial signaling websocket")
	}

	mux, err := srpc.NewWebSocketConn(ctx, conn, false, nil)
	if err != nil {
		conn.CloseNow()
		return nil, nil, nil, errors.Wrap(err, "create yamux muxed conn")
	}

	client := srpc.NewClientWithMuxedConn(mux)
	sig := signaling_rpc.NewSRPCSignalingClient(client)

	sc, err := signaling_rpc_client.NewClient(le, sig, priv, nil)
	if err != nil {
		conn.CloseNow()
		return nil, nil, nil, errors.Wrap(err, "create signaling client")
	}

	cleanup := func() {
		sc.ClearContext()
		conn.CloseNow()
	}

	return sc, conn, cleanup, nil
}

// wsSignalingCtrl integrates a direct-WS signaling client with the bus.
type wsSignalingCtrl struct {
	le    *logrus.Entry
	b     bus.Bus
	url   string
	priv  bifrost_crypto.PrivKey
	sigID string
	pid   peer.ID

	client *signaling_rpc_client.Client
	conn   *ws.Conn

	mtx        sync.Mutex
	ready      chan struct{}
	refs       map[string]listenRef
	dial       signalingDialFunc
	retryDelay time.Duration
}

// listenRef holds references for an incoming signaling session.
type listenRef struct {
	peerRef *signaling_rpc_client.ClientPeerRef
	dirRef  directive.Reference
}

// newWSSignalingCtrl constructs a new WebSocket signaling bus controller.
func newWSSignalingCtrl(
	le *logrus.Entry,
	b bus.Bus,
	url string,
	priv bifrost_crypto.PrivKey,
	sigID string,
	pid peer.ID,
) *wsSignalingCtrl {
	return &wsSignalingCtrl{
		le:    le,
		b:     b,
		url:   url,
		priv:  priv,
		sigID: sigID,
		pid:   pid,
		ready: make(chan struct{}),
		refs:  make(map[string]listenRef),
	}
}

// GetControllerInfo returns information about the controller.
func (c *wsSignalingCtrl) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"aperture/transport/ws-signaling",
		controller.MustParseVersion("0.0.1"),
		"WebSocket signaling client",
	)
}

// Execute keeps signaling attached for the session transport lifetime. A
// failed dial or broken WebSocket starts a fresh generation after backoff.
func (c *wsSignalingCtrl) Execute(ctx context.Context) error {
	delay := c.retryDelay
	if delay <= 0 {
		delay = signalingWebSocketRetryDelay
	}
	for {
		err := c.executeGeneration(ctx)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		c.le.WithError(err).Warn("signaling websocket generation failed; reconnecting")
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
		if delay < signalingWebSocketMaxRetryDelay {
			delay = min(delay*2, signalingWebSocketMaxRetryDelay)
		}
	}
}

func (c *wsSignalingCtrl) executeGeneration(ctx context.Context) error {
	dial := c.dial
	if dial == nil {
		dial = dialSignalingClient
	}
	client, conn, cleanup, err := dial(ctx, c.le, c.url, c.priv)
	if err != nil {
		return err
	}
	defer cleanup()

	c.mtx.Lock()
	c.client = client
	c.conn = conn
	close(c.ready)
	c.mtx.Unlock()
	defer func() {
		c.mtx.Lock()
		c.releaseAllRefsLocked()
		c.client = nil
		c.conn = nil
		c.ready = make(chan struct{})
		c.mtx.Unlock()
	}()

	client.SetListenHandler(func(lctx context.Context, reset, added bool, pid string) {
		c.mtx.Lock()
		defer c.mtx.Unlock()

		if reset {
			c.releaseAllRefsLocked()
		}
		if pid == "" {
			return
		}
		if !added {
			if lr, ok := c.refs[pid]; ok {
				lr.dirRef.Release()
				lr.peerRef.Release()
				delete(c.refs, pid)
			}
			return
		}

		peerRef := client.AddPeerRef(pid)
		sess := signaling_rpc_client.NewSessionWithRef(peerRef)
		di := signaling.NewHandleSignalPeer(c.sigID, sess)
		_, ref, addErr := c.b.AddDirective(di, nil)
		if addErr != nil {
			peerRef.Release()
			c.le.WithError(addErr).Warn("failed to add HandleSignalPeer directive")
			return
		}
		c.refs[pid] = listenRef{peerRef: peerRef, dirRef: ref}
	})
	client.SetContext(ctx)

	pingErr := make(chan error, 1)
	go func() {
		pingErr <- runWebSocketPing(ctx, conn, signalingWebSocketPingInterval)
	}()
	select {
	case <-ctx.Done():
		client.ClearContext()
		return ctx.Err()
	case err := <-pingErr:
		client.ClearContext()
		if err == nil {
			err = errors.New("signaling websocket ping stopped")
		}
		return err
	}
}

// releaseAllRefsLocked releases every listen reference and empties the map.
// The caller must hold c.mtx.
func (c *wsSignalingCtrl) releaseAllRefsLocked() {
	for k, lr := range c.refs {
		lr.dirRef.Release()
		lr.peerRef.Release()
		delete(c.refs, k)
	}
}

// runWebSocketPing pings the websocket until the context is canceled.
func runWebSocketPing(ctx context.Context, conn *ws.Conn, interval time.Duration) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}

		if err := conn.Ping(ctx); err != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			return errors.Wrap(err, "ping websocket")
		}
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *wsSignalingCtrl) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case signaling.SignalPeer:
		return c.resolveSignalPeer(ctx, di, dir)
	}
	return nil, nil
}

// resolveSignalPeer checks directive filters and returns a resolver.
func (c *wsSignalingCtrl) resolveSignalPeer(_ context.Context, _ directive.Instance, dir signaling.SignalPeer) ([]directive.Resolver, error) {
	if sid := dir.SignalingID(); sid != "" && sid != c.sigID {
		return nil, nil
	}
	if lpid := dir.SignalLocalPeerID(); len(lpid) > 0 && lpid != c.pid {
		return nil, nil
	}
	if len(dir.SignalRemotePeerID()) == 0 {
		return nil, nil
	}
	return directive.Resolvers(&wsSignalPeerResolver{c: c, dir: dir}), nil
}

// Close releases any resources used by the controller.
func (c *wsSignalingCtrl) Close() error {
	return nil
}

// wsSignalPeerResolver resolves a SignalPeer directive via the WS client.
type wsSignalPeerResolver struct {
	c   *wsSignalingCtrl
	dir signaling.SignalPeer
}

// Resolve resolves the values, emitting them to the handler.
func (r *wsSignalPeerResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	remotePeerIDStr := r.dir.SignalRemotePeerID().String()
	var peerRef *signaling_rpc_client.ClientPeerRef
	for peerRef == nil {
		r.c.mtx.Lock()
		client := r.c.client
		ready := r.c.ready
		if client != nil {
			peerRef = client.AddPeerRef(remotePeerIDStr)
		}
		r.c.mtx.Unlock()
		if peerRef != nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ready:
		}
	}

	var val signaling.SignalPeerValue = signaling_rpc_client.NewSessionWithRef(peerRef)
	vid, accepted := handler.AddValue(val)
	if !accepted {
		peerRef.Release()
		return nil
	}

	handler.AddValueRemovedCallback(vid, peerRef.Release)

	<-ctx.Done()
	return ctx.Err()
}

// _ is a type assertion
var _ controller.Controller = (*wsSignalingCtrl)(nil)

// _ is a type assertion
var _ directive.Resolver = (*wsSignalPeerResolver)(nil)
