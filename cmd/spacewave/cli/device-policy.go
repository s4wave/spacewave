//go:build !js

package spacewave_cli

import (
	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
)

// devicePolicyReloadDaemon asks the daemon to reload its policy file.
var devicePolicyReloadDaemon = requestDevicePolicyReload

// newDevicePolicyCommand builds the policy command group.
func newDevicePolicyCommand() *cli.Command {
	return &cli.Command{
		Name:  "policy",
		Usage: "manage daemon-local Device policy",
		Subcommands: []*cli.Command{
			newDevicePolicyForgeWorkerCommand(),
			newDevicePolicyNodeTypeCommand(),
		},
	}
}

// runDevicePolicyMutation applies mutate to the policy file and reloads the
// daemon.
func runDevicePolicyMutation(
	c *cli.Context,
	statePath string,
	mutate func(*device_policy.DevicePolicy) error,
) error {
	return runDevicePolicyMutationValidated(c, statePath, mutate, nil)
}

// runDevicePolicyMutationValidated applies mutate to the policy file, checks the
// result with validate when it is not nil, and reloads the daemon.
func runDevicePolicyMutationValidated(
	c *cli.Context,
	statePath string,
	mutate func(*device_policy.DevicePolicy) error,
	validate func(*sdkClient, string, *device_policy.DevicePolicy) error,
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

	// Mutate, validate, and write the policy, then reload the daemon.
	policy, err := device_policy.ReadFile(resolvedStatePath)
	if err != nil {
		return err
	}
	if err := mutate(policy); err != nil {
		return err
	}
	if validate != nil {
		if err := validate(client, resolvedStatePath, policy); err != nil {
			return err
		}
	}
	policy.Revision++
	if err := device_policy.WriteFile(resolvedStatePath, policy); err != nil {
		return err
	}
	return devicePolicyReloadDaemon(ctx, client)
}
