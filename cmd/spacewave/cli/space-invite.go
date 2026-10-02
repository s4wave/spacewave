//go:build !js

package spacewave_cli

import (
	"os"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// bearerInvitePrefix marks an encoded invite token, as the app writes it.
const bearerInvitePrefix = "bearer:"

// newSpaceInviteCommand builds the space invite subcommand.
func newSpaceInviteCommand(statePath *string, sessionIdx *uint) *cli.Command {
	var roleName string
	var maxUses uint
	var expires time.Duration
	return &cli.Command{
		Name:      "invite",
		Usage:     "create an invite another account redeems with space join",
		ArgsUsage: "<space>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "role",
				Usage:       "participant role granted on joining: reader or writer",
				Value:       "writer",
				Destination: &roleName,
			},
			&cli.UintFlag{
				Name:        "max-uses",
				Usage:       "number of joins the invite allows, 0 for unlimited",
				Value:       1,
				Destination: &maxUses,
			},
			&cli.DurationFlag{
				Name:        "expires",
				Usage:       "time until the invite expires, 0 for never",
				Value:       24 * time.Hour,
				Destination: &expires,
			},
		},
		Action: func(c *cli.Context) error {
			// Validate the role before contacting the daemon.
			role, err := parseParticipantRole(roleName)
			if err != nil {
				return err
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

			// Resolve the Space by name or ID.
			spaceID, err := client.resolveSpaceID(ctx, sess, c.Args().First())
			if err != nil {
				return err
			}

			// Create the signed invite on the Space.
			req := &s4wave_session.CreateSpaceInviteRequest{
				SpaceId: spaceID,
				Role:    role,
				MaxUses: uint32(min(maxUses, uint(^uint32(0)))),
			}
			if expires > 0 {
				req.ExpiresAt = timestamppb.New(time.Now().Add(expires))
			}
			resp, err := sess.CreateSpaceInvite(ctx, req)
			if err != nil {
				return errors.Wrap(err, "create space invite")
			}

			// Print the cloud short code, or the encoded invite for a local session.
			if code := resp.GetShortCode(); code != "" {
				os.Stdout.WriteString(code + "\n")
				return nil
			}
			data, err := resp.GetInviteMessage().MarshalVT()
			if err != nil {
				return errors.Wrap(err, "encode invite")
			}
			os.Stdout.WriteString(bearerInvitePrefix + b58.Encode(data) + "\n")
			return nil
		},
	}
}

// newSpaceJoinCommand builds the space join subcommand.
func newSpaceJoinCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "join",
		Usage:     "join a space with an invite code, link or token",
		ArgsUsage: "<invite>",
		Action: func(c *cli.Context) error {
			// Require the invite argument.
			input := strings.TrimSpace(c.Args().First())
			if input == "" {
				return errors.New("invite required")
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

			// Decode a link or token locally; look a short code up in the cloud.
			invite, err := decodeInvite(input)
			if err != nil {
				return err
			}
			if invite == nil {
				sessionClient, err := sess.GetResourceRef().GetClient()
				if err != nil {
					return errors.Wrap(err, "session client")
				}
				svc := s4wave_session.NewSRPCSpacewaveSessionResourceServiceClient(sessionClient)
				resp, err := svc.LookupInviteCode(ctx, &s4wave_provider_spacewave.LookupInviteCodeRequest{Code: input})
				if err != nil {
					return errors.Wrap(err, "look up invite code")
				}
				if invite = resp.GetInviteMessage(); invite == nil {
					return errors.New("invite code not found")
				}
			}

			// Join and report the outcome.
			resp, err := sess.JoinSpaceViaInvite(ctx, &s4wave_session.JoinSpaceViaInviteRequest{InviteMessage: invite})
			if err != nil {
				return errors.Wrap(err, "join space")
			}
			switch resp.GetResult() {
			case s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_REJECTED:
				return errors.New("the space owner rejected the join")
			case s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_OWNER_MUST_BE_ONLINE:
				return errors.New("the space owner must be online to accept this invite")
			case s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_PENDING_OWNER_APPROVAL:
				os.Stdout.WriteString("join submitted; waiting for the space owner to approve\n")
			default:
				os.Stdout.WriteString(resp.GetSharedObjectId() + "\n")
			}
			return nil
		},
	}
}

// decodeInvite decodes an invite link or bearer token. It returns nil when the
// input carries neither, which makes it a cloud short code.
func decodeInvite(input string) (*sobject.SOInviteMessage, error) {
	// Take the encoded invite from a link or a bearer token.
	var encoded string
	switch {
	case strings.Contains(input, "/"):
		encoded = input[strings.LastIndex(input, "/")+1:]
	case strings.HasPrefix(input, bearerInvitePrefix):
		encoded = strings.TrimPrefix(input, bearerInvitePrefix)
	default:
		return nil, nil
	}

	// Decode the invite message.
	data, err := b58.Decode(encoded)
	if err != nil || len(data) == 0 {
		return nil, errors.New("invalid invite link")
	}
	invite := &sobject.SOInviteMessage{}
	if err := invite.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode invite")
	}
	return invite, nil
}

// parseParticipantRole parses a reader or writer role name.
func parseParticipantRole(name string) (sobject.SOParticipantRole, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "reader":
		return sobject.SOParticipantRole_SOParticipantRole_READER, nil
	case "writer":
		return sobject.SOParticipantRole_SOParticipantRole_WRITER, nil
	default:
		return sobject.SOParticipantRole_SOParticipantRole_UNKNOWN, errors.Errorf("role must be reader or writer, not %q", name)
	}
}
