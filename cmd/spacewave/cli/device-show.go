//go:build !js

package spacewave_cli

import (
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
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
	mount, err := mountObjectChain(c, a.statePath, uri, nil)
	if err != nil {
		return err
	}
	defer mount.release()

	// Read the Device and its Worker claim from the same World revision.
	tx, err := mount.Engine.NewTransaction(c.Context, false)
	if err != nil {
		return errors.Wrap(err, "read device world")
	}
	defer tx.Discard()
	device, err := world.LookupObjectBody[s4wave_device.Device](c.Context, tx, mount.ObjectKey)
	if err != nil {
		return errors.Wrap(err, "read device state")
	}
	availability, err := forge_runtime.LookupForgeWorkerAvailability(c.Context, tx, mount.ObjectKey, device)
	if err != nil {
		return errors.Wrap(err, "read device availability")
	}

	// Print documents in the requested format.
	if a.output == "json" || a.output == "yaml" {
		data, err := marshalDeviceShow(mount.ObjectKey, device, availability)
		if err != nil {
			return errors.Wrap(err, "marshal device")
		}
		return formatOutput(data, a.output)
	}

	// Print the derived availability beside the Device's diagnostic fields.
	writeDeviceShow(os.Stdout, mount.ObjectKey, device, availability)
	return nil
}

// marshalDeviceShow adds effective labels and availability without changing the stored Device schema.
func marshalDeviceShow(objectKey string, device *s4wave_device.Device, availability forge_runtime.DeviceAvailability) ([]byte, error) {
	// Preserve the generated Device JSON representation.
	data, err := device.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var parser fastjson.Parser
	obj, err := parser.ParseBytes(data)
	if err != nil {
		return nil, err
	}

	// Describe the derived admission state independently of Device liveness.
	var arena fastjson.Arena
	claim := arena.NewObject()
	claim.Set("state", arena.NewString(string(availability.State)))
	claim.Set("reason", arena.NewString(availability.Reason))

	// Retain the observed owner epoch, deadline and state for diagnosis.
	claim.Set("claimEpoch", arena.NewNumberString(strconv.FormatUint(availability.ClaimEpoch, 10)))
	claim.Set("expiresAt", arena.NewString(formatDeviceClaimExpiry(availability.ExpiresAt)))
	claim.Set("ownerState", arena.NewString(formatDeviceOwnerState(availability.OwnerState)))

	// Expose the selector values that requirement placement actually matches.
	labels := arena.NewObject()
	for key, value := range device.EffectiveLabels(objectKey) {
		labels.Set(key, arena.NewString(value))
	}
	obj.Set("effectiveLabels", labels)

	// Add the read-only summary without changing the stored Device representation.
	obj.Set("availability", claim)
	return obj.MarshalTo(nil), nil
}

// writeDeviceShow prints availability before the self-reported capabilities.
func writeDeviceShow(w io.Writer, objectKey string, device *s4wave_device.Device, availability forge_runtime.DeviceAvailability) {
	// Show the claim deadline separately from the last self-reported liveness.
	writeFields(w, [][2]string{
		{"Device", objectKey},
		{"Label", device.GetLabel()},
		{"Peer", device.GetPeerId()},
		{"Availability", string(availability.State)},
		{"Reason", availability.Reason},
		{"Claim epoch", strconv.FormatUint(availability.ClaimEpoch, 10)},
		{"Claim expiry", formatDeviceClaimExpiry(availability.ExpiresAt)},
		{"Owner state", formatDeviceOwnerState(availability.OwnerState)},
		{"Last liveness", strings.TrimPrefix(device.GetLastStatus().GetLiveness().String(), "DEVICE_LIVENESS_")},
	})

	// Show derived selectors and stored overrides in stable key order.
	labels := device.EffectiveLabels(objectKey)
	io.WriteString(w, "\nLABELS:\n")
	rows := make([][]string, 0, len(labels)+1)
	rows = append(rows, []string{"KEY", "VALUE"})
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		rows = append(rows, []string{key, labels[key]})
	}
	writeTable(w, "  ", rows)

	// Keep capability reports visible as diagnostics, not admission evidence.
	if capabilities := device.GetCapabilities(); len(capabilities) != 0 {
		io.WriteString(w, "\nCAPABILITIES:\n")
		rows := [][]string{{"ID", "KIND", "STATE", "DETAIL"}}
		for _, capability := range capabilities {
			state := strings.TrimPrefix(capability.GetState().String(), "DEVICE_CAPABILITY_STATE_")
			rows = append(rows, []string{capability.GetId(), capability.GetKind(), strings.ToLower(state), capability.GetDetail()})
		}
		writeTable(w, "  ", rows)
	}
}

// formatDeviceClaimExpiry renders an observed deadline or no claim.
func formatDeviceClaimExpiry(expiry time.Time) string {
	if expiry.IsZero() {
		return ""
	}
	return expiry.UTC().Format(time.RFC3339Nano)
}

// formatDeviceOwnerState names the stored claim state independently of expiry.
func formatDeviceOwnerState(state forge_runtime.CapacityOwnerState) string {
	switch state {
	case forge_runtime.CapacityOwnerStateActive:
		return "ACTIVE"
	case forge_runtime.CapacityOwnerStateDraining:
		return "DRAINING"
	default:
		return "UNSPECIFIED"
	}
}
