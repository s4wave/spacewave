//go:build !js

package spacewave_cli

import (
	"context"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// newSpaceBackfillCommand builds the space backfill subcommand.
func newSpaceBackfillCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "backfill",
		Usage:     "choose whether this device copies the whole space locally",
		ArgsUsage: "[space-id] [on|off]",
		Description: "With backfill on, this device copies the whole space into its " +
			"local store in the background. With backfill off, the default, it " +
			"fetches data when it is read and keeps what it wrote or read. " +
			"Without on or off, it prints the current choice.",
		Action: func(c *cli.Context) error {
			// Take the optional choice from the last argument.
			args := c.Args().Slice()
			if len(args) > 2 {
				return errors.New("usage: space backfill [space-id] [on|off]")
			}
			var choice, spaceArg string
			if n := len(args); n != 0 && (args[n-1] == "on" || args[n-1] == "off") {
				choice, args = args[n-1], args[:n-1]
			}
			if len(args) == 2 {
				return errors.New("usage: space backfill [space-id] [on|off]")
			}
			if len(args) == 1 {
				spaceArg = args[0]
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

			// Resolve and mount the Space.
			spaceID, err := client.resolveSpaceID(ctx, sess, spaceArg)
			if err != nil {
				return err
			}
			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			// Print the current choice, or store the new one.
			if choice == "" {
				state := readSpaceBackfill(ctx, spaceSvc)
				if state == nil {
					return errors.New("this space has no backfill choice")
				}
				os.Stdout.WriteString(formatBackfill(state.GetBackfill()) + "\n")
				return nil
			}
			return setSpaceBackfill(ctx, spaceSvc, choice == "on")
		},
	}
}

// setSpaceBackfill stores this device's backfill choice for the Space and
// reports it.
func setSpaceBackfill(ctx context.Context, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient, backfill bool) error {
	req := &s4wave_space.SetSpaceBackfillRequest{Backfill: backfill}
	if _, err := spaceSvc.SetSpaceBackfill(ctx, req); err != nil {
		return errors.Wrap(err, "set space backfill")
	}
	os.Stdout.WriteString(formatBackfill(backfill) + "\n")
	return nil
}

// readSpaceBackfill reads this device's backfill choice for the Space, or
// returns nil when the Space has none.
func readSpaceBackfill(ctx context.Context, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient) *s4wave_space.SpaceBackfillState {
	// Read the first state from the watch.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	strm, err := spaceSvc.WatchSpaceBackfill(ctx, &s4wave_space.WatchSpaceBackfillRequest{})
	if err != nil {
		return nil
	}
	state, err := strm.Recv()
	if err != nil {
		return nil
	}
	return state
}

// formatBackfill describes a backfill choice.
func formatBackfill(backfill bool) string {
	if backfill {
		return "on, this device copies the whole space locally"
	}
	return "off, this device fetches data on demand"
}
