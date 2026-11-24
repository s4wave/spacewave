//go:build !js

package spacewave_cli

import (
	"slices"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
)

// devicePolicySpaceArgs selects the daemon, session and Space whose service changes.
type devicePolicySpaceArgs struct {
	// statePath is the daemon state directory holding the policy file.
	statePath string
	// sessionIdx is the session this Device reaches the Space through.
	sessionIdx int
	// space is the Space ID or name.
	space string
}

// newDevicePolicySpaceCommand builds the served Space command group.
func newDevicePolicySpaceCommand() *cli.Command {
	add := &devicePolicySpaceArgs{}
	remove := &devicePolicySpaceArgs{}
	return &cli.Command{
		Name:  "space",
		Usage: "manage which Spaces this Device runs its placed Flowgraph nodes in",
		Subcommands: []*cli.Command{
			{
				Name:   "add",
				Usage:  "serve a Space: write this Device's object in it and run the nodes placed on that object",
				Flags:  add.BuildFlags(),
				Action: add.RunAdd,
			},
			{
				Name:   "remove",
				Usage:  "stop serving a Space, leaving this Device's object in it",
				Flags:  remove.BuildFlags(),
				Action: remove.RunRemove,
			},
		},
	}
}

// BuildFlags returns the daemon, session and Space selection flags.
func (a *devicePolicySpaceArgs) BuildFlags() []cli.Flag {
	return append(daemonClientFlags(&a.statePath),
		&cli.IntFlag{
			Name:        "session-index",
			Usage:       "session this Device reaches the Space through; its peer is the Device in the Space",
			EnvVars:     []string{"SPACEWAVE_SESSION_INDEX"},
			Value:       1,
			Destination: &a.sessionIdx,
		},
		&cli.StringFlag{
			Name:        "space",
			Usage:       "Space ID or name",
			Required:    true,
			Destination: &a.space,
		},
	)
}

// RunAdd lists the Space as served. Its Reconciler mounts the Space through
// the session and writes this Device's object there when the Space lacks one.
func (a *devicePolicySpaceArgs) RunAdd(c *cli.Context) error {
	sessionIdx, err := sessionIndexFromInt(a.sessionIdx)
	if err != nil {
		return err
	}
	return runDevicePolicyMutation(c, a.statePath, func(client *sdkClient, policy *device_policy.DevicePolicy) error {
		// Resolve the named Space in the session and list it.
		spaceID, err := resolveSessionSpace(c, client, sessionIdx, a.space)
		if err != nil {
			return err
		}
		setServedSpace(policy, spaceID, sessionIdx)
		return nil
	})
}

// setServedSpace lists spaceID as served through the session sessionIdx,
// replacing any entry the Space already has.
func setServedSpace(policy *device_policy.DevicePolicy, spaceID string, sessionIdx uint32) {
	policy.ServedSpace = slices.DeleteFunc(policy.ServedSpace, func(entry *device_policy.ServedSpace) bool {
		return entry.GetSpaceId() == spaceID
	})
	policy.ServedSpace = append(policy.ServedSpace, &device_policy.ServedSpace{SpaceId: spaceID, SessionIndex: sessionIdx})
}

// RunRemove drops the Space from the served list. Its Reconciler stops, and
// the Device object stays for the Space's members to see or delete.
func (a *devicePolicySpaceArgs) RunRemove(c *cli.Context) error {
	sessionIdx, err := sessionIndexFromInt(a.sessionIdx)
	if err != nil {
		return err
	}
	return runDevicePolicyMutation(c, a.statePath, func(client *sdkClient, policy *device_policy.DevicePolicy) error {
		// Resolve the named Space in the session.
		spaceID, err := resolveSessionSpace(c, client, sessionIdx, a.space)
		if err != nil {
			return err
		}

		// Drop the Space's entry, refusing a remove that changes nothing.
		if !policy.ServesSpace(spaceID) {
			return errors.Errorf("this Device does not serve space %q", spaceID)
		}
		policy.ServedSpace = slices.DeleteFunc(policy.ServedSpace, func(entry *device_policy.ServedSpace) bool {
			return entry.GetSpaceId() == spaceID
		})
		return nil
	})
}

// resolveSessionSpace resolves a Space ID or name to its Space ID in the
// session sessionIdx.
func resolveSessionSpace(c *cli.Context, client *sdkClient, sessionIdx uint32, space string) (string, error) {
	sess, err := client.mountSession(c.Context, sessionIdx)
	if err != nil {
		return "", err
	}
	defer sess.Release()
	return client.resolveSpaceID(c.Context, sess, space)
}
