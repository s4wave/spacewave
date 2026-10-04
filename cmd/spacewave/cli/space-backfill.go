//go:build !js

package spacewave_cli

import (
	"context"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// newSpaceBackfillCommand builds the space backfill subcommand.
func newSpaceBackfillCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "backfill",
		Usage:     "choose whether this device copies the whole space locally",
		ArgsUsage: "[space-id] on|off",
		Description: "With backfill on, this device copies the whole space into its " +
			"local store in the background. With backfill off, the default, it " +
			"fetches data when it is read and keeps what it wrote or read.",
		Action: func(c *cli.Context) error {
			// Parse the choice from the last argument.
			args := c.Args().Slice()
			if len(args) == 0 || len(args) > 2 {
				return errors.New("usage: space backfill [space-id] on|off")
			}
			backfill, err := parseBackfill(args[len(args)-1])
			if err != nil {
				return err
			}
			var spaceArg string
			if len(args) == 2 {
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

			// Resolve the Space and store the choice.
			spaceID, err := client.resolveSpaceID(ctx, sess, spaceArg)
			if err != nil {
				return err
			}
			return setSpaceBackfill(ctx, client, sess, spaceID, backfill)
		},
	}
}

// parseBackfill parses an on or off backfill choice.
func parseBackfill(arg string) (bool, error) {
	switch arg {
	case "on":
		return true, nil
	case "off":
		return false, nil
	default:
		return false, errors.Errorf("backfill must be on or off, not %q", arg)
	}
}

// setSpaceBackfill stores this device's backfill choice for the Space and
// reports it.
func setSpaceBackfill(
	ctx context.Context,
	client *sdkClient,
	sess *s4wave_session.Session,
	spaceID string,
	backfill bool,
) error {
	// Mount the Space service.
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		return err
	}
	defer spaceCleanup()

	// Store the choice.
	req := &s4wave_space.SetSpaceBackfillRequest{Backfill: backfill}
	if _, err := spaceSvc.SetSpaceBackfill(ctx, req); err != nil {
		return errors.Wrap(err, "set space backfill")
	}
	if backfill {
		os.Stdout.WriteString("backfill on: this device copies the whole space locally\n")
	} else {
		os.Stdout.WriteString("backfill off: this device fetches data on demand\n")
	}
	return nil
}
