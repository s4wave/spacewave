//go:build !js

package spacewave_cli

import (
	"io"
	"os"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// deviceShowArgs selects the Device object to show.
type deviceShowArgs struct {
	// statePath is the daemon state directory.
	statePath string
	// sessionIdx is the session whose Space holds the Device.
	sessionIdx uint
	// spaceID is the Space ID or name, or empty for the only Space.
	spaceID string
	// output is the output format.
	output string
}

// newDeviceShowCommand builds the device show subcommand.
func newDeviceShowCommand() *cli.Command {
	args := &deviceShowArgs{}
	return &cli.Command{
		Name:      "show",
		Usage:     "show a Device object and its capability list",
		ArgsUsage: "<device-object-key>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// BuildFlags returns flags for device show.
func (a *deviceShowArgs) BuildFlags() []cli.Flag {
	return append(
		clientFlags(&a.statePath, &a.sessionIdx),
		&cli.StringFlag{
			Name:        "space",
			Usage:       "Space ID or name (default: the only Space)",
			EnvVars:     []string{"SPACEWAVE_SPACE"},
			Destination: &a.spaceID,
		},
		&cli.StringFlag{
			Name:        "output",
			Aliases:     []string{"o"},
			Usage:       "output format (text/json/yaml)",
			Value:       "text",
			Destination: &a.output,
		},
	)
}

// Run prints the current state of the Device object named by the argument.
func (a *deviceShowArgs) Run(c *cli.Context) error {
	// Require the Device object key.
	if c.NArg() != 1 {
		return errors.New("device show requires <device-object-key>")
	}

	// Mount the Device object's typed Resource.
	uri := fsURI{sessionIdx: sessionIndex32(a.sessionIdx), spaceID: a.spaceID, objectKey: c.Args().First()}
	mount, cleanup, err := mountObjectChain(c, a.statePath, uri, nil)
	if err != nil {
		return err
	}
	defer cleanup()

	// Read the current Device state from the Resource's watch.
	stream, err := s4wave_device.NewSRPCDeviceResourceServiceClient(mount.typedClient).
		WatchDeviceState(c.Context, &s4wave_device.WatchDeviceStateRequest{})
	if err != nil {
		return errors.Wrap(err, "watch device state")
	}
	defer stream.Close()
	resp, err := stream.Recv()
	if err != nil {
		return errors.Wrap(err, "read device state")
	}
	device := resp.GetState()

	// Print documents in the requested format.
	if a.output == "json" || a.output == "yaml" {
		data, err := device.MarshalJSON()
		if err != nil {
			return errors.Wrap(err, "marshal device")
		}
		return formatOutput(data, a.output)
	}

	// Print the header fields and one row per capability.
	w := os.Stdout
	writeFields(w, [][2]string{
		{"Device", mount.objectKey},
		{"Label", device.GetLabel()},
		{"Peer", device.GetPeerId()},
	})
	if capabilities := device.GetCapabilities(); len(capabilities) != 0 {
		io.WriteString(w, "\nCAPABILITIES:\n")
		rows := [][]string{{"ID", "KIND", "STATE", "DETAIL"}}
		for _, capability := range capabilities {
			state := strings.TrimPrefix(capability.GetState().String(), "DEVICE_CAPABILITY_STATE_")
			rows = append(rows, []string{capability.GetId(), capability.GetKind(), strings.ToLower(state), capability.GetDetail()})
		}
		writeTable(w, "  ", rows)
	}
	return nil
}
