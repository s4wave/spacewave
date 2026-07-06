//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"net"

	emptypb "github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
)

const (
	devicePolicyControlServiceID = "spacewave.cli.device_policy"
	devicePolicyReloadMethodID   = "Reload"
)

// newDaemonControlHandler constructs a daemon control handler that
// invokes requestShutdown when the peer issues the Shutdown RPC.
// The CLI daemon always yields when asked; the policy is AutoAllow.
func newDaemonControlHandler(requestShutdown func()) *listener_control.Handler {
	return listener_control.NewHandler(listener_control.AutoAllowPolicy, requestShutdown)
}

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
