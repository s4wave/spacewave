//go:build !js

package spacewave_cli

import (
	"context"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
)

// newDevicePolicyCommand builds the policy command group.
func newDevicePolicyCommand() *cli.Command {
	return &cli.Command{
		Name:  "policy",
		Usage: "manage daemon-local Device policy",
		Subcommands: []*cli.Command{
			newDevicePolicyNodeTypeCommand(),
			newDevicePolicyLaunchCommand(),
			newDevicePolicySpaceCommand(),
		},
	}
}

// runDevicePolicyMutation applies mutate to the policy file and reloads the
// daemon. mutate receives the connected daemon client, such as to resolve a
// Space name.
func runDevicePolicyMutation(
	c *cli.Context,
	statePath string,
	mutate func(*sdkClient, *device_policy.DevicePolicy) error,
) error {
	// Connect to the daemon for the policy mutation.
	ctx := c.Context
	resolvedStatePath, _, err := resolveDeviceDaemonPaths(c, statePath)
	if err != nil {
		return err
	}
	client, err := connectDaemonWithResolvedFallback(ctx, c, resolvedStatePath)
	if err != nil {
		return errors.Wrap(err, "connect daemon")
	}
	defer client.close()
	return mutateDevicePolicy(ctx, client, resolvedStatePath, mutate)
}

// mutateDevicePolicy applies mutate to the policy file under statePath,
// writes it under the next revision, and reloads the daemon client reaches.
func mutateDevicePolicy(
	ctx context.Context,
	client *sdkClient,
	statePath string,
	mutate func(*sdkClient, *device_policy.DevicePolicy) error,
) error {
	// Read, mutate and write the policy, then reload the daemon.
	policy, err := device_policy.ReadFile(statePath)
	if err != nil {
		return err
	}
	if err := mutate(client, policy); err != nil {
		return err
	}
	policy.Revision++
	if err := device_policy.WriteFile(statePath, policy); err != nil {
		return err
	}
	return requestDevicePolicyReload(ctx, client)
}
