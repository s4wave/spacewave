//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// newSpaceMembersCommand builds the space members subcommand.
func newSpaceMembersCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "members",
		Usage:     "list the members of a space",
		ArgsUsage: "[space-id]",
		Flags:     []cli.Flag{outputFlag()},
		Action: func(c *cli.Context) error {
			// Read the members of the Space.
			if c.NArg() > 1 {
				return errors.New("usage: space members [space-id]")
			}
			ctx := c.Context
			members, err := readSpaceMembers(ctx, c, *statePath, *sessionIdx, c.Args().First())
			if err != nil {
				return err
			}

			// Emit them as JSON or YAML.
			if format := c.String("output"); format == "json" || format == "yaml" {
				data, err := (&s4wave_space.SpaceSharingState{ParticipantInfo: members}).MarshalJSON()
				if err != nil {
					return err
				}
				return formatOutput(data, format)
			}

			// Print one row per member.
			rows := [][]string{{"ACCOUNT", "NAME", "ROLE", "PEERS"}}
			for _, member := range members {
				name := member.GetEntityId()
				if member.GetIsSelf() {
					name += " (you)"
				}
				rows = append(rows, []string{
					member.GetAccountId(),
					name,
					formatParticipantRole(member.GetRole()),
					strings.Join(member.GetPeerIds(), ","),
				})
			}
			writeTable(os.Stdout, "", rows)
			return nil
		},
		Subcommands: []*cli.Command{
			newSpaceMembersRemoveCommand(statePath, sessionIdx),
		},
	}
}

// newSpaceMembersRemoveCommand builds the space members remove subcommand.
func newSpaceMembersRemoveCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "remove",
		Usage:     "remove members from a space",
		ArgsUsage: "[space-id] <member>...",
		Description: "Removes each member, named by account ID, name or peer ID. " +
			"Listed members leave in one change, which rotates the space key. " +
			"An account ID the list does not show is resolved to its session " +
			"peers through the cloud and removed in its own change. With one " +
			"argument it names a member of the default space; with more, the " +
			"first names the space.",
		Action: func(c *cli.Context) error {
			// Split the Space from the members.
			args := c.Args().Slice()
			if len(args) == 0 {
				return errors.New("usage: space members remove [space-id] <member>...")
			}
			var spaceArg string
			if len(args) > 1 {
				spaceArg, args = args[0], args[1:]
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

			// Resolve the Space and read its members.
			spaceID, err := client.resolveSpaceID(ctx, sess, spaceArg)
			if err != nil {
				return err
			}
			members, err := watchSpaceMembers(ctx, client, sess, spaceID)
			if err != nil {
				return err
			}

			// Split the names into listed members and other account IDs.
			peerIDs, accountIDs, err := memberPeerIDs(members, args)
			if err != nil {
				return err
			}

			// Remove the listed members in one change.
			if len(peerIDs) != 0 {
				resp, err := sess.RemoveSpaceParticipants(ctx, spaceID, peerIDs)
				if err != nil {
					return err
				}
				if resp.GetAwaitingGroup() {
					os.Stdout.WriteString(awaitingGroupMessage)
				} else {
					os.Stdout.WriteString("Removed " + strings.Join(resp.GetRemovedPeerIds(), ", ") + "\n")
				}
			}

			// Remove each other account by the peers the cloud lists for it.
			for _, accountID := range accountIDs {
				if err := removeSpaceAccount(ctx, sess, spaceID, accountID); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// readSpaceMembers mounts the selected session and returns the members of the
// Space spaceArg names.
func readSpaceMembers(
	ctx context.Context,
	c *cli.Context,
	statePath string,
	sessionIdx uint,
	spaceArg string,
) ([]*s4wave_space.SpaceParticipantInfo, error) {
	// Mount the selected session.
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return nil, err
	}
	defer client.close()
	sess, err := client.mountSession(ctx, sessionIndex32(sessionIdx))
	if err != nil {
		return nil, err
	}
	defer sess.Release()

	// Resolve the Space and read its members.
	spaceID, err := client.resolveSpaceID(ctx, sess, spaceArg)
	if err != nil {
		return nil, err
	}
	return watchSpaceMembers(ctx, client, sess, spaceID)
}

// watchSpaceMembers mounts the Space and returns its current members.
func watchSpaceMembers(
	ctx context.Context,
	client *sdkClient,
	sess *s4wave_session.Session,
	spaceID string,
) ([]*s4wave_space.SpaceParticipantInfo, error) {
	// Mount the Space.
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		return nil, err
	}
	defer spaceCleanup()

	// Read the first sharing state.
	strm, err := spaceSvc.WatchSpaceSharingState(ctx, &s4wave_space.WatchSpaceSharingStateRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "watch space sharing state")
	}
	defer strm.Close()
	state, err := strm.Recv()
	if err != nil {
		return nil, errors.Wrap(err, "recv space sharing state")
	}
	return state.GetParticipantInfo(), nil
}

