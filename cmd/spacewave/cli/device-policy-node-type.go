//go:build !js

package spacewave_cli

import (
	"slices"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
)

// devicePolicyNodeTypeArgs selects the daemon whose node type allow list changes.
type devicePolicyNodeTypeArgs struct {
	// statePath is the daemon state directory holding the policy file.
	statePath string
}

// newDevicePolicyNodeTypeCommand builds the node-type command group.
func newDevicePolicyNodeTypeCommand() *cli.Command {
	return &cli.Command{
		Name:  "node-type",
		Usage: "manage the Flowgraph node types this Device runs",
		Subcommands: []*cli.Command{
			newDevicePolicyNodeTypeAddCommand(),
			newDevicePolicyNodeTypeRemoveCommand(),
		},
	}
}

// newDevicePolicyNodeTypeAddCommand builds the node-type add subcommand.
func newDevicePolicyNodeTypeAddCommand() *cli.Command {
	args := &devicePolicyNodeTypeArgs{}
	return &cli.Command{
		Name:      "add",
		Usage:     "allow this Device to run a Flowgraph node type",
		ArgsUsage: "<type-id>...",
		Flags:     args.BuildFlags(),
		Action:    args.RunAdd,
	}
}

// newDevicePolicyNodeTypeRemoveCommand builds the node-type remove subcommand.
func newDevicePolicyNodeTypeRemoveCommand() *cli.Command {
	args := &devicePolicyNodeTypeArgs{}
	return &cli.Command{
		Name:      "remove",
		Usage:     "stop this Device from running a Flowgraph node type",
		ArgsUsage: "<type-id>...",
		Flags:     args.BuildFlags(),
		Action:    args.RunRemove,
	}
}

// BuildFlags returns flags for node type allow list mutation.
func (a *devicePolicyNodeTypeArgs) BuildFlags() []cli.Flag {
	return daemonClientFlags(&a.statePath)
}

// RunAdd adds each named node type to the allow list.
func (a *devicePolicyNodeTypeArgs) RunAdd(c *cli.Context) error {
	typeIDs, err := nodeTypeIDArgs(c)
	if err != nil {
		return err
	}
	return runDevicePolicyMutation(c, a.statePath, func(policy *device_policy.DevicePolicy) error {
		for _, typeID := range typeIDs {
			if !slices.Contains(policy.GetNodeTypeId(), typeID) {
				policy.NodeTypeId = append(policy.NodeTypeId, typeID)
			}
		}
		return nil
	})
}

// RunRemove removes each named node type from the allow list.
func (a *devicePolicyNodeTypeArgs) RunRemove(c *cli.Context) error {
	typeIDs, err := nodeTypeIDArgs(c)
	if err != nil {
		return err
	}
	return runDevicePolicyMutation(c, a.statePath, func(policy *device_policy.DevicePolicy) error {
		for _, typeID := range typeIDs {
			if !slices.Contains(policy.GetNodeTypeId(), typeID) {
				return errors.Errorf("node type %q is not allowed", typeID)
			}
		}
		policy.NodeTypeId = slices.DeleteFunc(policy.NodeTypeId, func(typeID string) bool {
			return slices.Contains(typeIDs, typeID)
		})
		return nil
	})
}

// nodeTypeIDArgs returns the trimmed node type IDs named on the command line.
func nodeTypeIDArgs(c *cli.Context) ([]string, error) {
	// Require at least one non-empty node type ID.
	if c.NArg() == 0 {
		return nil, errors.New("node-type requires at least one <type-id>")
	}
	typeIDs := make([]string, 0, c.NArg())
	for _, arg := range c.Args().Slice() {
		typeID := strings.TrimSpace(arg)
		if typeID == "" {
			return nil, errors.New("node type id is required")
		}
		typeIDs = append(typeIDs, typeID)
	}
	return typeIDs, nil
}
