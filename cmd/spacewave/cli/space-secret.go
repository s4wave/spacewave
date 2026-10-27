//go:build !js

package spacewave_cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// newSpaceSecretCommand builds the space secret command group.
func newSpaceSecretCommand(statePath *string, sessionIdx *uint) *cli.Command {
	var spaceID string
	return &cli.Command{
		Name:  "secret",
		Usage: "manage access to a space's Secrets",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "space-id",
				Aliases:     []string{"space"},
				Usage:       "space ID or name (auto-detected if only one space)",
				EnvVars:     []string{"SPACEWAVE_SPACE"},
				Destination: &spaceID,
			},
		},
		Subcommands: []*cli.Command{
			newSpaceSecretGrantCommand(statePath, sessionIdx, &spaceID),
		},
	}
}

// newSpaceSecretGrantCommand builds the space secret grant subcommand.
func newSpaceSecretGrantCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	var peerID, role string
	return &cli.Command{
		Name:      "grant",
		Usage:     "let a peer, such as a device, read or replace a Secret",
		ArgsUsage: "<object-key>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "peer", Usage: "peer ID to grant", Required: true, Destination: &peerID},
			&cli.StringFlag{Name: "role", Usage: "reader or writer", Value: "reader", Destination: &role},
		},
		Action: func(c *cli.Context) error {
			// Validate the Secret and role before connecting.
			objectKey := c.Args().First()
			if objectKey == "" {
				return errors.New("Secret object key required")
			}
			var participantRole sobject.SOParticipantRole
			switch role {
			case "reader":
				participantRole = sobject.SOParticipantRole_SOParticipantRole_READER
			case "writer":
				participantRole = sobject.SOParticipantRole_SOParticipantRole_WRITER
			default:
				return errors.Errorf("unknown role %q: use reader or writer", role)
			}

			// Connect to the daemon and mount the selected session.
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

			// Mount the Space and grant the peer under the session's key.
			sid, err := client.resolveSpaceID(ctx, sess, *spaceID)
			if err != nil {
				return err
			}
			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()
			if _, err := spaceSvc.AddSecretParticipant(ctx, &s4wave_space.AddSecretParticipantRequest{
				ObjectKey:       objectKey,
				PeerId:          peerID,
				ParticipantRole: participantRole,
			}); err != nil {
				return errors.Wrap(err, "grant Secret")
			}
			os.Stdout.WriteString("granted " + peerID + " " + role + " access to " + objectKey + "\n")
			return nil
		},
	}
}
