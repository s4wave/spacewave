//go:build !js

// Package control implements the local daemon control protocol that
// backs spacewave stop and socket takeover between the CLI daemon
// and the desktop app's resource listener.
//
// The protocol is intentionally minimal: a Shutdown RPC for an explicit
// takeover and a Yield RPC for a manual start, each answered by a
// caller-supplied YieldPolicy. If the policy allows the request, the
// handler invokes the caller-supplied shutdown callback and returns
// success to the peer; if the policy denies it, the handler returns the
// policy's error to the peer so the caller can surface a clear message.
package control

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"

	emptypb "github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// ServiceID is the starpc service identifier for the daemon control RPC.
const ServiceID = "spacewave.cli.daemon"

// ShutdownMethodID is the method identifier for the Shutdown RPC, which
// asks the runtime to yield to an explicit takeover.
const ShutdownMethodID = "Shutdown"

// YieldMethodID is the method identifier for the Yield RPC, which asks the
// runtime to yield to a manually started one. Runtimes deny it unless they
// were started on demand for a command.
const YieldMethodID = "Yield"

// DenyErrorMarker is a substring embedded in denied takeover errors so
// callers can distinguish policy denials from transport failures.
const DenyErrorMarker = "spacewave.daemon.shutdown.denied:"

// YieldPolicy decides whether an incoming Shutdown or Yield RPC should be
// honored. Returning nil means the handler will fire its shutdown
// callback and acknowledge the peer; returning a non-nil error causes
// the handler to reply with that error so the peer sees a clear
// denial.
type YieldPolicy func(ctx context.Context) error

// AutoAllowPolicy is a YieldPolicy that always allows the takeover.
// It is suitable for the CLI daemon where the presence of a local
// socket is an unambiguous signal that the user launched spacewave
// serve.
func AutoAllowPolicy(context.Context) error { return nil }

// denyYieldPolicy is the default Yield policy: a runtime keeps its socket
// for a manual start unless its owner allows otherwise.
func denyYieldPolicy(context.Context) error {
	return errors.New("the running daemon does not yield to a manual start")
}

// Handler handles local daemon control RPCs. It is registered on the
// same mux that serves the Resource SDK so takeover flows can target
// a single socket regardless of which runtime owns it.
type Handler struct {
	policy      YieldPolicy
	yieldPolicy YieldPolicy
	shutdown    func()

	mtx              sync.Mutex
	claimed          bool
	shutdownGranted  func(context.Context)
	shutdownComplete chan struct{}
	completed        atomic.Bool
}

// NewHandler constructs a daemon control handler. policy decides
// whether to honor a Shutdown RPC; every Yield RPC is denied until
// SetYieldPolicy allows it. For the one permitted requester,
// shutdown runs before the completion acknowledgement and must
// synchronously release the listener and its socket path.
//
// If policy is nil, the handler auto-allows every request.
func NewHandler(policy YieldPolicy, shutdown func()) *Handler {
	if policy == nil {
		policy = AutoAllowPolicy
	}
	if shutdown == nil {
		shutdown = func() {}
	}
	return &Handler{
		policy:           policy,
		yieldPolicy:      denyYieldPolicy,
		shutdown:         shutdown,
		shutdownComplete: make(chan struct{}),
	}
}

// ShutdownComplete closes after the granted shutdown invocation finishes
// writing its acknowledgement, including the final stream close attempt.
func (h *Handler) ShutdownComplete() <-chan struct{} {
	return h.shutdownComplete
}

// SetShutdownGrantedCallback sets fn to run after a Shutdown request passes
// policy and claims the handler. fn receives the granted stream context.
func (h *Handler) SetShutdownGrantedCallback(fn func(context.Context)) {
	h.mtx.Lock()
	h.shutdownGranted = fn
	h.mtx.Unlock()
}

// SetYieldPolicy sets the policy that decides whether to honor a Yield RPC.
// Call it before registering the handler.
func (h *Handler) SetYieldPolicy(policy YieldPolicy) {
	h.mtx.Lock()
	h.yieldPolicy = policy
	h.mtx.Unlock()
}

// GetServiceID returns the service identifier.
func (h *Handler) GetServiceID() string {
	return ServiceID
}

// GetMethodIDs returns the supported method identifiers.
func (h *Handler) GetMethodIDs() []string {
	return []string{ShutdownMethodID, YieldMethodID}
}

