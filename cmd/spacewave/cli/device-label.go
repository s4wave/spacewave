//go:build !js

package spacewave_cli

import (
	"context"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// deviceLabelArgs selects the Space and Device whose stored labels change.
type deviceLabelArgs struct {
	// statePath is the daemon state directory.
	statePath string
	// sessionIdx is the mounted session holding the Device's Space.
	sessionIdx uint
	// spaceID selects the Space, or defaults to the only Space.
	spaceID string
}

// newDeviceLabelCommand builds the device label subcommand.
func newDeviceLabelCommand() *cli.Command {
	args := &deviceLabelArgs{}
	return &cli.Command{Name: "label", Usage: "set or remove Device labels (example: device label devices/forge gpu=true old-key-)", ArgsUsage: "<device-object-key> <key=value|key->...", Flags: args.BuildFlags(), Action: args.Run}
}

// BuildFlags returns the standard mounted-Space selection flags.
func (a *deviceLabelArgs) BuildFlags() []cli.Flag {
	return append(clientFlags(&a.statePath, &a.sessionIdx), &cli.StringFlag{Name: "space", Usage: "Space ID or name (default: the only Space)", EnvVars: []string{"SPACEWAVE_SPACE"}, Destination: &a.spaceID})
}

// Run changes stored Device labels through the mounted World.
func (a *deviceLabelArgs) Run(c *cli.Context) error {
	// Require a target and at least one set or removal.
	if c.NArg() < 2 {
		return errors.New("device label requires <device-object-key> <key=value|key->...")
	}
	uri := fsURI{sessionIdx: sessionIndex32(a.sessionIdx), spaceID: a.spaceID, objectKey: c.Args().First()}
	mount, err := mountObjectChain(c, a.statePath, uri, nil)
	if err != nil {
		return err
	}
	defer mount.release()

	// Commit all label edits together, replaying against concurrent Device reports.
	return labelDeviceInWorld(c.Context, mount.Engine, mount.ObjectKey, c.Args().Slice()[1:])
}

// labelDeviceInWorld updates only stored labels, keeping daemon-owned fields.
func labelDeviceInWorld(ctx context.Context, engine world.Engine, key string, changes []string) error {
	return world.ExecTransaction(ctx, engine, true, func(ctx context.Context, ws world.WorldState) error {
		// Require a Device before editing an independently writable labels map.
		if err := world_types.CheckObjectType(ctx, ws, key, s4wave_device.DeviceTypeID); err != nil {
			return err
		}
		device, err := world.LookupObjectBody[s4wave_device.Device](ctx, ws, key)
		if err != nil {
			return err
		}
		if device.Labels == nil {
			device.Labels = make(map[string]string)
		}

		// Apply typed CLI arguments in order; an invalid edit discards the whole write.
		for _, change := range changes {
			label, value, set := strings.Cut(change, "=")
			if !set {
				var remove bool
				label, remove = strings.CutSuffix(change, "-")
				if !remove {
					return errors.Errorf("label %q must use key=value or key-", change)
				}
			}
			if label == "" || strings.TrimSpace(label) != label {
				return errors.New("label key must be nonempty and trimmed")
			}
			if set {
				device.Labels[label] = value
				continue
			}
			delete(device.Labels, label)
		}

		// Keep the Device's diagnostic update time beside its new stored labels.
		device.UpdatedAt = timestamppb.New(time.Now())
		_, _, err = world.AccessWorldObject(ctx, ws, key, true, func(cursor *block.Cursor) error { cursor.SetBlock(device, true); return nil })
		return err
	})
}
