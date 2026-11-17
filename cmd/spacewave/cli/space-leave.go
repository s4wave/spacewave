//go:build !js

package spacewave_cli

import (
	"os"
	"slices"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// newSpaceLeaveCommand builds the space leave subcommand.
func newSpaceLeaveCommand(statePath *string, sessionIdx *uint) *cli.Command {
	var successor string
	return &cli.Command{
		Name:      "leave",
		Usage:     "leave a space, keeping its local data",
		ArgsUsage: "[space-id]",
		Description: "Removes this session from the space; the data it holds " +
			"stays on this device. When this session owns the space and others " +
			"remain, ownership passes to --successor, a member named by account " +
			"ID, name or peer ID as in space members remove. Without it the " +
			"remaining member with the highest role, earliest admitted, becomes " +
			"the owner.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "successor",
				Usage:       "member who becomes the owner: account ID, name or peer ID",
				Destination: &successor,
			},
		},
		Action: func(c *cli.Context) error {
			// Mount the selected session.
			if c.NArg() > 1 {
				return errors.New("usage: space leave [space-id] [--successor <member>]")
			}
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

			// Resolve the Space, and the successor's peer from its members.
			spaceID, err := client.resolveSpaceID(ctx, sess, c.Args().First())
			if err != nil {
				return err
			}
			var successorPeer string
			if successor != "" {
				state, err := watchSpaceMembers(ctx, client, sess, spaceID)
				if err != nil {
					return err
				}
				successorPeer, err = successorPeerID(state, successor)
				if err != nil {
					return err
				}
			}

			// Leave, handing ownership to the successor when this session owns the Space.
			if _, err := sess.LeaveSpace(ctx, spaceID, successorPeer); err != nil {
				return errors.Wrap(err, "leave space")
			}
			if successorPeer != "" {
				os.Stdout.WriteString("Left " + spaceID + "; successor " + successorPeer + "\n")
			} else {
				os.Stdout.WriteString("Left " + spaceID + "\n")
			}
			return nil
		},
	}
}

// successorPeerID returns the participant peer that name selects as the
// owner's successor. A peer ID selects itself. An account ID or name selects
// that member's peer with the highest role, the earliest in configuration
// order among equals, as the default successor is chosen. The viewer cannot
// succeed itself.
func successorPeerID(state *s4wave_space.SpaceSharingState, name string) (string, error) {
	// Find the first member the name selects, as space members remove does.
	members := state.GetParticipantInfo()
	idx := slices.IndexFunc(members, func(member *s4wave_space.SpaceParticipantInfo) bool {
		return name == member.GetAccountId() ||
			name == member.GetEntityId() ||
			slices.Contains(member.GetPeerIds(), name)
	})
	if idx < 0 {
		return "", errors.Errorf("%s is not a member of this space", name)
	}
	member := members[idx]
	if member.GetIsSelf() {
		return "", errors.Errorf("%s is you; name another member", name)
	}
	if slices.Contains(member.GetPeerIds(), name) {
		return name, nil
	}

	// Pick the member's strongest peer.
	var best *sobject.SOParticipantConfig
	for _, p := range state.GetParticipants() {
		if slices.Contains(member.GetPeerIds(), p.GetPeerId()) && p.GetRole() > best.GetRole() {
			best = p
		}
	}
	if best == nil {
		return "", errors.Errorf("%s has no participant peer in this space", name)
	}
	return best.GetPeerId(), nil
}
