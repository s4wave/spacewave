//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
)

// requestDevicePolicyReload asks the daemon to reread its policy file.
func requestDevicePolicyReload(ctx context.Context, client *sdkClient) error {
	if client == nil || client.srpc == nil {
		return errors.New("daemon control client unavailable")
	}
	_, err := device_policy.NewSRPCDevicePolicyServiceClient(client.srpc).Reload(ctx, &device_policy.ReloadDevicePolicyRequest{})
	return err
}
