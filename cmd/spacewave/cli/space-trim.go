//go:build !js

package spacewave_cli

import (
	"fmt"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
)

// newSpaceTrimCommand builds the space trim subcommand.
func newSpaceTrimCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "trim",
		Usage:     "show how far this device can trim the space's history",
		ArgsUsage: "<space-id>",
		Description: "Prints this device's view of the checkpoint and the " +
			"operations above it: how many replay has placed, the stable " +
			"prefix every roster writer built on, whether this device should " +
			"acknowledge, the heads that wait for a missing operation, and " +
			"each writer's latest operation.",
		Action: func(c *cli.Context) error {
			// Take the space.
			if c.Args().Len() != 1 {
				return errors.New("usage: space trim <space-id>")
			}

			// Mount the selected session.
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()
			sess, err := client.mountSession(ctx, sessionIndex32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			// Resolve and mount the Space's shared object.
			spaceID, err := client.resolveSpaceID(ctx, sess, c.Args().First())
			if err != nil {
				return err
			}
			soSvc, soRelease, err := client.mountSharedObject(ctx, sess, spaceID)
			if err != nil {
				return err
			}
			defer soRelease()

			// Read and print the trim state.
			trim, err := soSvc.GetSharedObjectTrim(ctx, &s4wave_sobject.GetSharedObjectTrimRequest{})
			if err != nil {
				return errors.Wrap(err, "get shared object trim")
			}
			printSharedObjectTrim(trim)
			return nil
		},
	}
}

// printSharedObjectTrim prints trim to stdout.
func printSharedObjectTrim(trim *s4wave_sobject.SharedObjectTrim) {
	// Print the order, the stable point and this device's acknowledgment.
	w := os.Stdout
	fmt.Fprintf(w, "Viewer:          %s\n", trim.GetViewerPeerId())
	fmt.Fprintf(w, "Checkpoint:      %d\n", trim.GetCheckpointHeight())
	fmt.Fprintf(w, "Operations:      %d (%d placed)\n", trim.GetOperations(), trim.GetPlaced())
	fmt.Fprintf(w, "Stable:          %d\n", trim.GetStable())
	fmt.Fprintf(w, "Sequencer:       %s\n", trim.GetSequencerPeerId())
	fmt.Fprintf(w, "Checkpointer:    %s\n", trim.GetCheckpointerPeerId())
	fmt.Fprintf(w, "Acknowledge now: %t\n", trim.GetNeedsAcknowledgment())

	// List the heads that wait for an operation this device lacks.
	for _, head := range trim.GetUnplacedHeads() {
		fmt.Fprintf(w, "Unplaced head:   %s nonce %d\n", head.GetPeerId(), head.GetNonce())
	}

	// List each writer's share in the stable point.
	fmt.Fprintf(w, "%-54s  %-7s  %-9s  %s\n", "WRITER", "ROSTER", "NONCE", "BUILT ON CHECKPOINTER")
	for _, writer := range trim.GetWriters() {
		roster := "member"
		if writer.GetDropped() {
			roster = "dropped"
		}
		fmt.Fprintf(w, "%-54s  %-7s  %-9d  %t\n", writer.GetPeerId(), roster, writer.GetLatestNonce(), writer.GetBuiltOnCheckpointer())
	}
}
