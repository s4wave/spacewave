//go:build !js

package spacewave_cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// billingOwnerTypeAccount is the owner type that assigns a billing account to
// a personal account.
const billingOwnerTypeAccount = "account"

// newBillingCreateCommand builds the billing create command.
func newBillingCreateCommand() *cli.Command {
	var statePath string
	var sessionIdx uint
	var displayName string
	return &cli.Command{
		Name:  "create",
		Usage: "create a billing account and assign it to the session's account",
		Flags: append(clientFlags(&statePath, &sessionIdx),
			&cli.StringFlag{
				Name:        "display-name",
				Usage:       "billing account display name",
				Destination: &displayName,
			},
		),
		Action: func(c *cli.Context) error {
			// Mount the selected cloud Session.
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, statePath)
			if err != nil {
				return err
			}
			defer client.close()
			sess, err := client.mountSession(ctx, sessionIndex32(sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			// Resolve the Session's account and its cloud client.
			info, err := sess.GetSessionInfo(ctx)
			if err != nil {
				return errors.Wrap(err, "get session info")
			}
			accountID := info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()
			sessionClient, err := sess.GetResourceRef().GetClient()
			if err != nil {
				return errors.Wrap(err, "session client")
			}
			svc := s4wave_session.NewSRPCSpacewaveSessionResourceServiceClient(sessionClient)

			// Create the billing account the caller manages.
			created, err := svc.CreateBillingAccount(ctx, &s4wave_provider_spacewave.CreateBillingAccountRequest{
				DisplayName: displayName,
			})
			if err != nil {
				return errors.Wrap(err, "create billing account")
			}

			// Assign it to the Session's account.
			baID := created.GetBillingAccountId()
			_, err = svc.AssignBillingAccount(ctx, &s4wave_provider_spacewave.AssignBillingAccountRequest{
				BillingAccountId: baID,
				TargetOwnerType:  billingOwnerTypeAccount,
				TargetOwnerId:    accountID,
			})
			if err != nil {
				return errors.Wrapf(err, "assign billing account %s", baID)
			}

			// Print the new billing account ID.
			os.Stdout.WriteString(baID + "\n")
			return nil
		},
	}
}
