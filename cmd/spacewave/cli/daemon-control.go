//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"net"
	"sync/atomic"

	emptypb "github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
)

const (
	devicePolicyControlServiceID = "spacewave.cli.device_policy"
	devicePolicyReloadMethodID   = "Reload"
)

// daemonControlHandler wraps the shared daemon-control handler and records the
// granted connection so the serve loop can wait for that requester to read its
// acknowledgement and close before draining.
type daemonControlHandler struct {
	*listener_control.Handler
	shutdownConn atomic.Pointer[trackedConn]
}

// newDaemonControlHandler constructs a daemon control handler that invokes
// requestShutdown when the peer issues the Shutdown RPC. The CLI daemon always
// yields when asked; the policy is AutoAllow.
func newDaemonControlHandler(requestShutdown func()) *daemonControlHandler {
	return newDaemonControlHandlerWithPolicy(listener_control.AutoAllowPolicy, requestShutdown)
}

func newDaemonControlHandlerWithPolicy(
	policy listener_control.YieldPolicy,
	requestShutdown func(),
) *daemonControlHandler {
	h := &daemonControlHandler{
		Handler: listener_control.NewHandler(policy, requestShutdown),
	}
	h.SetShutdownGrantedCallback(func(ctx context.Context) {
		if tc, ok := ctx.Value(daemonConnCtxKey{}).(*trackedConn); ok {
			h.shutdownConn.Store(tc)
		}
	})
	return h
}

// _ is a type assertion
var _ srpc.Handler = (*daemonControlHandler)(nil)

type devicePolicyControlHandler struct {
	reload func() error
}

// newDevicePolicyControlHandler constructs the Device policy reload handler.
func newDevicePolicyControlHandler(reload func() error) *devicePolicyControlHandler {
	if reload == nil {
		reload = func() error { return nil }
	}
	return &devicePolicyControlHandler{reload: reload}
}

// GetServiceID returns the Device policy control service identifier.
func (h *devicePolicyControlHandler) GetServiceID() string {
	return devicePolicyControlServiceID
}

// GetMethodIDs returns the Device policy control method identifiers.
func (h *devicePolicyControlHandler) GetMethodIDs() []string {
	return []string{devicePolicyReloadMethodID}
}

// InvokeMethod handles Device policy reload requests.
func (h *devicePolicyControlHandler) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if serviceID != devicePolicyControlServiceID || methodID != devicePolicyReloadMethodID {
		return false, nil
	}
	req := &emptypb.Empty{}
	if err := strm.MsgRecv(req); err != nil && err != io.EOF {
		return true, err
	}
	if err := h.reload(); err != nil {
		return true, err
	}
	if err := strm.MsgSend(&emptypb.Empty{}); err != nil {
		return true, err
	}
	return true, strm.CloseSend()
}

// requestDevicePolicyReload issues the Device policy reload RPC.
func requestDevicePolicyReload(ctx context.Context, client *sdkClient) error {
	if client == nil || client.srpc == nil {
		return errors.New("daemon control client unavailable")
	}
	return client.srpc.ExecCall(
		ctx,
		devicePolicyControlServiceID,
		devicePolicyReloadMethodID,
		&emptypb.Empty{},
		&emptypb.Empty{},
	)
}

// _ is a type assertion
var _ srpc.Handler = (*devicePolicyControlHandler)(nil)

// requestDaemonShutdown issues the Shutdown RPC over conn.
func requestDaemonShutdown(ctx context.Context, conn net.Conn) error {
	return listener_control.RequestShutdown(ctx, conn)
}
