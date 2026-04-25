package e2e_wasm_session

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/s4wave/spacewave/net/signaling"
)

// ControllerID is the controller identifier.
const ControllerID = "e2e/wasm/session"

// Version is the component version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
const controllerDescrip = "e2e wasm session harness controller"

// Controller is the session harness controller running inside the WASM
// plugin. It exposes RPC services for test orchestration: peer info,
// link establishment, and signaling relay.
type Controller struct {
	*bus.BusController[*Config]
	mux    srpc.Mux
	relays *relayRegistry
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			c := &Controller{
				BusController: base,
				relays:        newRelayRegistry(),
			}
			c.mux = srpc.NewMux()
			if err := SRPCRegisterPeerInfoResourceService(c.mux, c); err != nil {
				return nil, err
			}
			if err := SRPCRegisterSignalRelayService(c.mux, c); err != nil {
				return nil, err
			}
			if err := SRPCRegisterEstablishLinkResourceService(c.mux, c); err != nil {
				return nil, err
			}
			return c, nil
		},
	)
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		return c.resolveLookupRpcService(d)
	case signaling.SignalPeer:
		return c.resolveSignalPeer(d)
	}
	return nil, nil
}

// resolveLookupRpcService resolves LookupRpcService directives for the
// session harness service IDs.
func (c *Controller) resolveLookupRpcService(
	d bifrost_rpc.LookupRpcService,
) ([]directive.Resolver, error) {
	switch d.LookupRpcServiceID() {
	case SRPCPeerInfoResourceServiceServiceID,
		SRPCSignalRelayServiceServiceID,
		SRPCEstablishLinkResourceServiceServiceID:
		return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c), nil)
	}
	return nil, nil
}

// resolveSignalPeer resolves SignalPeer directives matching signalingID
// "webrtc". Returns a resolver that waits for the relay session for the
// target remote peer.
func (c *Controller) resolveSignalPeer(
	d signaling.SignalPeer,
) ([]directive.Resolver, error) {
	if sigID := d.SignalingID(); sigID != "" && sigID != "webrtc" {
		return nil, nil
	}
	return directive.R(&signalPeerResolver{c: c, dir: d}, nil)
}

// GetPeerInfo returns the local bifrost peer ID.
func (c *Controller) GetPeerInfo(
	ctx context.Context,
	req *GetPeerInfoRequest,
) (*GetPeerInfoResponse, error) {
	b := c.GetBus()
	p, _, ref, err := peer.GetPeerWithID(ctx, b, peer.ID(""), false, nil)
	if err != nil {
		return nil, err
	}
	defer ref.Release()
	return &GetPeerInfoResponse{PeerId: p.GetPeerID().String()}, nil
}

// SignalRelay handles a bidirectional signaling relay stream. The first
// message must be a SignalRelayInit identifying the remote peer. Subsequent
// messages carry opaque signaling bytes forwarded between the Go test
// process and the browser WASM SignalPeer resolver.
func (c *Controller) SignalRelay(strm SRPCSignalRelayService_SignalRelayStream) error {
	ctx := strm.Context()

	// Read the init message.
	msg, err := strm.Recv()
	if err != nil {
		return err
	}
	init := msg.GetInit()
	if init == nil {
		return errors.New("first message must be SignalRelayInit")
	}
	remotePeer, err := peer.IDB58Decode(init.GetRemotePeerId())
	if err != nil {
		return errors.Wrap(err, "decode remote peer id")
	}

	rs := newRelaySession(ctx, remotePeer)
	defer rs.Close()
	if err := c.relays.register(rs); err != nil {
		return err
	}
	defer c.relays.unregister(remotePeer)

	// Register the incoming signaling handler for this relay session so the
	// WebRTC transport can consume remote offers / answers / ICE over the same
	// bidirectional stream.
	b := c.GetBus()
	p, _, ref, err := peer.GetPeerWithID(ctx, b, peer.ID(""), false, nil)
	if err != nil {
		return err
	}
	defer ref.Release()

	sess := &signalPeerSession{
		localPeerID:  p.GetPeerID(),
		remotePeerID: remotePeer,
		relay:        rs,
	}
	di, dirRef, err := b.AddDirective(
		signaling.NewHandleSignalPeer("webrtc", sess),
		nil,
	)
	if err != nil {
		return err
	}
	defer di.Close()
	defer dirRef.Release()

	// Bidirectional forwarding: read from stream into incoming channel,
	// read from outgoing channel into stream.
	errCh := make(chan error, 2)

	// Stream -> incoming (messages from Go test to SignalPeer resolver)
	go func() {
		for {
			msg, err := strm.Recv()
			if err != nil {
				errCh <- err
				return
			}
			data := msg.GetData()
			if data == nil {
				continue
			}
			data = slices.Clone(data)
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case rs.incoming <- data:
			}
		}
	}()

	// Outgoing -> stream (messages from SignalPeer resolver to Go test)
	go func() {
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case data := <-rs.outgoing:
				err := strm.Send(&SignalRelayMessage{
					Body: &SignalRelayMessage_Data{Data: data},
				})
				if err != nil {
					errCh <- err
					return
				}
			}
		}
	}()

	// Wait for either goroutine to finish.
	return <-errCh
}

