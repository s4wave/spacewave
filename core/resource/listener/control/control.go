//go:build !js

// Package control implements the local daemon control protocol that
// backs spacewave stop and socket takeover between the CLI daemon
// and the desktop app's resource listener.
//
// The protocol is intentionally minimal: a single Shutdown RPC whose
// handler consults a caller-supplied YieldPolicy. If the policy
// allows the takeover, the handler invokes the caller-supplied
// shutdown callback and returns success to the peer; if the policy
// denies the request, the handler returns the policy's error to the
// peer so the caller can surface a clear message.
package control

import (
	"context"
	"io"
	"net"
	"strings"

	emptypb "github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// ServiceID is the starpc service identifier for the daemon control RPC.
const ServiceID = "spacewave.cli.daemon"

// ShutdownMethodID is the method identifier for the Shutdown RPC.
const ShutdownMethodID = "Shutdown"

// DenyErrorMarker is a substring embedded in denied takeover errors so
// callers can distinguish policy denials from transport failures.
const DenyErrorMarker = "spacewave.daemon.shutdown.denied:"

// YieldPolicy decides whether an incoming Shutdown RPC should be
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

// Handler handles local daemon control RPCs. It is registered on the
// same mux that serves the Resource SDK so takeover flows can target
// a single socket regardless of which runtime owns it.
type Handler struct {
	policy   YieldPolicy
	shutdown func()
}

// NewHandler constructs a daemon control handler. policy decides
// whether to honor a Shutdown RPC; shutdown is invoked once after the
// handler acknowledges a permitted request. shutdown must be safe to
// call from an RPC goroutine and from multiple goroutines.
//
// If policy is nil, the handler auto-allows every request (matching
// the legacy "always allow" behavior of the CLI daemon).
func NewHandler(policy YieldPolicy, shutdown func()) *Handler {
	if policy == nil {
		policy = AutoAllowPolicy
	}
	if shutdown == nil {
		shutdown = func() {}
	}
	return &Handler{policy: policy, shutdown: shutdown}
}

// GetServiceID returns the service identifier.
func (h *Handler) GetServiceID() string {
	return ServiceID
}

// GetMethodIDs returns the supported method identifiers.
func (h *Handler) GetMethodIDs() []string {
	return []string{ShutdownMethodID}
}

// InvokeMethod handles the Shutdown RPC. The handler consults its
// YieldPolicy; on nil it acknowledges the peer and fires the shutdown
// callback. On policy error, it returns a wrapped error that the peer
// sees as the RPC result.
func (h *Handler) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if serviceID != ServiceID || methodID != ShutdownMethodID {
		return false, nil
	}

	req := &emptypb.Empty{}
	if err := strm.MsgRecv(req); err != nil && err != io.EOF {
		return true, err
	}

	ctx := strm.Context()
	if err := h.policy(ctx); err != nil {
		return true, errors.Errorf("%s %s", DenyErrorMarker, err.Error())
	}

	if err := strm.MsgSend(&emptypb.Empty{}); err != nil {
		return true, err
	}
	if err := strm.CloseSend(); err != nil {
		return true, err
	}
	h.shutdown()
	return true, nil
}

// RequestShutdown issues the Shutdown RPC over conn and waits for the
// peer's acknowledgement. If the peer denies the takeover, the
// returned error is a DenyError describing the denial reason.
// Callers are responsible for closing conn.
func RequestShutdown(ctx context.Context, conn net.Conn) error {
	client, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		return errors.Wrap(err, "create daemon control client")
	}
	if err := client.ExecCall(ctx, ServiceID, ShutdownMethodID, &emptypb.Empty{}, &emptypb.Empty{}); err != nil {
		if denyReason, ok := extractDenyReason(err); ok {
			return &DenyError{Reason: denyReason}
		}
		return errors.Wrap(err, "request daemon shutdown")
	}
	return nil
}

// DenyError indicates that the peer explicitly denied the takeover.
// CLI callers use errors.As to distinguish this from generic RPC
// transport errors and print a clearer message.
type DenyError struct {
	// Reason is the peer-supplied denial reason.
	Reason string
}

// Error implements error.
func (e *DenyError) Error() string {
	if e.Reason == "" {
		return "takeover denied by peer"
	}
	return e.Reason
}

// extractDenyReason extracts the deny reason from an error string if
// it contains the DenyErrorMarker embedded by InvokeMethod.
func extractDenyReason(err error) (string, bool) {
	msg := err.Error()
	_, after, ok := strings.Cut(msg, DenyErrorMarker)
	if !ok {
		return "", false
	}
	reason := strings.TrimSpace(after)
	return reason, true
}

// _ is a type assertion
var _ srpc.Handler = (*Handler)(nil)