// awaitingGroupMessage reports a removal the group must still agree to.
const awaitingGroupMessage = "Asked the group; the members leave once enough members agree.\n"

// removeSpaceAccount removes every session peer of the account from the Space
// in one change.
func removeSpaceAccount(
	ctx context.Context,
	sess *s4wave_session.Session,
	spaceID string,
	accountID string,
) error {
	// Ask the cloud session to remove the account's peers.
	sessionClient, err := sess.GetResourceRef().GetClient()
	if err != nil {
		return errors.Wrap(err, "session client")
	}
	svc := s4wave_session.NewSRPCSpacewaveSessionResourceServiceClient(sessionClient)
	resp, err := svc.RemoveSpaceMember(ctx, &s4wave_provider_spacewave.RemoveSpaceMemberRequest{
		SpaceId:   spaceID,
		AccountId: accountID,
	})
	if err != nil {
		return errors.Wrapf(err, "remove %s", accountID)
	}
	if resp.GetAwaitingGroup() {
		os.Stdout.WriteString(awaitingGroupMessage)
		return nil
	}

	// Report the removed peers.
	var removed []string
	for _, result := range resp.GetResults() {
		if msg := result.GetError(); msg != "" {
			return errors.Errorf("remove %s peer %s: %s", accountID, result.GetPeerId(), msg)
		}
		if result.GetRemoved() {
			removed = append(removed, result.GetPeerId())
		}
	}
	if len(removed) == 0 {
		os.Stdout.WriteString(accountID + " had no peers in this space\n")
		return nil
	}
	os.Stdout.WriteString("Removed " + accountID + ": " + strings.Join(removed, ", ") + "\n")
	return nil
}

// memberPeerIDs returns the peers of the listed members the names select. A
// name matches a member's account ID, name or one of its peer IDs. A name that
// matches no listed member is returned as an account ID.
func memberPeerIDs(
	members []*s4wave_space.SpaceParticipantInfo,
	names []string,
) (peerIDs, accountIDs []string, err error) {
	for _, name := range names {
		// Find the member, or keep the name as an account ID.
		idx := slices.IndexFunc(members, func(member *s4wave_space.SpaceParticipantInfo) bool {
			return name == member.GetAccountId() ||
				name == member.GetEntityId() ||
				slices.Contains(member.GetPeerIds(), name)
		})
		if idx < 0 {
			if !slices.Contains(accountIDs, name) {
				accountIDs = append(accountIDs, name)
			}
			continue
		}

		// Keep the viewer, who leaves with space leave instead.
		member := members[idx]
		if member.GetIsSelf() {
			return nil, nil, errors.Errorf("%s is you; use space leave instead", name)
		}
		for _, peerID := range member.GetPeerIds() {
			if !slices.Contains(peerIDs, peerID) {
				peerIDs = append(peerIDs, peerID)
			}
		}
	}
	return peerIDs, accountIDs, nil
}

// formatParticipantRole returns the lowercase name of a participant role.
func formatParticipantRole(role sobject.SOParticipantRole) string {
	return strings.ToLower(strings.TrimPrefix(role.String(), "SOParticipantRole_"))
}