// WatchState handles the EstablishLink streaming RPC. It emits an
// EstablishLinkWithPeer directive on the bus and streams state updates
// as the link progresses from PENDING to CONNECTED (or FAILED).
func (c *Controller) WatchState(
	req *WatchStateRequest,
	strm SRPCEstablishLinkResourceService_WatchStateStream,
) error {
	ctx := strm.Context()

	targetPeer, err := peer.IDB58Decode(req.GetTargetPeerId())
	if err != nil {
		return errors.Wrap(err, "decode target peer id")
	}

	// Send initial PENDING state.
	if err := strm.Send(&WatchStateResponse{
		State: EstablishLinkState_EstablishLinkState_PENDING,
	}); err != nil {
		return err
	}

	b := c.GetBus()

	linkCh := make(chan link.MountedLink, 1)
	failCh := make(chan struct{}, 1)

	notifyFail := func() {
		select {
		case failCh <- struct{}{}:
		default:
		}
	}

	handler := directive.NewTypedCallbackHandler(
		func(v directive.TypedAttachedValue[link.MountedLink]) {
			select {
			case linkCh <- v.GetValue():
			default:
			}
		},
		func(v directive.TypedAttachedValue[link.MountedLink]) {
			notifyFail()
		},
		nil, nil,
	)

	di, diRef, err := b.AddDirective(
		link.NewEstablishLinkWithPeer(peer.ID(""), targetPeer),
		handler,
	)
	if err != nil {
		return err
	}
	defer diRef.Release()

	// Detect when all resolvers are idle with errors and no link was added.
	removeIdleCb := di.AddIdleCallback(func(isIdle bool, errs []error) {
		if isIdle && len(errs) > 0 {
			notifyFail()
		}
	})
	defer removeIdleCb()

	// Stream state transitions until context is cancelled.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-linkCh:
			if err := strm.Send(&WatchStateResponse{
				State: EstablishLinkState_EstablishLinkState_CONNECTED,
			}); err != nil {
				return err
			}
			return nil
		case <-failCh:
			return strm.Send(&WatchStateResponse{
				State: EstablishLinkState_EstablishLinkState_FAILED,
			})
		}
	}
}

// InvokeMethod invokes the method matching the service and method IDs.
func (c *Controller) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	return c.mux.InvokeMethod(serviceID, methodID, strm)
}

// _ is a type assertion
var (
	_ controller.Controller                  = (*Controller)(nil)
	_ srpc.Invoker                           = (*Controller)(nil)
	_ SRPCPeerInfoResourceServiceServer      = (*Controller)(nil)
	_ SRPCSignalRelayServiceServer           = (*Controller)(nil)
	_ SRPCEstablishLinkResourceServiceServer = (*Controller)(nil)
)