// InvokeMethod handles the Shutdown and Yield RPCs. The handler consults
// the method's YieldPolicy and grants at most one caller. On approval it
// notifies the granted-requester callback, releases the listener before
// acknowledging the peer, then signals ShutdownComplete after
// response-stream completion. On policy error or an already-claimed
// handoff, it returns a wrapped denial to the peer.
func (h *Handler) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Match a daemon control route and select its policy.
	if serviceID != ServiceID {
		return false, nil
	}
	var policy YieldPolicy
	switch methodID {
	case ShutdownMethodID:
		policy = h.policy
	case YieldMethodID:
		h.mtx.Lock()
		policy = h.yieldPolicy
		h.mtx.Unlock()
	default:
		return false, nil
	}

	// Receive the request from the control stream.
	req := &emptypb.Empty{}
	if err := strm.MsgRecv(req); err != nil && err != io.EOF {
		return true, err
	}

	// Require the method's yield policy to allow this requester.
	ctx := strm.Context()
	if err := policy(ctx); err != nil {
		return true, errors.Errorf("%s %s", DenyErrorMarker, err.Error())
	}

	// Claim the shutdown handoff for one requester under the handler lock.
	h.mtx.Lock()
	if h.claimed {
		h.mtx.Unlock()
		return true, errors.Errorf("%s takeover already granted to another requester", DenyErrorMarker)
	}
	h.claimed = true
	shutdownGranted := h.shutdownGranted
	h.mtx.Unlock()

	// Notify the granted requester before releasing the daemon listener.
	if shutdownGranted != nil {
		shutdownGranted(ctx)
	}

	// Release the socket before acknowledging the requester. If the
	// process or connection disappears before the acknowledgement, the
	// requester verifies that no listener remains and reclaims the stale
	// path.
	h.shutdown()
	defer func() {
		if h.completed.CompareAndSwap(false, true) {
			close(h.shutdownComplete)
		}
	}()
	if err := strm.MsgSend(&emptypb.Empty{}); err != nil {
		return true, err
	}
	return true, strm.CloseSend()
}

// RequestShutdown issues the Shutdown RPC over conn and waits for both the
// peer's acknowledgement and its stream-completion event. If the peer denies
// the takeover, the returned error is a DenyError describing the denial reason.
// Callers are responsible for closing conn.
func RequestShutdown(ctx context.Context, conn net.Conn) error {
	return requestHandoff(ctx, conn, ShutdownMethodID)
}

// RequestYield issues the Yield RPC over conn with RequestShutdown's
// completion and denial semantics. Callers are responsible for closing conn.
func RequestYield(ctx context.Context, conn net.Conn) error {
	return requestHandoff(ctx, conn, YieldMethodID)
}

// requestHandoff issues a Shutdown or Yield RPC over conn and waits for the
// peer's acknowledgement and stream completion.
func requestHandoff(ctx context.Context, conn net.Conn, methodID string) error {
	// Connect an SRPC client to the daemon control connection.
	client, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		return errors.Wrap(err, "create daemon control client")
	}

	// Open the request stream and retain it through completion.
	strm, err := client.NewStream(
		ctx,
		ServiceID,
		methodID,
		&emptypb.Empty{},
	)
	if err != nil {
		return wrapRequestError(methodID, err)
	}
	defer strm.Close()

	// Require the daemon's acknowledgement of the request.
	if err := strm.MsgRecv(&emptypb.Empty{}); err != nil {
		return wrapRequestError(methodID, err)
	}

	// Require stream completion after the daemon's single acknowledgement.
	if err := strm.MsgRecv(&emptypb.Empty{}); err != io.EOF {
		if err == nil {
			return errors.Errorf(
				"request daemon %s: unexpected response after acknowledgement",
				methodID,
			)
		}
		return wrapRequestError(methodID, err)
	}
	return nil
}

// wrapRequestError returns a DenyError for a peer denial and wraps any other
// failure of the methodID request.
func wrapRequestError(methodID string, err error) error {
	if denyReason, ok := extractDenyReason(err); ok {
		return &DenyError{Reason: denyReason}
	}
	return errors.Wrapf(err, "request daemon %s", methodID)
}

// _ is a type assertion
var _ srpc.Handler = (*Handler)(nil)
