//go:build !js

package spacewave_cli

import (
	"context"
	"io"

	emptypb "github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

const (
	// devicePolicyControlServiceID identifies the Device policy control service.
	devicePolicyControlServiceID = "spacewave.cli.device_policy"
	// devicePolicyReloadMethodID identifies the Device policy reload method.
	devicePolicyReloadMethodID = "Reload"
)

// devicePolicyControlHandler reloads the daemon's Device policy on request.
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
	// Reload device policy when the control method is invoked.
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
