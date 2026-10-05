package resource_session

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/transport"
	bifrost_api "github.com/s4wave/spacewave/net/daemon/api"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	stream_api "github.com/s4wave/spacewave/net/stream/api"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// AccessPeerTransport exposes the account's authenticated stream transport for
// this Session. The account owns the transport; the returned resource owns the
// optional UDP transport and remote resource service, which stop when the
// caller releases it.
func (r *SessionResource) AccessPeerTransport(
	ctx context.Context,
	req *s4wave_session.AccessPeerTransportRequest,
) (*s4wave_session.AccessPeerTransportResponse, error) {
	// Resolve the calling resource client and the Session's running transport.
	owner, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	st, releaseTransport, err := r.resolvePeerTransport(ctx)
	if err != nil {
		return nil, err
	}
	childBus := st.GetChildBus()
	if childBus == nil {
		releaseTransport()
		return nil, errors.New("account peer transport is unavailable")
	}

	// Run the requested listeners until the caller releases the resource.
	runCtx, runCancel := context.WithCancel(r.ctx)
	release := func() {
		runCancel()
		releaseTransport()
	}
	udpAddr, err := r.startPeerTransportServices(runCtx, st, req)
	if err != nil {
		release()
		return nil, err
	}

	// Expose the transport's stream API as a caller-owned resource.
	api, err := bifrost_api.NewAPI(childBus, &bifrost_api.Config{})
	if err != nil {
		release()
		return nil, err
	}
	mux := srpc.NewMux()
	if err := stream_api.SRPCRegisterStreamService(mux, api); err != nil {
		release()
		return nil, err
	}
	id, err := owner.AddResource(mux, release)
	if err != nil {
		release()
		return nil, err
	}
	return &s4wave_session.AccessPeerTransportResponse{
		ResourceId: id,
		PeerId:     st.GetPeerID().String(),
		UdpAddr:    udpAddr,
	}, nil
}

// resolvePeerTransport borrows the Session's running transport. A local
// account starts it on demand; a cloud account runs it while direct peer
// connections are enabled.
func (r *SessionResource) resolvePeerTransport(ctx context.Context) (*transport.SessionTransport, func(), error) {
	// Start the local account's transport when it is not running yet.
	privKey := r.session.GetPrivKey()
	if account, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount); ok {
		if err := account.EnsureConfiguredSessionTransport(ctx, privKey); err != nil {
			return nil, nil, err
		}
	}

	// Borrow the transport published for this Session's peer.
	sessionPeerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, nil, err
	}
	st, release, err := transport.ResolveSessionTransport(ctx, r.b, sessionPeerID, nil)
	if err != nil {
		return nil, nil, err
	}
	if st == nil {
		release()
		return nil, nil, errors.New("account peer transport is not running")
	}
	return st, release, nil
}

// startPeerTransportServices starts the UDP transport and the remote resource
// service req asks for, bound to runCtx. It returns the bound UDP address.
func (r *SessionResource) startPeerTransportServices(
	runCtx context.Context,
	st *transport.SessionTransport,
	req *s4wave_session.AccessPeerTransportRequest,
) (string, error) {
	// Serve this daemon's Resource service to account sessions.
	if req.GetServeResources() {
		if err := r.serveRemoteResources(runCtx, st); err != nil {
			return "", err
		}
	}

	// Start UDP when the caller listens or dials over it.
	if req.GetUdpListenAddr() == "" && len(req.GetUdpPeerAddrs()) == 0 {
		return "", nil
	}
	peerAddrs := make(map[peer.ID]string, len(req.GetUdpPeerAddrs()))
	for id, addr := range req.GetUdpPeerAddrs() {
		remote, err := peer.IDB58Decode(id)
		if err != nil {
			return "", errors.Wrapf(err, "parse UDP peer %q", id)
		}
		peerAddrs[remote] = addr
	}
	addr, err := st.StartUDP(runCtx, req.GetUdpListenAddr(), peerAddrs)
	if err != nil {
		return "", errors.Wrap(err, "start UDP transport")
	}
	return addr.String(), nil
}

// serveRemoteResources serves the Resource service over remote resource
// streams from account sessions until ctx ends.
func (r *SessionResource) serveRemoteResources(ctx context.Context, st *transport.SessionTransport) error {
	// Hold the Resource service for the lifetime of the handler.
	invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(
		ctx,
		r.b,
		resource.SRPCResourceServiceServiceID,
		"",
		true,
		nil,
	)
	if err != nil {
		return err
	}
	if len(invokers) == 0 {
		invokerRef.Release()
		return errors.New("resource service not found")
	}

	// Register the handler and drop it with the service when ctx ends.
	handler := &remoteResourceHandler{
		b:      st.GetChildBus(),
		server: srpc.NewServer(invokers[0]),
	}
	releaseHandler, err := st.ServeAuthorized(s4wave_session.RemoteResourceProtocolID, handler, refuseRemoteResources)
	if err != nil {
		invokerRef.Release()
		return err
	}
	context.AfterFunc(ctx, func() {
		releaseHandler()
		invokerRef.Release()
	})
	return nil
}

// remoteResourceHandler serves the Resource service over each admitted stream.
type remoteResourceHandler struct {
	// b holds the peer link while a stream is served.
	b bus.Bus
	// server dispatches the multiplexed Resource RPCs.
	server *srpc.Server
}

// HandleMountedStream multiplexes the stream and serves it until it closes.
func (h *remoteResourceHandler) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	// Keep the link to the remote session while its client is connected.
	_, elRef, err := h.b.AddDirective(
		link.NewEstablishLinkWithPeer(ms.GetLink().GetLocalPeer(), ms.GetPeerID()),
		nil,
	)
	if err != nil {
		return err
	}
	mc, err := srpc.NewMuxedConnWithRwc(ctx, ms.GetStream(), false, nil)
	if err != nil {
		elRef.Release()
		return err
	}
	go func() {
		defer elRef.Release()
		defer ms.GetStream().Close()
		_ = h.server.AcceptMuxedConn(ctx, mc)
	}()
	return nil
}

// remoteRefusalTimeout bounds how long a refused peer may hold its stream
// while it reads the refusal.
const remoteRefusalTimeout = 10 * time.Second

// refuseRemoteResources answers every call on a refused remote resource stream
// with err, so the client reports why it was refused.
func refuseRemoteResources(ctx context.Context, ms link.MountedStream, err error) {
	// Serve the refusal until the client hangs up or the timeout ends.
	ctx, cancel := context.WithTimeout(ctx, remoteRefusalTimeout)
	defer cancel()
	mc, muxErr := srpc.NewMuxedConnWithRwc(ctx, ms.GetStream(), false, nil)
	if muxErr != nil {
		return
	}
	_ = srpc.NewServer(refusedInvoker{err: err}).AcceptMuxedConn(ctx, mc)
}

// refusedInvoker fails every call with err.
type refusedInvoker struct {
	// err is the refusal.
	err error
}

// InvokeMethod fails the call with the refusal.
func (i refusedInvoker) InvokeMethod(_, _ string, _ srpc.Stream) (bool, error) {
	return true, i.err
}

// sharedObjectTransportPeer returns the verified endpoint retained by enrollment.
func (r *SessionResource) sharedObjectTransportPeer(sharedObjectID string) string {
	account, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok {
		return ""
	}
	for _, entry := range account.GetSOListCtr().GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == sharedObjectID {
			return entry.GetTransportPeerId()
		}
	}
	return ""
}

// _ is a type assertion
var (
	_ link.MountedStreamHandler = (*remoteResourceHandler)(nil)
	_ srpc.Invoker              = refusedInvoker{}
)
