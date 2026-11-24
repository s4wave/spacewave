//go:build !js

package spacewave_cli

import (
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

	// Mutate and write the policy, then reload the daemon.
	policy, err := device_policy.ReadFile(resolvedStatePath)
	if err != nil {
		return err
	}
	if err := mutate(client, policy); err != nil {
		return err
	}
	policy.Revision++
	if err := device_policy.WriteFile(resolvedStatePath, policy); err != nil {
		return err
	}
	return requestDevicePolicyReload(ctx, client)
}
