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
	"github.com/aperturerobotics/util/routine"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
	signaling_rpc "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	"github.com/sirupsen/logrus"
)

// signalingWebSocketPingInterval is the liveness interval for signaling WS.
const signalingWebSocketPingInterval = 15 * time.Second

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
	le     *logrus.Entry
	b      bus.Bus
	client *signaling_rpc_client.Client
	conn   *ws.Conn
	sigID  string
	pid    peer.ID

	mtx  sync.Mutex
	refs map[string]listenRef
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
	client *signaling_rpc_client.Client,
	conn *ws.Conn,
	sigID string,
	pid peer.ID,
) *wsSignalingCtrl {
	return &wsSignalingCtrl{
		le:     le,
		b:      b,
		client: client,
		conn:   conn,
		sigID:  sigID,
		pid:    pid,
		refs:   make(map[string]listenRef),
	}
}

// GetControllerInfo returns information about the controller.
func (c *wsSignalingCtrl) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"aperture/transport/ws-signaling",
		semver.MustParse("0.0.1"),
		"WebSocket signaling client",
	)
}

// Execute runs the signaling client lifecycle.
func (c *wsSignalingCtrl) Execute(ctx context.Context) error {
	pingRoutine := routine.NewRoutineContainer()
	pingRoutine.SetRoutine(func(rctx context.Context) error {
		return runWebSocketPing(rctx, c.conn, signalingWebSocketPingInterval)
	})
	pingRoutine.SetContext(ctx, false)
	defer pingRoutine.ClearContext()

	c.client.SetListenHandler(func(lctx context.Context, reset, added bool, pid string) {
		c.mtx.Lock()
		defer c.mtx.Unlock()

		if reset {
			for k, lr := range c.refs {
				lr.dirRef.Release()
				lr.peerRef.Release()
				delete(c.refs, k)
			}
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

		peerRef := c.client.AddPeerRef(pid)
		sess := signaling_rpc_client.NewSessionWithRef(peerRef)
		di := signaling.NewHandleSignalPeer(c.sigID, sess)
		_, ref, err := c.b.AddDirective(di, nil)
		if err != nil {
			peerRef.Release()
			c.le.WithError(err).Warn("failed to add HandleSignalPeer directive")
			return
		}
		c.refs[pid] = listenRef{peerRef: peerRef, dirRef: ref}
	})

	c.client.SetContext(ctx)

	<-ctx.Done()

	c.client.ClearContext()
	c.mtx.Lock()
	for k, lr := range c.refs {
		lr.dirRef.Release()
		lr.peerRef.Release()
		delete(c.refs, k)
	}
	c.mtx.Unlock()

	return ctx.Err()
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
	peerRef := r.c.client.AddPeerRef(remotePeerIDStr)

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
var _ controller.Controller = ((*wsSignalingCtrl)(nil))

// _ is a type assertion
var _ directive.Resolver = ((*wsSignalPeerResolver)(nil))
