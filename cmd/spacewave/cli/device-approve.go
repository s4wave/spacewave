//go:build !js

package spacewave_cli

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// decodeDeviceLocalCompletion decodes a prefixed local SpaceLink completion.
func decodeDeviceLocalCompletion(encoded string) (*s4wave_session.LocalSpaceLinkCompletion, error) {
	// Decode the base64 completion and unmarshal its proto record.
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, deviceLocalCompletionPrefix))
	if err != nil {
		return nil, errors.Wrap(err, "decode local SpaceLink completion")
	}
	completion := &s4wave_session.LocalSpaceLinkCompletion{}
	if err := completion.UnmarshalVT(raw); err != nil {
		return nil, errors.Wrap(err, "unmarshal local SpaceLink completion")
	}
	return completion, nil
}

// deviceLocalCompletionPrefix marks a completion produced by a local
// mounted-session approval. Cloud completions stay bare base64 so existing
// stored records remain readable.
const deviceLocalCompletionPrefix = "spacelink-local-v1:"

type deviceApproveArgs struct {
	statePath  string
	sessionIdx uint
	spaceID    string
	ticket     string
}

func newDeviceApproveCommand() *cli.Command {
	args := &deviceApproveArgs{}
	return &cli.Command{
		Name:      "approve",
		Usage:     "approve a SpaceLink Device ticket for a Space",
		ArgsUsage: "<ticket>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// BuildFlags returns flags for owner-session Device approval.
func (a *deviceApproveArgs) BuildFlags() []cli.Flag {
	return append(
		clientFlags(&a.statePath, &a.sessionIdx),
		&cli.StringFlag{
			Name:        "space",
			Usage:       "target Space name or shared object ID",
			Required:    true,
			Destination: &a.spaceID,
		},
		&cli.StringFlag{
			Name:        "ticket",
			Usage:       "base64 SpaceLink ticket from the Device",
			Destination: &a.ticket,
		},
	)
}

// Run consumes one SpaceLink ticket through the mounted session approval API.
func (a *deviceApproveArgs) Run(c *cli.Context) error {
	// Collect the ticket text from the flag or the single argument.
	if c.NArg() > 1 {
		return errors.New("device approve accepts one ticket")
	}
	ticketText := strings.TrimSpace(a.ticket)
	if ticketText == "" && c.NArg() == 1 {
		ticketText = strings.TrimSpace(c.Args().First())
	}
	if ticketText == "" {
		return errors.New("device approve ticket is required")
	}

	// Decode the ticket and reject an empty payload.
	ticket, err := base64.StdEncoding.DecodeString(ticketText)
	if err != nil {
		return errors.Wrap(err, "decode SpaceLink ticket")
	}
	if len(ticket) == 0 {
		return errors.New("device approve ticket is empty")
	}

	// Connect to the daemon and mount the owner session for the Space.
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, a.statePath)
	if err != nil {
		return err
	}
	defer client.close()
	sess, err := client.mountSession(ctx, sessionIndex32(a.sessionIdx))
	if err != nil {
		return err
	}
	defer sess.Release()

	// Resolve the Space's resource ID and the session's provider.
	resourceID, err := client.resolveSpaceID(ctx, sess, a.spaceID)
	if err != nil {
		return err
	}

	// Read the session's provider ID from its info record.
	info, err := sess.GetSessionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get session info")
	}
	providerID := info.GetSessionRef().GetProviderResourceRef().GetProviderId()

	// Resolve the session's RPC client for the provider's approval service.
	sessionClient, err := sess.GetResourceRef().GetClient()
	if err != nil {
		return errors.Wrap(err, "session client")
	}

	// Approve the ticket through the cloud session service and print its completion.
	var payload []byte
	switch providerID {
	case "spacewave":
		service := s4wave_session.NewSRPCSpacewaveSessionResourceServiceClient(sessionClient)
		response, err := service.ApproveSpaceLink(ctx, &s4wave_provider_spacewave.ApproveSpaceLinkRequest{
			Ticket:     ticket,
			ResourceId: []byte(resourceID),
		})
		if err != nil {
			return errors.Wrap(err, "approve SpaceLink Device")
		}
		completion := response.GetCompletion()
		if completion == nil {
			return errors.New("SpaceLink approval returned no completion")
		}
		payload, err = completion.MarshalVT()
		if err != nil {
			return errors.Wrap(err, "encode SpaceLink completion")
		}
		_, err = fmt.Fprintln(os.Stdout, base64.StdEncoding.EncodeToString(payload))
		return err
	case "local":
	default:
		return errors.Errorf("session provider %q does not support SpaceLink Device approval", providerID)
	}

	// Approve the ticket through the local session service and print its completion.
	service := s4wave_session.NewSRPCLocalSessionResourceServiceClient(sessionClient)
	response, err := service.ApproveSpaceLink(ctx, &s4wave_session.ApproveLocalSpaceLinkRequest{
		Ticket:     ticket,
		ResourceId: resourceID,
	})
	if err != nil {
		return errors.Wrap(err, "approve SpaceLink Device")
	}

	// Reject a response without a completion and encode it.
	completion := response.GetCompletion()
	if completion == nil {
		return errors.New("SpaceLink approval returned no completion")
	}
	payload, err = completion.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "encode SpaceLink completion")
	}
	_, err = fmt.Fprintln(os.Stdout, deviceLocalCompletionPrefix+base64.StdEncoding.EncodeToString(payload))
	return err
}
