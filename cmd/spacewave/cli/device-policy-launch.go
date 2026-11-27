//go:build !js

package spacewave_cli

import (
	"slices"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
)

// devicePolicyLaunchArgs selects the daemon and Space whose launch approval changes.
type devicePolicyLaunchArgs struct {
	// statePath is the daemon state directory holding the policy file.
	statePath string
	// sessionIdx is the session that resolves the Space name.
	sessionIdx int
	// space is the Space ID or name the approval covers.
	space string
	// hostExecution permits the ordinary host launcher.
	hostExecution bool
	// childModels are the additional child model selectors granted.
	childModels cli.StringSlice
	// secretPlugins are the plugins granted the Space's Secrets.
	secretPlugins cli.StringSlice
}

// newDevicePolicyLaunchCommand builds the launch approval command group.
func newDevicePolicyLaunchCommand() *cli.Command {
	return &cli.Command{
		Name:  "launch",
		Usage: "manage which execution profiles this Device launches for each Space",
		Subcommands: []*cli.Command{
			newDevicePolicyLaunchApproveCommand(),
			newDevicePolicyLaunchRevokeCommand(),
		},
	}
}

// newDevicePolicyLaunchApproveCommand builds the launch approve subcommand.
func newDevicePolicyLaunchApproveCommand() *cli.Command {
	args := &devicePolicyLaunchArgs{}
	return &cli.Command{
		Name:   "approve",
		Usage:  "set this Device's launch approval for a Space, replacing any earlier one",
		Flags:  args.BuildApproveFlags(),
		Action: args.RunApprove,
	}
}

// newDevicePolicyLaunchRevokeCommand builds the launch revoke subcommand.
func newDevicePolicyLaunchRevokeCommand() *cli.Command {
	args := &devicePolicyLaunchArgs{}
	return &cli.Command{
		Name:   "revoke",
		Usage:  "remove this Device's launch approval for a Space",
		Flags:  args.BuildFlags(),
		Action: args.RunRevoke,
	}
}

// BuildFlags returns the daemon and Space selection flags.
func (a *devicePolicyLaunchArgs) BuildFlags() []cli.Flag {
	return append(daemonClientFlags(&a.statePath),
		&cli.IntFlag{
			Name:        "session-index",
			Usage:       "session index that resolves the Space name",
			EnvVars:     []string{"SPACEWAVE_SESSION_INDEX"},
			Value:       1,
			Destination: &a.sessionIdx,
		},
		&cli.StringFlag{
			Name:        "space",
			Usage:       "Space ID or name the approval covers",
			Required:    true,
			Destination: &a.space,
		},
	)
}

// BuildApproveFlags returns the selection flags and the granted profile.
func (a *devicePolicyLaunchArgs) BuildApproveFlags() []cli.Flag {
	return append(a.BuildFlags(),
		&cli.BoolFlag{
			Name:        "host-execution",
			Usage:       "run this Space's work directly on this Device with no process sandbox",
			Destination: &a.hostExecution,
		},
		&cli.StringSliceFlag{
			Name:        "allow-child-model",
			Usage:       "grant agents launched for this Space an additional child model selector",
			Destination: &a.childModels,
		},
		&cli.StringSliceFlag{
			Name:        "allow-secret-plugin",
			Usage:       "let this plugin read, create and write the Space's Secrets when the Space loads it, such as glados-core",
			Destination: &a.secretPlugins,
		},
	)
}

// RunApprove replaces the Space's launch approval with the flags' profile.
func (a *devicePolicyLaunchArgs) RunApprove(c *cli.Context) error {
	// Normalize the granted child models and Secret plugins.
	models, err := uniqueFlagValues(a.childModels.Value(), "child model selector")
	if err != nil {
		return err
	}
	secretPlugins, err := uniqueFlagValues(a.secretPlugins.Value(), "Secret plugin ID")
	if err != nil {
		return err
	}

	// Replace the Space's entry, or append one.
	return runDevicePolicyMutation(c, a.statePath, func(client *sdkClient, policy *device_policy.DevicePolicy) error {
		// Resolve the named Space in the daemon's session.
		spaceID, err := a.resolveSpace(c, client)
		if err != nil {
			return err
		}

		// Swap the Space's entry for the new profile.
		approval := &device_policy.LaunchApproval{
			SpaceId:            spaceID,
			HostExecution:      a.hostExecution,
			AllowedChildModels: models,
			SecretPluginIds:    secretPlugins,
		}
		policy.LaunchApproval = slices.DeleteFunc(policy.LaunchApproval, func(entry *device_policy.LaunchApproval) bool {
			return entry.GetSpaceId() == spaceID
		})
		policy.LaunchApproval = append(policy.LaunchApproval, approval)
		return nil
	})
}

// RunRevoke removes the Space's launch approval.
func (a *devicePolicyLaunchArgs) RunRevoke(c *cli.Context) error {
	return runDevicePolicyMutation(c, a.statePath, func(client *sdkClient, policy *device_policy.DevicePolicy) error {
		// Resolve the named Space in the daemon's session.
		spaceID, err := a.resolveSpace(c, client)
		if err != nil {
			return err
		}

		// Drop the Space's entry, refusing a revoke that changes nothing.
		before := len(policy.LaunchApproval)
		policy.LaunchApproval = slices.DeleteFunc(policy.LaunchApproval, func(entry *device_policy.LaunchApproval) bool {
			return entry.GetSpaceId() == spaceID
		})
		if len(policy.LaunchApproval) == before {
			return errors.Errorf("this Device holds no launch approval for space %q", spaceID)
		}
		return nil
	})
}

// resolveSpace resolves the --space name to its Space ID in the selected session.
func (a *devicePolicyLaunchArgs) resolveSpace(c *cli.Context, client *sdkClient) (string, error) {
	// Mount the selected session for its Space list.
	sessionIdx, err := sessionIndexFromInt(a.sessionIdx)
	if err != nil {
		return "", err
	}
	sess, err := client.mountSession(c.Context, sessionIdx)
	if err != nil {
		return "", err
	}
	defer sess.Release()
	return client.resolveSpaceID(c.Context, sess, a.space)
}

// uniqueFlagValues trims values and keeps each once in argument order,
// refusing a blank value of the kind described by what.
func uniqueFlagValues(values []string, what string) ([]string, error) {
	var out []string
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.Errorf("%s is required", what)
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out, nil
}
