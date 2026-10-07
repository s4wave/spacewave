//go:build !js

package spacewave_cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// newSpaceRosterCommand builds the space roster subcommand.
func newSpaceRosterCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "roster",
		Usage:     "set the writers dropped from the space's trimming roster",
		ArgsUsage: "<space-id> [peer-id...]",
		Description: "The space trims only the history every writer on its " +
			"roster has built on, so a writer that stopped syncing holds the " +
			"history back. The listed writers become the whole dropped set: " +
			"list every writer that should stay dropped, since a dropped writer " +
			"left out returns to the roster. Without peer IDs it returns every " +
			"dropped writer. A dropped writer returns on its own once it catches " +
			"up. Only an owner can change it.",
		Action: func(c *cli.Context) error {
			// Take the space and the writers to drop.
			args := c.Args().Slice()
			if len(args) == 0 {
				return errors.New("usage: space roster <space-id> [peer-id...]")
			}
			spaceArg, dropped := args[0], args[1:]

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

			// Change the roster and report the result.
			req := &s4wave_space.SetSpaceRosterRequest{DroppedPeerIds: dropped}
			resp, err := spaceSvc.SetSpaceRoster(ctx, req)
			if err != nil {
				return errors.Wrap(err, "set space roster")
			}
			switch {
			case resp.GetAwaitingGroup():
				os.Stdout.WriteString("agreed; the change applies once the group decides it\n")
			case resp.GetChanged():
				os.Stdout.WriteString("roster changed\n")
			default:
				os.Stdout.WriteString("roster already drops exactly these writers\n")
			}
			return nil
		},
	}
}
