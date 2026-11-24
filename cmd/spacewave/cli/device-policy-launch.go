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
	)
}

// RunApprove replaces the Space's launch approval with the flags' profile.
func (a *devicePolicyLaunchArgs) RunApprove(c *cli.Context) error {
	// Normalize the granted child models in argument order, once each.
	var models []string
	seen := make(map[string]struct{})
	for _, model := range a.childModels.Value() {
		model = strings.TrimSpace(model)
		if model == "" {
			return errors.New("child model selector is required")
		}
		if _, ok := seen[model]; !ok {
			seen[model] = struct{}{}
			models = append(models, model)
		}
	}

	// Replace the Space's entry, or append one.
	return runDevicePolicyMutation(c, a.statePath, func(client *sdkClient, policy *device_policy.DevicePolicy) error {
		// Resolve the named Space in the daemon's session.
		spaceID, err := a.resolveSpace(c, client)
		if err != nil {
			return err
		}

		// Swap the Space's entry for the new profile.
		approval := &device_policy.LaunchApproval{SpaceId: spaceID, HostExecution: a.hostExecution, AllowedChildModels: models}
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
